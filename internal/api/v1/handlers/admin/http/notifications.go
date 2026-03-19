package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type TestNotificationInput struct {
	Channel string `path:"channel" doc:"Channel to test: slack, discord, email"`
}

func (h *Handler) TestNotification(ctx context.Context, input *TestNotificationInput) (*struct{}, error) {
	if err := h.notifier.Test(ctx, input.Channel); err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}
	return nil, nil
}
