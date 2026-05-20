package http

import (
	"context"
	"fmt"
	"strings"

	"github.com/tidefly-oss/tidefly-plane/internal/api/v1/handlers/images/helpers"
	"github.com/tidefly-oss/tidefly-plane/internal/infrastructure/runtime"
	"github.com/tidefly-oss/tidefly-plane/internal/platform/eventbus"
)

type Handler struct {
	runtime runtime.Runtime
	bus     *eventbus.Bus
}

func New(rt runtime.Runtime, bus *eventbus.Bus) *Handler {
	return &Handler{runtime: rt, bus: bus}
}

type ImageContainerRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ListInput struct{}
type ListOutput struct {
	Body []runtime.Image
}

type DeleteInput struct {
	ID    string `path:"id"`
	Force bool   `query:"force"`
}

type ContainersInput struct {
	ID string `path:"id"`
}
type ContainersOutput struct {
	Body []ImageContainerRef
}

func (h *Handler) List(ctx context.Context, _ *ListInput) (*ListOutput, error) {
	list, err := h.runtime.ListImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	internalImages := map[string]bool{}
	if containers, err := h.runtime.ListContainers(ctx, true); err == nil {
		for _, ct := range containers {
			if ct.Labels["tidefly.internal"] == "true" && ct.Image != "" {
				internalImages[ct.Image] = true
			}
		}
	}
	result := make([]runtime.Image, 0, len(list))
	for _, img := range list {
		if !helpers.HasRealTag(img.Tags) || helpers.IsInternalImage(img.Tags, internalImages) {
			continue
		}
		result = append(result, img)
	}
	return &ListOutput{Body: result}, nil
}

func (h *Handler) Delete(ctx context.Context, input *DeleteInput) (*struct{}, error) {
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

func (h *Handler) Containers(ctx context.Context, input *ContainersInput) (*ContainersOutput, error) {
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
		return &ContainersOutput{Body: []ImageContainerRef{}}, nil
	}
	containers, err := h.runtime.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list containers: %w", err)
	}
	refs := make([]ImageContainerRef, 0)
	for _, ct := range containers {
		if ct.Labels["tidefly.internal"] == "true" {
			continue
		}
		for _, tag := range matchedTags {
			if ct.Image == tag || strings.HasPrefix(ct.Image, tag) {
				refs = append(refs, ImageContainerRef{ID: ct.ID, Name: ct.Name})
				break
			}
		}
	}
	return &ContainersOutput{Body: refs}, nil
}
