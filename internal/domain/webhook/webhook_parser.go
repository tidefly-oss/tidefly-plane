package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Provider string

const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderGitea     Provider = "gitea"
	ProviderBitbucket Provider = "bitbucket"
	ProviderGeneric   Provider = "generic"

	eventPush = "push"
	eventTag  = "tag"
	eventPing = "ping"
)

type Payload struct {
	Provider  Provider
	EventType string
	Branch    string
	Tag       string
	Commit    string
	CommitMsg string
	PushedBy  string
	RepoURL   string
	RepoName  string
	RawBody   []byte
}

func (p *Payload) IsPush() bool { return p.EventType == eventPush }
func (p *Payload) IsTag() bool  { return p.EventType == eventTag }
func (p *Payload) IsPing() bool { return p.EventType == eventPing }

func VerifyAndParse(r *http.Request, provider Provider, secret string) (*Payload, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 5<<20))
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}
	if secret != "" {
		if err := verifySignature(provider, r, body, secret); err != nil {
			return nil, err
		}
	}
	return parsePayload(provider, r, body)
}

func verifySignature(provider Provider, r *http.Request, body []byte, secret string) error {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))

	var got string
	switch provider {
	case ProviderGitHub, ProviderGitea:
		got = strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	case ProviderGitLab:
		if r.Header.Get("X-Gitlab-Token") != secret {
			return fmt.Errorf("invalid GitLab token")
		}
		return nil
	case ProviderBitbucket:
		got = strings.TrimPrefix(r.Header.Get("X-Hub-Signature"), "sha256=")
	default:
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			sig = r.Header.Get("X-Webhook-Signature")
		}
		got = strings.TrimPrefix(sig, "sha256=")
	}

	if got == "" {
		return fmt.Errorf("missing signature header")
	}
	if !hmac.Equal([]byte(got), []byte(expected)) {
		return fmt.Errorf("signature mismatch")
	}
	return nil
}

func parsePayload(provider Provider, r *http.Request, body []byte) (*Payload, error) {
	p := &Payload{Provider: provider, RawBody: body}
	switch provider {
	case ProviderGitHub:
		return parseGitHub(r, body, p)
	case ProviderGitLab:
		return parseGitLab(body, p)
	case ProviderGitea:
		return parseGitea(r, body, p)
	case ProviderBitbucket:
		return parseBitbucket(body, p)
	default:
		return parseGeneric(r, body, p)
	}
}

type githubPush struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		CloneURL string `json:"clone_url"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit *struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"head_commit"`
	Pusher struct {
		Name string `json:"name"`
	} `json:"pusher"`
	Zen string `json:"zen"`
}

func parseGitHub(r *http.Request, body []byte, p *Payload) (*Payload, error) {
	if r.Header.Get("X-Github-Event") == eventPing {
		p.EventType = eventPing
		return p, nil
	}
	var push githubPush
	if err := json.Unmarshal(body, &push); err != nil {
		return nil, fmt.Errorf("parsing github payload: %w", err)
	}
	p.RepoURL = push.Repository.CloneURL
	p.RepoName = push.Repository.FullName
	p.Commit = push.After
	p.PushedBy = push.Pusher.Name
	if push.HeadCommit != nil {
		p.CommitMsg = firstLine(push.HeadCommit.Message)
	}
	if strings.HasPrefix(push.Ref, "refs/tags/") {
		p.EventType = eventTag
		p.Tag = strings.TrimPrefix(push.Ref, "refs/tags/")
	} else {
		p.EventType = eventPush
		p.Branch = strings.TrimPrefix(push.Ref, "refs/heads/")
	}
	return p, nil
}

type gitlabPush struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		GitHTTPURL string `json:"git_http_url"`
		Name       string `json:"name"`
	} `json:"repository"`
	Commits []struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
		} `json:"author"`
	} `json:"commits"`
	UserName string `json:"user_name"`
}

