package v1

import (
	"strings"
	"time"
)

type AttemptStatus string

const (
	AttemptStatusOpen      AttemptStatus = "open"
	AttemptStatusMerged    AttemptStatus = "merged"
	AttemptStatusAbandoned AttemptStatus = "abandoned"
)

type ScopeViolation struct {
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
}

type Attempt struct {
	ID              string            `json:"id"`
	Intent          string            `json:"intent"`
	Author          string            `json:"author"`
	World           WorldHash         `json:"world"`
	BaseWorld       WorldHash         `json:"base_world"`
	GitCommit       string            `json:"git_commit,omitempty"`
	ScopeViolations []ScopeViolation  `json:"scope_violations,omitempty"`
	Status          AttemptStatus     `json:"status"`
	CreatedAt       time.Time         `json:"created_at"`
	Signature       string            `json:"signature"`
}

func (a Attempt) Validate() error {
	if !validContentID(a.ID, "att_") || !validContentID(a.Intent, "int_") || !validPrincipalID(a.Author) || a.World.Validate() != nil || a.BaseWorld.Validate() != nil || a.CreatedAt.IsZero() || strings.TrimSpace(a.Signature) == "" {
		return ErrInvalidAttempt
	}
	if a.Status != AttemptStatusOpen && a.Status != AttemptStatusMerged && a.Status != AttemptStatusAbandoned {
		return ErrInvalidAttempt
	}
	for _, v := range a.ScopeViolations {
		if strings.TrimSpace(v.Path) == "" {
			return ErrInvalidAttempt
		}
	}
	return nil
}
