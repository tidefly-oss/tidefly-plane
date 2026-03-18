// Package podman provides a minimal HTTP client for the Podman libpod REST API
// over a Unix socket. It replaces the broken oapi-codegen generated client.
package podman

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// apiError represents a structured error response from the Podman API.
type apiError struct {
	StatusCode int
	Method     string
	Path       string
	Body       string
}

func (e *apiError) Error() string {
	if e.Body != "" {
		// Try to extract the "message" field from a JSON error body
		var podmanErr struct {
			Message string `json:"message"`
			Cause   string `json:"cause"`
		}
		if err := json.Unmarshal([]byte(e.Body), &podmanErr); err == nil {
			if podmanErr.Message != "" {
				if podmanErr.Cause != "" {
					return fmt.Sprintf("status %d: %s: %s", e.StatusCode, podmanErr.Message, podmanErr.Cause)
				}
				return fmt.Sprintf("status %d: %s", e.StatusCode, podmanErr.Message)
			}
		}
		return fmt.Sprintf("status %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("status %d", e.StatusCode)
}

func newAPIError(statusCode int, method, path string, body []byte) *apiError {
	return &apiError{
		StatusCode: statusCode,
		Method:     method,
		Path:       path,
		Body:       strings.TrimSpace(string(body)),
	}
}

type client struct {
	socketPath string
	apiVersion string
	http       *http.Client
}

func newClient(socketPath string) *client {
	c := &client{
		socketPath: socketPath,
		apiVersion: "v5.0.0",
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
				},
			},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if v := c.negotiateVersion(ctx); v != "" {
		c.apiVersion = v
	}
	return c
}

func (c *client) negotiateVersion(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, podmanRoot+"/_ping", nil)
	if err == nil {
		if resp, err := c.http.Do(req); err == nil {
			v := resp.Header.Get("Libpod-API-Version")
			err := resp.Body.Close()
			if err != nil {
				return ""
			}
			if v != "" {
				return "v" + v
			}
		}
	}

	for _, v := range []string{"v5.4.0", "v5.3.0", "v5.2.0", "v5.1.0", "v5.0.0", "v4.9.0"} {
		testURL := podmanRoot + "/" + v + "/libpod/containers/json?limit=0"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
		if err != nil {
			continue
		}
		resp, err := c.http.Do(req)
		if err != nil {
			continue
		}
		err2 := resp.Body.Close()
		if err2 != nil {
			return ""
		}
		if resp.StatusCode == 200 || resp.StatusCode == 204 {
			return v
		}
	}
	return ""
}

const podmanRoot = "http://localhost"

func (c *client) url(path string, query url.Values) string {
	base := podmanRoot + "/" + c.apiVersion
	if strings.HasSuffix(path, "_ping") || strings.HasPrefix(path, "/libpod/version") {
		base = podmanRoot
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *client) get(ctx context.Context, path string, query url.Values) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path, query), nil)
	if err != nil {
		return nil, fmt.Errorf("build GET %s: %w", path, err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	return resp, nil
}

// getJSON decodes a JSON response into dst. On non-2xx responses it returns
// an apiError with the full response body included.
func (c *client) getJSON(ctx context.Context, path string, query url.Values, dst any) (int, error) {
	resp, err := c.get(ctx, path, query)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("warning: failed to close response body for GET %s: %v\n", path, err)
		}
	}(resp.Body)

	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, newAPIError(resp.StatusCode, http.MethodGet, path, b)
	}

	if dst != nil && len(b) > 0 {
		if err := json.Unmarshal(b, dst); err != nil && err != io.EOF {
			return resp.StatusCode, fmt.Errorf("decode %s: %w (body: %s)", path, err, truncate(b, 200))
		}
	}
	return resp.StatusCode, nil
}

// post sends a JSON body and returns (statusCode, responseBody, error).
// Errors are only returned for transport failures — callers must check
// the status code themselves. Use checkStatus() for convenience.
func (c *client) post(ctx context.Context, path string, query url.Values, body any) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("marshal body for POST %s: %w", path, err)
		}
		r = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path, query), r)
	if err != nil {
		return 0, nil, fmt.Errorf("build POST %s: %w", path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("warning: failed to close response body for POST %s: %v\n", path, err)
		}
	}(resp.Body)

	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b, nil
}

// postExpect is a convenience wrapper around post that returns an apiError
// when the status code is not one of the expected codes.
func (c *client) postExpect(ctx context.Context, path string, query url.Values, body any, expected ...int) (
	int, []byte, error,
) {
	code, b, err := c.post(ctx, path, query, body)
	if err != nil {
		return code, b, err
	}
	for _, e := range expected {
		if code == e {
			return code, b, nil
		}
	}
	return code, b, newAPIError(code, http.MethodPost, path, b)
}

func (c *client) postRaw(
	ctx context.Context, path string, query url.Values, contentType string, body io.Reader,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path, query), body)
	if err != nil {
		return nil, fmt.Errorf("build POST %s: %w", path, err)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	return resp, nil
}

// delete sends a DELETE request and returns (statusCode, error).
// On non-2xx responses it reads the body and returns an apiError.
func (c *client) delete(ctx context.Context, path string, query url.Values) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.url(path, query), nil)
	if err != nil {
		return 0, fmt.Errorf("build DELETE %s: %w", path, err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Printf("warning: failed to close response body for DELETE %s: %v\n", path, err)
		}
	}(resp.Body)

	b, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, newAPIError(resp.StatusCode, http.MethodDelete, path, b)
	}
	return resp.StatusCode, nil
}

// hijack dials the socket directly for HTTP Upgrade (exec/attach).
func (c *client) hijack(ctx context.Context, path string, body string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("dial podman socket: %w", err)
	}

	versionedPath := "/" + c.apiVersion + path

	req := fmt.Sprintf(
		"POST %s HTTP/1.1\r\n"+
			"Host: localhost\r\n"+
			"Content-Type: application/json\r\n"+
			"Content-Length: %d\r\n"+
			"Connection: Upgrade\r\n"+
			"Upgrade: tcp\r\n"+
			"\r\n"+
			"%s",
		versionedPath, len(body), body,
	)

	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := io.WriteString(conn, req); err != nil {
		err := conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("hijack write: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		err := conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("hijack response: %w", err)
	}
	err2 := resp.Body.Close()
	if err2 != nil {
		return nil, err2
	}

	if resp.StatusCode != http.StatusSwitchingProtocols {
		err := conn.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("hijack: expected 101, got %d", resp.StatusCode)
	}

	_ = conn.SetDeadline(time.Time{})

	if br.Buffered() > 0 {
		buf := make([]byte, br.Buffered())
		_, _ = io.ReadFull(br, buf)
		return &prefixConn{Conn: conn, prefix: buf}, nil
	}
	return conn, nil
}

// prefixConn wraps net.Conn and replays buffered bytes first.
type prefixConn struct {
	net.Conn
	prefix []byte
	offset int
}

func (pc *prefixConn) Read(b []byte) (int, error) {
	if pc.offset < len(pc.prefix) {
		n := copy(b, pc.prefix[pc.offset:])
		pc.offset += n
		return n, nil
	}
	return pc.Conn.Read(b)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func escPath(s string) string { return url.PathEscape(s) }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func filterQuery(filters map[string][]string) string {
	b, _ := json.Marshal(filters)
	return string(b)
}

func stripSHA(id string) string {
	id = strings.TrimPrefix(id, "sha256:")
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// truncate shortens a byte slice for use in error messages.
func truncate(b []byte, max int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
