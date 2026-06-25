package jobs

// update_check_worker.go — River worker that triggers OCI digest polling.
// The actual digest fetching and drift detection lives in the Reconciler,
// keeping all desired↔actual logic in one place.

import (
	"context"

	"github.com/riverqueue/river"
	"github.com/tidefly-oss/tidefly-plane/internal/reconciler"
)

type UpdateCheckArgs struct{}

func (UpdateCheckArgs) Kind() string { return "system:update_check" }

type UpdateCheckWorker struct {
	river.WorkerDefaults[UpdateCheckArgs]
	rec *reconciler.Reconciler
}

func NewUpdateCheckWorker(rec *reconciler.Reconciler) *UpdateCheckWorker {
	return &UpdateCheckWorker{rec: rec}
}

func (w *UpdateCheckWorker) Work(ctx context.Context, _ *river.Job[UpdateCheckArgs]) error {
	w.rec.PollDigests(ctx)
	return nil
}
