package v1

import (
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

type IntentStatus string

const (
	IntentStatusOpen       IntentStatus = "open"
	IntentStatusAttempting IntentStatus = "attempting"
	IntentStatusMerged     IntentStatus = "merged"
	IntentStatusAbandoned  IntentStatus = "abandoned"
	IntentStatusStale      IntentStatus = "stale"
)

type Criterion struct {
	Text  string         `json:"text"`
	Check map[string]any `json:"check,omitempty"`
}

type ScopeMode string

const (
	ScopeModeEnforce ScopeMode = "enforce"
	ScopeModeWarn    ScopeMode = "warn"
)

type Scope struct {
	Allow []string  `json:"allow"`
	Deny  []string  `json:"deny,omitempty"`
	Mode  ScopeMode `json:"mode"`
}

type ContextPinKind string

const (
	ContextPinKindSpec       ContextPinKind = "spec"
	ContextPinKindADR        ContextPinKind = "adr"
	ContextPinKindIssue      ContextPinKind = "issue"
	ContextPinKindDesign     ContextPinKind = "design"
	ContextPinKindConvention ContextPinKind = "convention"
)

type ContextPin struct {
	Ref    string         `json:"ref"`
	Digest string         `json:"digest"`
	Kind   ContextPinKind `json:"kind"`
}

type Intent struct {
	ID          string       `json:"id"`
	Repo        string       `json:"repo"`
	Title       string       `json:"title"`
	Goal        string       `json:"goal"`
	Acceptance  []Criterion  `json:"acceptance"`
	NonGoals    []string     `json:"non_goals,omitempty"`
	Scope       Scope        `json:"scope"`
	ContextPins []ContextPin `json:"context_pins,omitempty"`
	Parent      string       `json:"parent,omitempty"`
	DependsOn   []string     `json:"depends_on,omitempty"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
	Status      IntentStatus `json:"status"`
	Signature   string       `json:"signature"`
}

func (i Intent) Validate() error {
	if !validContentID(i.ID, "int_") || strings.TrimSpace(i.Repo) == "" || strings.TrimSpace(i.Title) == "" || strings.TrimSpace(i.Goal) == "" || len(i.Acceptance) == 0 || !validPrincipalID(i.CreatedBy) || i.CreatedAt.IsZero() || i.Signature == "" {
		return ErrInvalidIntent
	}
	if i.Status != IntentStatusOpen && i.Status != IntentStatusAttempting && i.Status != IntentStatusMerged && i.Status != IntentStatusAbandoned && i.Status != IntentStatusStale {
		return ErrInvalidIntent
	}
	if i.Scope.Validate() != nil {
		return ErrInvalidIntent
	}
	for _, c := range i.Acceptance {
		if strings.TrimSpace(c.Text) == "" {
			return ErrInvalidIntent
		}
	}
	for _, id := range append(append([]string{}, i.DependsOn...), i.Parent) {
		if id != "" && !validContentID(id, "int_") {
			return ErrInvalidIntent
		}
	}
	for _, p := range i.ContextPins {
		if strings.TrimSpace(p.Ref) == "" || !validHexDigest(p.Digest) || !validContextPinKind(p.Kind) {
			return ErrInvalidIntent
		}
	}
	return nil
}

func (s Scope) Validate() error {
	if len(s.Allow) == 0 || (s.Mode != ScopeModeEnforce && s.Mode != ScopeModeWarn) {
		return ErrInvalidIntent
	}
	for _, pattern := range append(append([]string{}, s.Allow...), s.Deny...) {
		if pattern == "" {
			return ErrInvalidIntent
		}
		if _, err := doublestar.Match(pattern, "x"); err != nil {
			return ErrInvalidIntent
		}
	}
	return nil
}

func validContextPinKind(k ContextPinKind) bool {
	return k == ContextPinKindSpec || k == ContextPinKindADR || k == ContextPinKindIssue || k == ContextPinKindDesign || k == ContextPinKindConvention
}
