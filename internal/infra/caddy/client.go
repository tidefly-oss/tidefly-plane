package caddy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/platform/config"
)

const (
	caddyHandlerKey   = "handler"
	caddyKeyID        = "@id"
	caddyKeyHandle    = "handle"
	caddyKeyDial      = "dial"
	caddyKeyUpstreams = "upstreams"
)

// Client speaks to the Caddy Admin API.
// All routing is configured via API — no Caddyfile needed.
type Client struct {
	adminURL string
	cfg      config.CaddyConfig
	http     *http.Client
}

func New(cfg config.CaddyConfig) *Client {
	return &Client{
		adminURL: cfg.AdminURL,
		cfg:      cfg,
		http:     &http.Client{},
	}
}

// Config returns the CaddyConfig this client was initialized with.
func (c *Client) Config() config.CaddyConfig {
	return c.cfg
}

// ── Route management ──────────────────────────────────────────────────────────

// AddHTTPRoute registers an HTTP(S) route for a deployed container.
func (c *Client) AddHTTPRoute(ctx context.Context, routeID, host, upstream string) error {
	route := map[string]any{
		caddyKeyID: routeID,
		"match": []map[string]any{
			{"host": []string{host}},
		},
		caddyKeyHandle: []map[string]any{
			{
				caddyHandlerKey:   "reverse_proxy",
				caddyKeyUpstreams: []map[string]string{{caddyKeyDial: upstream}},
			},
		},
		"terminal": true,
	}

	if err := c.patch(ctx, fmt.Sprintf("/id/%s", routeID), route); err == nil {
		return nil
	}

	count, err := c.routeCount(ctx)
	if err != nil {
		count = 0
	}

	return c.put(ctx, fmt.Sprintf("/config/apps/http/servers/tidefly-plane/routes/%d", count), route)
}

// RemoveRoute removes a route by its @id.
func (c *Client) RemoveRoute(ctx context.Context, routeID string) error {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodDelete,
		fmt.Sprintf("%s/id/%s", c.adminURL, routeID),
		nil,
	)
	if err != nil {
		return fmt.Errorf("caddy remove route: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("caddy remove route: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("caddy remove route: status %d", resp.StatusCode)
	}
	return nil
}

// UpdateRoute updates the upstream for an existing route.
func (c *Client) UpdateRoute(ctx context.Context, routeID, host, upstream string) error {
	_ = c.RemoveRoute(ctx, routeID)
	return c.AddHTTPRoute(ctx, routeID, host, upstream)
}

// ── TLS ───────────────────────────────────────────────────────────────────────

func (c *Client) ConfigureTLS(ctx context.Context) error {
	tlsConfig := map[string]any{
		"automation": map[string]any{
			"policies": []map[string]any{
				{
					"issuers": []map[string]any{
						{
							"module": "acme",
							"email":  c.cfg.ACMEEmail,
							"ca":     acmeCA(c.cfg.ACMEStaging),
						},
					},
				},
			},
		},
	}
	return c.post(ctx, "/config/apps/tls", tlsConfig)
}

func (c *Client) ConfigureInternalTLS(ctx context.Context) error {
	tlsConfig := map[string]any{
		"automation": map[string]any{
			"policies": []map[string]any{
				{
					"subjects": []string{"*.tidefly-plane.internal"},
					"issuers": []map[string]any{
						{"module": "internal"},
					},
				},
			},
		},
	}
	return c.post(ctx, "/config/apps/tls", tlsConfig)
}

// ── Bootstrap ─────────────────────────────────────────────────────────────────

func (c *Client) Bootstrap(ctx context.Context) error {
	serverConfig := map[string]any{
		"apps": map[string]any{
			"http": map[string]any{
				"servers": map[string]any{
					"tidefly-plane": map[string]any{
						"listen": []string{":80", ":443"},
						"routes": []any{},
						"automatic_https": map[string]any{
							"disable": !c.cfg.ForceHTTPS,
						},
						"logs": map[string]any{
							"default_logger_name": "tidefly_access",
						},
					},
				},
			},
		},
	}

	if err := c.patch(ctx, "/config/", serverConfig); err != nil {
		return fmt.Errorf("caddy bootstrap: %w", err)
	}

	if c.cfg.InternalTLS {
		if err := c.ConfigureInternalTLS(ctx); err != nil {
			return fmt.Errorf("caddy internal tls: %w", err)
		}
	}

	if c.cfg.ACMEEmail != "" {
		if err := c.ConfigureTLS(ctx); err != nil {
			return fmt.Errorf("caddy acme tls: %w", err)
		}
	}

	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func RouteID(containerName string) string {
	return "tidefly-plane-" + sanitizeName(containerName)
}

func Domain(cfg config.CaddyConfig, name string) string {
	if !cfg.Enabled || cfg.BaseDomain == "" {
		return ""
	}
	return fmt.Sprintf("%s.%s", sanitizeName(name), cfg.BaseDomain)
}

var nonAlphanumDash = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = nonAlphanumDash.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

func acmeCA(staging bool) string {
	if staging {
		return "https://acme-staging-v02.api.letsencrypt.org/directory"
	}
	return "https://acme-v02.api.letsencrypt.org/directory"
}

func (c *Client) post(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("caddy marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.adminURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("caddy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("caddy post: %w", err)
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("caddy post %s: status %d", path, resp.StatusCode)
	}
	return nil
}

func (c *Client) put(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("caddy marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.adminURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("caddy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("caddy put: %w", err)
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("caddy put %s: status %d", path, resp.StatusCode)
	}
	return nil
}

func (c *Client) patch(ctx context.Context, path string, body any) error {
	data, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.adminURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("caddy patch %s: status %d", path, resp.StatusCode)
	}
	return nil
}

