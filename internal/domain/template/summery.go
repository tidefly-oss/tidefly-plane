package template

// Summary is the lightweight version for listing — no fields, no containers.
type Summary struct {
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Icon           string   `json:"icon"`
	Description    string   `json:"description"`
	Versions       []string `json:"versions"`
	DefaultVersion string   `json:"default_version"`
}

func (t *Template) ToSummary() Summary {
	return Summary{
		Slug:           t.Slug,
		Name:           t.Name,
		Category:       t.Category,
		Icon:           t.Icon,
		Description:    t.Description,
		Versions:       t.Versions,
		DefaultVersion: t.DefaultVersion,
	}
}
