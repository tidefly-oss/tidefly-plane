package containers

// BuildAndDeploy bleibt auf rohem Echo weil es SSE-Streaming verwendet.
// Huma kann keine raw http.ResponseWriter SSE-Streams handeln.
// Einzige Änderung: AuditC → Audit mit ctx.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/logger"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/runtime"
	traefiksvc "github.com/tidefly-oss/tidefly-backend/internal/services/traefik"
)

type BuildAndDeployRequest struct {
	Dockerfile   string   `json:"dockerfile"`
	Name         string   `json:"name"`
	Tag          string   `json:"tag"`
	Port         int      `json:"port"`
	Expose       bool     `json:"expose"`
	CustomDomain string   `json:"custom_domain"`
	Restart      string   `json:"restart"`
	Env          []string `json:"env"`
	ProjectID    string   `json:"project_id"`
}

func (h *Handler) BuildAndDeploy(c *echo.Context) error {
	var req BuildAndDeployRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request"})
	}
	if req.Dockerfile == "" || req.Name == "" || req.ProjectID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "dockerfile, name and project_id are required"})
	}
	if req.Tag == "" {
		req.Tag = fmt.Sprintf("localhost/tidefly/%s:latest", req.Name)
	}
	req.Tag = qualifyLocalTag(req.Tag)
	if req.Restart == "" {
		req.Restart = "unless-stopped"
	}
	if req.Expose && req.Port == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "port is required when expose=true"})
	}
	if req.Expose && !h.traefik.Enabled {
		return c.JSON(
			http.StatusBadRequest, map[string]string{"error": "Traefik integration is not enabled on this instance"},
		)
	}

	var project models.Project
	if err := h.db.First(&project, "id = ?", req.ProjectID).Error; err != nil {
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

	buildOutput, err := h.runtime.BuildImage(ctx, req.Tag, req.Dockerfile)
	if err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditContainerDeploy, Success: false,
				Details: fmt.Sprintf(
					"dockerfile build image failed name=%q project=%s err=%s", req.Name, req.ProjectID, err,
				),
			},
		)
		sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		return nil
	}
	defer func(buildOutput io.ReadCloser) {
		err := buildOutput.Close()
		if err != nil {
			h.log.Error("failed to close build output", "error", err)
		}
	}(buildOutput)

	type buildLine struct {
		Stream string `json:"stream"`
		Error  string `json:"error"`
	}
	scanner := bufio.NewScanner(buildOutput)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		var line buildLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Error != "" {
			h.log.Audit(
				ctx, logger.AuditEntry{
					Action: logger.AuditContainerDeploy, Success: false,
					Details: fmt.Sprintf("build error name=%q project=%s err=%s", req.Name, req.ProjectID, line.Error),
				},
			)
			data, _ := json.Marshal(map[string]string{"message": line.Error})
			sendEvent("error", string(data))
			return nil
		}
		if line.Stream != "" {
			data, _ := json.Marshal(map[string]string{"message": line.Stream})
			sendEvent("build", string(data))
		}
	}

	sendEvent("status", `{"message":"Build complete. Starting container..."}`)
	stackID := uuid.New().String()
	labels := map[string]string{
		"tidefly.managed":  "true",
		"tidefly.stack_id": stackID,
		"tidefly.source":   "dockerfile",
		"tidefly.project":  req.ProjectID,
	}

	var publicDomain string
	if req.Expose {
		traefikLabels := traefiksvc.LabelsFor(
			*h.traefik, traefiksvc.ServiceConfig{
				Name: req.Name, Port: req.Port, CustomDomain: req.CustomDomain,
			},
		)
		traefiksvc.MergeLabels(labels, traefikLabels)
		if req.CustomDomain != "" {
			publicDomain = req.CustomDomain
		} else {
			publicDomain = traefiksvc.Domain(*h.traefik, req.Name)
		}
	}

	spec := runtime.ContainerSpec{
		Name: req.Name, Image: req.Tag, Env: req.Env,
		Labels: labels, Restart: req.Restart, Network: project.NetworkName,
	}
	if req.Port > 0 && !req.Expose {
		spec.Ports = []runtime.PortBinding{
			{HostPort: fmt.Sprintf("%d", req.Port), ContainerPort: req.Port, Protocol: "tcp"},
		}
	}

	if err := h.runtime.CreateContainer(ctx, spec); err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: false,
				Details: fmt.Sprintf("create container name=%q project=%s err=%s", req.Name, req.ProjectID, err),
			},
		)
		sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		return nil
	}
	if err := h.runtime.StartContainer(ctx, req.Name); err != nil {
		h.log.Audit(
			ctx, logger.AuditEntry{
				Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: false,
				Details: fmt.Sprintf("start container name=%q project=%s err=%s", req.Name, req.ProjectID, err),
			},
		)
		sendEvent("error", fmt.Sprintf(`{"message":%q}`, err.Error()))
		return nil
	}

	h.log.Audit(
		ctx, logger.AuditEntry{
			Action: logger.AuditContainerDeploy, ResourceID: stackID, Success: true,
			Details: fmt.Sprintf(
				"name=%q image=%q project=%s expose=%v domain=%s", req.Name, req.Tag, req.ProjectID, req.Expose,
				publicDomain,
			),
		},
	)

	donePayload := map[string]string{"message": "Container started successfully", "stack_id": stackID, "name": req.Name}
	if publicDomain != "" {
		donePayload["url"] = "https://" + publicDomain
	}
	data, _ := json.Marshal(donePayload)
	sendEvent("done", string(data))
	return nil
}

func qualifyLocalTag(tag string) string {
	parts := strings.SplitN(tag, "/", 2)
	if strings.Contains(parts[0], ".") || parts[0] == "localhost" {
		return tag
	}
	return "localhost/" + tag
}