func parseGitLab(body []byte, p *Payload) (*Payload, error) {
	var push gitlabPush
	if err := json.Unmarshal(body, &push); err != nil {
		return nil, fmt.Errorf("parsing gitlab payload: %w", err)
	}
	p.RepoURL = push.Repository.GitHTTPURL
	p.RepoName = push.Repository.Name
	p.Commit = push.After
	p.PushedBy = push.UserName
	if len(push.Commits) > 0 {
		p.CommitMsg = firstLine(push.Commits[0].Message)
	}
	if strings.HasPrefix(push.Ref, "refs/tags/") {
		p.EventType = eventTag
		p.Tag = strings.TrimPrefix(push.Ref, "refs/tags/")
	} else {
		p.EventType = eventPush
		p.Branch = strings.TrimPrefix(push.Ref, "refs/heads/")
	}
	return p, nil
}

func parseGitea(r *http.Request, body []byte, p *Payload) (*Payload, error) {
	return parseGitHub(r, body, p)
}

type bitbucketPush struct {
	Push struct {
		Changes []struct {
			New *struct {
				Type   string `json:"type"`
				Name   string `json:"name"`
				Target struct {
					Hash    string `json:"hash"`
					Message string `json:"message"`
					Author  struct {
						Raw string `json:"raw"`
					} `json:"author"`
				} `json:"target"`
			} `json:"new"`
		} `json:"changes"`
	} `json:"push"`
	Repository struct {
		Links struct {
			Clone []struct {
				Href string `json:"href"`
				Name string `json:"name"`
			} `json:"clone"`
		} `json:"links"`
		FullName string `json:"full_name"`
	} `json:"repository"`
	Actor struct {
		DisplayName string `json:"display_name"`
	} `json:"actor"`
}

func parseBitbucket(body []byte, p *Payload) (*Payload, error) {
	var push bitbucketPush
	if err := json.Unmarshal(body, &push); err != nil {
		return nil, fmt.Errorf("parsing bitbucket payload: %w", err)
	}
	for _, link := range push.Repository.Links.Clone {
		if link.Name == "https" {
			p.RepoURL = link.Href
			break
		}
	}
	p.RepoName = push.Repository.FullName
	p.PushedBy = push.Actor.DisplayName
	if len(push.Push.Changes) > 0 {
		if change := push.Push.Changes[0]; change.New != nil {
			p.Commit = change.New.Target.Hash
			p.CommitMsg = firstLine(change.New.Target.Message)
			if change.New.Type == eventTag {
				p.EventType = eventTag
				p.Tag = change.New.Name
			} else {
				p.EventType = eventPush
				p.Branch = change.New.Name
			}
		}
	}
	return p, nil
}

type genericPush struct {
	Ref    string `json:"ref"`
	After  string `json:"after"`
	Commit string `json:"commit"`
	Branch string `json:"branch"`
	Tag    string `json:"tag"`
	Repo   string `json:"repo"`
	By     string `json:"pushed_by"`
	Msg    string `json:"message"`
}

func parseGeneric(_ *http.Request, body []byte, p *Payload) (*Payload, error) {
	var push genericPush
	if err := json.Unmarshal(body, &push); err != nil {
		p.EventType = eventPush
		return p, nil
	}
	p.RepoURL = push.Repo
	p.PushedBy = push.By
	p.CommitMsg = push.Msg
	p.Commit = push.After
	if p.Commit == "" {
		p.Commit = push.Commit
	}
	switch {
	case push.Branch != "":
		p.EventType = eventPush
		p.Branch = push.Branch
	case push.Tag != "":
		p.EventType = eventTag
		p.Tag = push.Tag
	case strings.HasPrefix(push.Ref, "refs/tags/"):
		p.EventType = eventTag
		p.Tag = strings.TrimPrefix(push.Ref, "refs/tags/")
	default:
		p.EventType = eventPush
		p.Branch = strings.TrimPrefix(push.Ref, "refs/heads/")
	}
	return p, nil
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func MatchesBranch(filter, branch string) bool {
	if filter == "" || filter == "*" {
		return true
	}
	return filter == branch
}
