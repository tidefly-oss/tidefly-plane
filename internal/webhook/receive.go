package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/tidefly-oss/tidefly-plane/internal/models"
	"github.com/tidefly-oss/tidefly-plane/internal/queue"
)

func (h *Handler) Receive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	writeJSON := func(status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}

	wh, err := h.store.LoadActive(ctx, id)
	if err != nil {
		writeJSON(http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	rawSecret, err := h.svc.DecryptSecret(wh.Secret)
	if err != nil {
		h.log.Error("webhook", "secret decrypt failed", err)
		writeJSON(http.StatusOK, map[string]string{"status": "ok"})
		return
	}

	provider := Provider(wh.Provider)
	payload, err := VerifyAndParse(r, provider, rawSecret)
	if err != nil {
		h.log.Warn("webhook", "signature invalid",
			fmt.Sprintf("webhook_id=%s err=%s", id, err.Error()))
		writeJSON(http.StatusUnauthorized, map[string]string{"error": "invalid signature"})
		return
	}

	if payload.IsPing() {
		writeJSON(http.StatusOK, map[string]string{"status": "pong"})
		return
	}

	if !MatchesBranch(wh.Branch, payload.Branch) {
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
	h.store.CreateDelivery(ctx, &delivery)

	qp := queue.WebhookPayload{
		Provider:  string(payload.Provider),
		EventType: payload.EventType,
		Branch:    payload.Branch,
		Tag:       payload.Tag,
		Commit:    payload.Commit,
		CommitMsg: payload.CommitMsg,
		PushedBy:  payload.PushedBy,
		RepoURL:   payload.RepoURL,
		RepoName:  payload.RepoName,
	}

	if err := queue.EnqueueWebhookDeploy(h.queue, wh.ID, delivery.ID, qp); err != nil {
		h.store.UpdateDelivery(ctx, &delivery, map[string]any{
			"status":    models.WebhookStatusFailed,
			"error_msg": err.Error(),
		})
		writeJSON(http.StatusInternalServerError, map[string]string{"error": "enqueue failed"})
		return
	}

	h.store.UpdateLastTriggered(ctx, wh, models.WebhookStatusPending)
	writeJSON(http.StatusAccepted, map[string]string{"status": "accepted", "delivery_id": delivery.ID})
}
