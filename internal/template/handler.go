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

// ── List ──────────────────────────────────────────────────────────────────────

type listTemplatesInput struct {
	Category string `query:"category" doc:"Filter by category"`
	Tag      string `query:"tag"      doc:"Filter by tag"`
}

type listTemplatesOutput struct {
	Body []Summary
}

func (h *Handler) listTemplates(_ context.Context, input *listTemplatesInput) (*listTemplatesOutput, error) {
	var (
		summaries []Summary
		err       error
	)

	switch {
	case input.Category != "":
		summaries, err = h.loader.ListByCategory(input.Category)
	case input.Tag != "":
		summaries, err = h.loader.ListByTag(input.Tag)
	default:
		summaries, err = h.loader.List()
	}

	if err != nil {
		return nil, huma.Error503ServiceUnavailable("templates unavailable: " + err.Error())
	}
	if summaries == nil {
		summaries = []Summary{}
	}
	return &listTemplatesOutput{Body: summaries}, nil
}

// ── Get ───────────────────────────────────────────────────────────────────────

type getTemplateInput struct {
	Slug string `path:"slug"`
}

type getTemplateOutput struct {
	Body *Template
}

func (h *Handler) getTemplate(_ context.Context, input *getTemplateInput) (*getTemplateOutput, error) {
	tmpl, err := h.loader.Get(input.Slug)
	if err != nil {
		return nil, huma.Error404NotFound("template not found")
	}
	return &getTemplateOutput{Body: tmpl}, nil
}
