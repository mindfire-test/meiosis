package v1

import (
	"encoding/json"
	"strings"
	"time"
)

type EvidenceKind string

const (
	EvidenceKindTestRun        EvidenceKind = "test-run"
	EvidenceKindCoverage       EvidenceKind = "coverage"
	EvidenceKindTypeCheck      EvidenceKind = "type-check"
	EvidenceKindStaticAnalysis EvidenceKind = "static-analysis"
	EvidenceKindBenchmark      EvidenceKind = "benchmark"
	EvidenceKindMutation       EvidenceKind = "mutation"
	EvidenceKindSecurityScan   EvidenceKind = "security-scan"
	EvidenceKindReplayMatch    EvidenceKind = "replay-match"
	EvidenceKindHumanReview    EvidenceKind = "human-review"
)

type EvidenceOutcome string

const (
	EvidenceOutcomePass         EvidenceOutcome = "pass"
	EvidenceOutcomeFail         EvidenceOutcome = "fail"
	EvidenceOutcomeInconclusive EvidenceOutcome = "inconclusive"
)

type Evidence struct {
	ID        string          `json:"id"`
	Attempt   string          `json:"attempt"`
	World     WorldHash       `json:"world"`
	Kind      EvidenceKind    `json:"kind"`
	Producer  string          `json:"producer"`
	Outcome   EvidenceOutcome `json:"outcome"`
	Footprint []string        `json:"footprint,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	ExpiresAt time.Time       `json:"expires_at,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	Signature string          `json:"signature"`
}

func (e Evidence) Validate() error {
	if strings.TrimSpace(e.ID) == "" || !validContentID(e.Attempt, "att_") || e.World.Validate() != nil || !validPrincipalID(e.Producer) || !validEvidenceKind(e.Kind) || !validEvidenceOutcome(e.Outcome) || len(e.Payload) == 0 || !json.Valid(e.Payload) || e.CreatedAt.IsZero() || strings.TrimSpace(e.Signature) == "" {
		return ErrInvalidEvidence
	}
	if !e.ExpiresAt.IsZero() && e.ExpiresAt.Before(e.CreatedAt) {
		return ErrInvalidEvidence
	}
	return nil
}

func validEvidenceKind(k EvidenceKind) bool {
	return k == EvidenceKindTestRun || k == EvidenceKindCoverage || k == EvidenceKindTypeCheck || k == EvidenceKindStaticAnalysis || k == EvidenceKindBenchmark || k == EvidenceKindMutation || k == EvidenceKindSecurityScan || k == EvidenceKindReplayMatch || k == EvidenceKindHumanReview
}

func validEvidenceOutcome(o EvidenceOutcome) bool {
	return o == EvidenceOutcomePass || o == EvidenceOutcomeFail || o == EvidenceOutcomeInconclusive
}
