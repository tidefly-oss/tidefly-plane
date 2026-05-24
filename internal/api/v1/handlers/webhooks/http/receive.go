package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/tidefly-oss/tidefly-plane/internal/domain/webhook"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/jobs"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
)

func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	writeJSON := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	wh, err := h.webhooks.LoadActive(ctx, id)
	if err != nil {
		writeJSON(http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	rawSecret, err := h.svc.DecryptSecret(wh.Secret)
	if err != nil {
		h.log.Error("webhooks", "secret decrypt failed", err)
		writeJSON(http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	provider := webhook.Provider(wh.Provider)
	payload, err := webhook.VerifyAndParse(r, provider, rawSecret)
	if err != nil {
		h.log.Warn("webhooks", "signature invalid",
			fmt.Sprintf("webhook_id=%s err=%s", id, err.Error()))
		writeJSON(http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	if payload.IsPing() {
		writeJSON(http.StatusOK, map[string]string{"status": "pong"})
		return
	}

	if !webhook.MatchesBranch(wh.Branch, payload.Branch) {
		writeJSON(http.StatusOK, map[string]string{"status": "skipped", "reason": "branch filter"})
		return
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
		h.webhooks.UpdateDelivery(ctx, &delivery, map[string]any{
			"status":    models.WebhookStatusFailed,
			"error_msg": err.Error(),
		})
		writeJSON(http.StatusInternalServerError, map[string]string{"error": "enqueue failed"})
		return
	}

	h.webhooks.UpdateLastTriggered(ctx, wh, models.WebhookStatusPending)
	writeJSON(http.StatusAccepted, map[string]string{"status": "accepted", "delivery_id": delivery.ID})
}