func (c *Client) routeCount(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		c.adminURL+"/config/apps/http/servers/tidefly-plane/routes", nil,
	)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(resp.Body)
	var routes []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&routes); err != nil {
		return 0, err
	}
	return len(routes), nil
}

func (c *Client) SetBaseDomain(domain string) {
	c.cfg.BaseDomain = domain
}

// upsertRoute tries PATCH first, falls back to PUT at the next route index.
func (c *Client) upsertRoute(ctx context.Context, id string, route map[string]any) error {
	if err := c.patch(ctx, "/id/"+id, route); err == nil {
		return nil
	}
	count, _ := c.routeCount(ctx)
	if err := c.put(ctx, fmt.Sprintf("/config/apps/http/servers/tidefly-plane/routes/%d", count), route); err != nil {
		return fmt.Errorf("upsert route %s: %w", id, err)
	}
	return nil
}

func (c *Client) RegisterDashboard(ctx context.Context) error {
	if !c.cfg.Enabled || c.cfg.BaseDomain == "" {
		return nil
	}

	host := "dashboard." + c.cfg.BaseDomain

	// API Route — /api/* → Backend
	apiRoute := map[string]any{
		caddyKeyID: "tidefly-plane-dashboard-api",
		"match": []map[string]any{
			{
				"host": []string{host},
				"path": []string{"/api/*"},
			},
		},
		caddyKeyHandle: []map[string]any{
			{
				caddyHandlerKey:   "reverse_proxy",
				caddyKeyUpstreams: []map[string]string{{caddyKeyDial: "tidefly_backend:8181"}},
			},
		},
		"terminal": true,
	}

	// Docs Route — /docs* + /openapi* → Backend
	docsRoute := map[string]any{
		caddyKeyID: "tidefly-plane-dashboard-docs",
		"match": []map[string]any{
			{
				"host": []string{host},
				"path": []string{"/docs*", "/openapi*"},
			},
		},
		caddyKeyHandle: []map[string]any{
			{
				caddyHandlerKey:   "reverse_proxy",
				caddyKeyUpstreams: []map[string]string{{caddyKeyDial: "tidefly_backend:8181"}},
			},
		},
		"terminal": true,
	}

	// UI Route — /* → UI
	uiRoute := map[string]any{
		caddyKeyID: "tidefly-plane-dashboard",
		"match": []map[string]any{
			{"host": []string{host}},
		},
		caddyKeyHandle: []map[string]any{
			{
				caddyHandlerKey:   "reverse_proxy",
				caddyKeyUpstreams: []map[string]string{{caddyKeyDial: "tidefly_ui:3000"}},
			},
		},
		"terminal": true,
	}

	if err := c.upsertRoute(ctx, "tidefly-plane-dashboard-api", apiRoute); err != nil {
		return fmt.Errorf("dashboard api route: %w", err)
	}
	if err := c.upsertRoute(ctx, "tidefly-plane-dashboard-docs", docsRoute); err != nil {
		return fmt.Errorf("dashboard docs route: %w", err)
	}
	if err := c.upsertRoute(ctx, "tidefly-plane-dashboard", uiRoute); err != nil {
		return fmt.Errorf("dashboard ui route: %w", err)
	}

	return nil
}

// RemoveDashboard removes all dashboard routes from Caddy.
func (c *Client) RemoveDashboard(ctx context.Context) {
	_ = c.RemoveRoute(ctx, "tidefly-plane-dashboard-api")
	_ = c.RemoveRoute(ctx, "tidefly-plane-dashboard-docs")
	_ = c.RemoveRoute(ctx, "tidefly-plane-dashboard")
}
