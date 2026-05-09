package http

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	caddysvc "github.com/tidefly-oss/tidefly-plane/internal/infrastructure/caddy"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/logger"
)

type BuildAndDeployRequest struct {
	Dockerfile       string   `json:"dockerfile"`
	Name             string   `json:"name"`
	Tag              string   `json:"tag"`
	Port             int      `json:"port"`
	Expose           bool     `json:"expose"`
	CustomDomain     string   `json:"custom_domain"`
	Restart          string   `json:"restart"`
	Env              []string `json:"env"`
	ProjectID        string   `json:"project_id"`
	RepoURL          string   `json:"repo_url"`
	Branch           string   `json:"branch"`
	GitIntegrationID string   `json:"git_integration_id"`
	DockerfilePath   string   `json:"dockerfile_path"`
}

func (h *Handler) BuildAndDeploy(c *echo.Context) error {
	var req BuildAndDeployRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.Name == "" || req.ProjectID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "name and project_id are required"})
	}
	if req.Dockerfile == "" && req.RepoURL == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "dockerfile or repo_url is required"})
	}
	if req.Tag == "" {
		req.Tag = fmt.Sprintf("localhost/tidefly-plane/%s:latest", req.Name)
	}
	req.Tag = qualifyLocalTag(req.Tag)
	if req.Restart == "" {
		req.Restart = "unless-stopped"
	}
	if req.DockerfilePath == "" {
		req.DockerfilePath = "Dockerfile"
	}
	if req.Branch == "" {
		req.Branch = "main"
	}
	if req.Expose && req.Port == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "port is required when expose=true"})
	}
	if req.Expose && !h.CaddyEnabled() {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Caddy integration is not enabled on this instance"})
	}

	project, err := h.projects.GetByID(req.ProjectID)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "project not found"})
	}

	resp := c.Response()
	resp.Header().Set("Content-Type", "text/event-stream")
	resp.Header().Set("Cache-Control", "no-cache")
	resp.Header().Set("Connection", "keep-alive")
	resp.Header().Set("X-Accel-Buffering", "no")
	resp.WriteHeader(http.StatusOK)
	flusher, ok := resp.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	ctx := c.Request().Context()
	sendEvent := func(event, msg string) {
		_, err := fmt.Fprintf(resp, "event: %s\ndata: %s\n\n", event, msg)
		if err != nil {
			return
		}
		flusher.Flush()
	}

	sendEvent("status", `{"message":"Starting build..."}`)

	buildOutput, err := h.prepareBuildOutput(ctx, req, sendEvent)
	if err != nil || buildOutput == nil {
		return nil
	}
	defer func() {
		if err := buildOutput.Close(); err != nil {
			h.log.Error("streams", "failed to close build output", err)
		}
	}()

	if !h.streamBuildOutput(ctx, buildOutput, req, sendEvent) {
		return nil
	}

	inspect, err := h.runtime.InspectImage(ctx, req.Tag)
	if err != nil {
		sendEvent("error", `{"message":"Image build failed - image not found after build"}`)
		return nil
	}
	if len(inspect.Cmd) == 0 && len(inspect.Entrypoint) == 0 {
		sendEvent("error", `{"message":"Built image has no CMD or Entrypoint. If your Dockerfile uses COPY, a build context (e.g. Git repository) is required."}`)
		return nil
	}

	sendEvent("status", `{"message":"Build complete. Starting container..."}`)

	stackID, err := h.startContainer(ctx, req, project.NetworkName, sendEvent)
	if err != nil || stackID == "" {
		return nil
	}

	publicDomain := ""
	if req.Expose && h.CaddyEnabled() {
		publicDomain = h.registerCaddyRoute(ctx, req)
	}

	h.log.Audit(ctx, logger.AuditEntry{
		Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: true,
		Details: fmt.Sprintf("name=%q image=%q project=%s expose=%v domain=%s", req.Name, req.Tag, req.ProjectID, req.Expose, publicDomain),
	})

	donePayload := map[string]string{"message": "Container started successfully", "stack_id": stackID, "name": req.Name}
	if publicDomain != "" {
		donePayload["url"] = "https://" + publicDomain
	}
	data, _ := json.Marshal(donePayload)
	sendEvent("done", string(data))
	return nil
}

