package http

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"github.com/tidefly-oss/tidefly-plane/internal/services/template"
)

type Handler struct {
	loader *template.Loader
}

func New(loader *template.Loader) *Handler {
	return &Handler{loader: loader}
}

type ListTemplatesInput struct{}
type ListTemplatesOutput struct {
	Body []template.Summary
}

type GetTemplateInput struct {
	Slug string `path:"slug"`
}
type GetTemplateOutput struct {
	Body *template.Template
}

func (h *Handler) ListTemplates(_ context.Context, _ *ListTemplatesInput) (*ListTemplatesOutput, error) {
	list := h.loader.List()
	summaries := make([]template.Summary, 0, len(list))
	for _, t := range list {
		summaries = append(summaries, t.ToSummary())
	}
	return &ListTemplatesOutput{Body: summaries}, nil
}

func (h *Handler) GetTemplate(_ context.Context, input *GetTemplateInput) (*GetTemplateOutput, error) {
	tmpl, err := h.loader.Get(input.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("template not found")
	}
	return &GetTemplateOutput{Body: tmpl}, nil
}
