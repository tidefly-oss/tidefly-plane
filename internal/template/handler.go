package template

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type Handler struct {
	loader *Loader
}

func NewHandler(loader *Loader) *Handler {
	return &Handler{loader: loader}
}

type listTemplatesOutput struct {
	Body []Summary
}

type getTemplateInput struct {
	Slug string `path:"slug"`
}

type getTemplateOutput struct {
	Body *Template
}

func (h *Handler) listTemplates(_ context.Context, _ *struct{}) (*listTemplatesOutput, error) {
	list, err := h.loader.List()
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("templates unavailable: " + err.Error())
	}
	summaries := make([]Summary, 0, len(list))
	for _, t := range list {
		summaries = append(summaries, t.ToSummary())
	}
	return &listTemplatesOutput{Body: summaries}, nil
}

func (h *Handler) getTemplate(_ context.Context, input *getTemplateInput) (*getTemplateOutput, error) {
	tmpl, err := h.loader.Get(input.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("template not found")
	}
	return &getTemplateOutput{Body: tmpl}, nil
}
