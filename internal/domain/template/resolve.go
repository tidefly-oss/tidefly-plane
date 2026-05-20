package template

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// ResolveResult holds the resolved manifest JSON and any generated credentials.
type ResolveResult struct {
	// ManifestJSON is the fully resolved ServiceManifest JSON ready to deploy.
	ManifestJSON string

	// Credentials holds generated plaintext values for credential fields.
	// Only populated for fields with generated=true + show_plaintext_once=true.
	// Key = field key, value = plaintext credential.
	Credentials map[string]string
}

// Resolve applies field values to the template manifest, generates credentials,
// and returns the resolved manifest JSON.
//
// vars contains user-provided field values (key → value).
// version is the selected template version (replaces {version}).
func (t *Template) Resolve(vars map[string]string, version string) (*ResolveResult, error) {
	if len(t.Manifest) == 0 {
		return nil, fmt.Errorf("template %q has no manifest — use legacy YAML container format", t.Slug)
	}

	// ── 1. Generate credentials ───────────────────────────────────────────────
	credentials := make(map[string]string)
	allVars := make(map[string]string, len(vars))
	for k, v := range vars {
		allVars[k] = v
	}

	for _, f := range t.Fields {
		if f.Type != "credential" {
			continue
		}
		if val, ok := allVars[f.Key]; ok && val != "" {
			// User provided a value — use it
			continue
		}
		if f.Generated {
			cred, err := generateCredential()
			if err != nil {
				return nil, fmt.Errorf("generate credential for %q: %w", f.Key, err)
			}
			allVars[f.Key] = cred
			if f.ShowPlaintextOnce {
				credentials[f.Key] = cred
			}
		}
	}

	// ── 2. Set version ────────────────────────────────────────────────────────
	if version == "" {
		version = t.DefaultVersion
	}
	allVars["version"] = version

	// ── 3. Interpolate manifest JSON ──────────────────────────────────────────
	raw := string(t.Manifest)
	raw = interpolateString(raw, allVars)

	// ── 4. Validate it's still valid JSON ─────────────────────────────────────
	var check map[string]any
	if err := json.Unmarshal([]byte(raw), &check); err != nil {
		return nil, fmt.Errorf("resolved manifest is invalid JSON: %w", err)
	}

	return &ResolveResult{
		ManifestJSON: raw,
		Credentials:  credentials,
	}, nil
}

// interpolateString replaces all {key} placeholders with values from vars.
func interpolateString(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// generateCredential creates a random 32-byte hex credential.
func generateCredential() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
