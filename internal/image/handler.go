package image

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/access"
	"github.com/tidefly-oss/tidefly-plane/internal/infra/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
)

type Handler struct {
	runtime runtime.Runtime
	bus     *eventbus.Bus
}

func NewHandler(rt runtime.Runtime, bus *eventbus.Bus) *Handler {
	return &Handler{runtime: rt, bus: bus}
}

type imageContainerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listOutput struct {
	Body []runtime.Image
}

type deleteInput struct {
	ID    string `path:"id"`
	Force bool   `query:"force"`
}

type containersInput struct {
	ID string `path:"id"`
}

type containersOutput struct {
	Body []imageContainerRef
}

// list returns all non-internal images.
// Internal filtering is done inside runtime.ListImages via container labels.
func (h *Handler) list(ctx context.Context, _ *struct{}) (*listOutput, error) {
	list, err := h.runtime.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	return &listOutput{Body: list}, nil
}

func (h *Handler) delete(ctx context.Context, input *deleteInput) (*struct{}, error) {
	if err := h.runtime.DeleteImage(ctx, input.ID, input.Force); err != nil {
		return nil, fmt.Errorf("delete image: %w", err)
	}
	h.bus.Publish(eventbus.Event{
		Type:    eventbus.EventImageDeleted,
		Topic:   eventbus.TopicImages,
		Payload: eventbus.ImageDeletedPayload{ID: input.ID},
	})
	return nil, nil
}

func (h *Handler) containers(ctx context.Context, input *containersInput) (*containersOutput, error) {
	images, err := h.runtime.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	var matchedTags []string
outer:
	for _, img := range images {
		if img.ID == input.ID || strings.HasPrefix(img.ID, input.ID) {
			matchedTags = img.Tags
			break
		}
		for _, tag := range img.Tags {
			if tag == input.ID {
				matchedTags = img.Tags
				break outer
			}
		}
	}
	if len(matchedTags) == 0 {
		return &containersOutput{Body: []imageContainerRef{}}, nil
	}
	cs, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	refs := make([]imageContainerRef, 0)
	for _, ct := range cs {
		if access.IsInternal(ct.Labels) {
			continue
		}
		for _, tag := range matchedTags {
			if ct.Image == tag || strings.HasPrefix(ct.Image, tag) {
				refs = append(refs, imageContainerRef{ID: ct.ID, Name: ct.Name})
				break
			}
		}
	}
	return &containersOutput{Body: refs}, nil
}
