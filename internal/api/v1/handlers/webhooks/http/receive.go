package http

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/tidefly-oss/tidefly-backend/internal/jobs"
	"github.com/tidefly-oss/tidefly-backend/internal/models"
	"github.com/tidefly-oss/tidefly-backend/internal/services/webhook"
)

func (h *Handler) Receive(c *echo.Context) error {
	id := c.Param("id")
	ctx := c.Request().Context()

	wh, err := h.webhooks.LoadActive(ctx, id)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	rawSecret, err := h.svc.DecryptSecret(wh.Secret)
	if err != nil {
		h.log.Error("webhooks", "secret decrypt failed", err)
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	provider := webhook.Provider(wh.Provider)
	payload, err := webhook.VerifyAndParse(c.Request(), provider, rawSecret)
	if err != nil {
		h.log.Warn(
			"webhooks", "signature invalid",
			fmt.Sprintf("webhook_id=%s err=%s", id, err.Error()),
		)
		return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
	}

	if payload.IsPing() {
		return c.JSON(http.StatusOK, map[string]string{"status": "pong"})
	}
	if !webhook.MatchesBranch(wh.Branch, payload.Branch) {
		return c.JSON(http.StatusOK, map[string]string{"status": "skipped", "reason": "branch filter"})
	}

	delivery := models.WebhookDelivery{
		ID: uuid.New().String(), WebhookID: wh.ID,
		Provider: string(provider), EventType: payload.EventType,
		Branch: payload.Branch, Commit: payload.Commit,
		CommitMsg: payload.CommitMsg, PushedBy: payload.PushedBy,
		RepoURL: payload.RepoURL, Status: models.WebhookStatusPending,
	}
	h.webhooks.CreateDelivery(ctx, &delivery)

	if err := jobs.EnqueueWebhookDeploy(h.queue, wh.ID, delivery.ID, *payload); err != nil {
		h.webhooks.UpdateDelivery(
			ctx, &delivery, map[string]any{
				"status":    models.WebhookStatusFailed,
				"error_msg": err.Error(),
			},
		)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "enqueue failed"})
	}

	h.webhooks.UpdateLastTriggered(ctx, wh, models.WebhookStatusPending)
	return c.JSON(http.StatusAccepted, map[string]string{"status": "accepted", "delivery_id": delivery.ID})
}
