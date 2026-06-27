package template

// Summary is the lightweight version returned by GET /templates.
// Sourced from index.json — no manifest, no fields.
type Summary struct {
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Icon           string   `json:"icon"`
	Description    string   `json:"description"`
	Tags           []string `json:"tags,omitempty"`
	Versions       []string `json:"versions"`
	DefaultVersion string   `json:"default_version"`
	DocsURL        string   `json:"docs_url,omitempty"`
	MinTidefly     string   `json:"min_tidefly,omitempty"`
	Official       bool     `json:"official"`
}

func (t *Template) ToSummary() Summary {
	return Summary{
		Slug:           t.Slug,
		Name:           t.Name,
		Category:       t.Category,
		Icon:           t.Icon,
		Description:    t.Description,
		Tags:           t.Tags,
		Versions:       t.Versions,
		DefaultVersion: t.DefaultVersion,
		DocsURL:        t.DocsURL,
		MinTidefly:     t.MinTidefly,
		Official:       t.Official,
	}
}