func (h *Handler) prepareBuildOutput(ctx context.Context, req BuildAndDeployRequest, sendEvent func(string, string)) (io.ReadCloser, error) {
	if req.RepoURL != "" {
		sendEvent("status", `{"message":"Cloning repository..."}`)
		tarBuf, err := h.buildContextFromGit(req)
		if err != nil {
			sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
			return nil, err
		}
		sendEvent("status", `{"message":"Building image from repository..."}`)
		out, err := h.runtime.BuildImageFromContext(ctx, req.Tag, req.DockerfilePath, tarBuf)
		if err != nil {
			sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
			return nil, err
		}
		return out, nil
	}
	out, err := h.runtime.BuildImage(ctx, req.Tag, req.Dockerfile)
	if err != nil {
		h.log.Audit(ctx, logger.AuditEntry{
			Action: logger.AuditContainerDeploy, Success: false,
			Details: fmt.Sprintf("dockerfile build image failed name=%q project=%s err=%s", req.Name, req.ProjectID, err),
		})
		sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		return nil, err
	}
	return out, nil
}

func (h *Handler) streamBuildOutput(ctx context.Context, buildOutput io.ReadCloser, req BuildAndDeployRequest, sendEvent func(string, string)) bool {
	type buildLine struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
	}
	scanner := bufio.NewScanner(buildOutput)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		var line buildLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Error != "" {
			h.log.Audit(ctx, logger.AuditEntry{
				Action: logger.AuditContainerDeploy, Success: false,
				Details: fmt.Sprintf("build error name=%q project=%s err=%s", req.Name, req.ProjectID, line.Error),
			})
			data, _ := json.Marshal(map[string]string{"message": line.Error})
			sendEvent("error", string(data))
			return false
		}
		if line.Stream != "" {
			data, _ := json.Marshal(map[string]string{"message": line.Stream})
			sendEvent("build", string(data))
		}
	}
	return true
}

func (h *Handler) startContainer(ctx context.Context, req BuildAndDeployRequest, networkName string, sendEvent func(string, string)) (string, error) {
	stackID := uuid.New().String()
	labels := map[string]string{
		"tidefly-plane.managed":  "true",
		"tidefly-plane.stack_id": stackID,
		"tidefly-plane.source":   "dockerfile",
		"tidefly-plane.project":  req.ProjectID,
	}
	spec := runtime.ContainerSpec{
		Name: req.Name, Image: req.Tag, Env: req.Env,
		Labels: labels, Restart: req.Restart, Network: networkName,
	}
	if req.Port > 0 && !req.Expose {
		spec.Ports = []runtime.PortBinding{
			{HostPort: fmt.Sprintf("%d", req.Port), ContainerPort: req.Port, Protocol: "tcp"},
		}
	}
	if err := h.runtime.CreateContainer(ctx, spec); err != nil {
		h.log.Audit(ctx, logger.AuditEntry{
			Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: false,
			Details: fmt.Sprintf("create container name=%q project=%s err=%s", req.Name, req.ProjectID, err),
		})
		sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		return "", err
	}
	if err := h.runtime.StartContainer(ctx, req.Name); err != nil {
		h.log.Audit(ctx, logger.AuditEntry{
			Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: false,
			Details: fmt.Sprintf("start container name=%q project=%s err=%s", req.Name, req.ProjectID, err),
		})
		sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		return "", err
	}
	if req.Expose {
		if err := h.runtime.ConnectNetwork(ctx, req.Name, "tidefly_proxy"); err != nil {
			h.log.Warn("caddy", "failed to connect container to internal network", err)
		}
	}
	return stackID, nil
}

func (h *Handler) registerCaddyRoute(ctx context.Context, req BuildAndDeployRequest) string {
	domain := req.CustomDomain
	if domain == "" {
		domain = caddysvc.Domain(h.caddy.Config(), req.Name)
	}
	upstream := fmt.Sprintf("%s:%d", req.Name, req.Port)
	routeID := caddysvc.RouteID(req.Name)
	h.log.Info("caddy", fmt.Sprintf("expose=%v caddy_enabled=%v port=%d", req.Expose, h.CaddyEnabled(), req.Port))
	if err := h.caddy.AddHTTPRoute(ctx, routeID, domain, upstream); err != nil {
		h.log.Error("caddy", "failed to register route", err)
		return ""
	}
	return domain
}

func qualifyLocalTag(tag string) string {
	parts := strings.SplitN(tag, "/", 2)
	if strings.Contains(parts[0], ".") || parts[0] == "localhost" {
		return tag
	}
	return "localhost/" + tag
}
