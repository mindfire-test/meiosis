package graph

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage"
)

// ErrInvalidParams is returned when a built record fails validation before
// it would otherwise be persisted.
var ErrInvalidParams = fmt.Errorf("graph: invalid parameters")

// NewStore returns a Store persisting through backend. It is equivalent to
// New, save for the nil check, and exists for callers (the MCP daemon,
// issue #16) built against that constructor name.
func NewStore(backend storage.Store) *Store {
	return &Store{backend: backend}
}

// CreateIntentParams describes the intent being declared.
type CreateIntentParams struct {
	Repo       string
	Title      string
	Goal       string
	Acceptance []specv1.Criterion
	Scope      specv1.Scope
	CreatedBy  string // principal declaring the intent
}

// CreateIntent builds, signs, persists and returns a new Intent. Its ID is
// content-derived (specv1.NewContentID) so identical declarations collide
// onto the same ID rather than minting duplicates.
func (s *Store) CreateIntent(ctx context.Context, p CreateIntentParams, signerKey ed25519.PrivateKey) (specv1.Intent, error) {
	intent := specv1.Intent{
		Repo:       p.Repo,
		Title:      p.Title,
		Goal:       p.Goal,
		Acceptance: p.Acceptance,
		Scope:      p.Scope,
		CreatedBy:  p.CreatedBy,
		CreatedAt:  time.Now().UTC(),
		Status:     specv1.IntentStatusOpen,
	}

	id, err := specv1.NewContentID("int_", intent)
	if err != nil {
		return specv1.Intent{}, fmt.Errorf("graph: generate intent id: %w", err)
	}
	intent.ID = id

	signature, err := crypto.Sign(intent, signerKey)
	if err != nil {
		return specv1.Intent{}, fmt.Errorf("graph: sign intent: %w", err)
	}
	intent.Signature = signature

	if err := intent.Validate(); err != nil {
		return specv1.Intent{}, fmt.Errorf("%w: %v", ErrInvalidParams, err)
	}

	encoded, err := specv1.Canonicalize(intent)
	if err != nil {
		return specv1.Intent{}, fmt.Errorf("graph: encode intent: %w", err)
	}
	if err := s.backend.Put(ctx, storage.KindIntent, intent.ID, encoded); err != nil {
		return specv1.Intent{}, fmt.Errorf("graph: persist intent: %w", err)
	}
	return intent, nil
}

// GetIntent returns the intent with the given ID.
func (s *Store) GetIntent(ctx context.Context, id string) (specv1.Intent, error) {
	data, err := s.backend.Get(ctx, storage.KindIntent, id)
	if err != nil {
		return specv1.Intent{}, fmt.Errorf("graph: get intent: %w", err)
	}
	var intent specv1.Intent
	if err := json.Unmarshal(data, &intent); err != nil {
		return specv1.Intent{}, fmt.Errorf("graph: decode intent: %w", err)
	}
	return intent, nil
}

// ListIntents returns every persisted intent, in no particular order.
func (s *Store) ListIntents(ctx context.Context) ([]specv1.Intent, error) {
	records, err := s.backend.List(ctx, storage.KindIntent)
	if err != nil {
		return nil, fmt.Errorf("graph: list intents: %w", err)
	}
	intents := make([]specv1.Intent, 0, len(records))
	for _, data := range records {
		var intent specv1.Intent
		if err := json.Unmarshal(data, &intent); err != nil {
			return nil, fmt.Errorf("graph: decode intent: %w", err)
		}
		intents = append(intents, intent)
	}
	return intents, nil
}

// SubmitEvidenceParams describes the evidence being submitted.
//
// Attempt is taken as-is here: unlike PutEvidence, this predates Attempt
// persistence and does not check the referenced attempt is active. Kept
// for callers (the MCP daemon's evidence_submit tool) that build and sign
// evidence from raw parameters rather than passing an already-signed record.
type SubmitEvidenceParams struct {
	Attempt   string
	Producer  string
	World     specv1.WorldHash
	Kind      specv1.EvidenceKind
	Outcome   specv1.EvidenceOutcome
	Payload   json.RawMessage
	Footprint []string
	ExpiresAt time.Time
}

// SubmitEvidence builds, signs, persists and returns a new Evidence record.
func (s *Store) SubmitEvidence(ctx context.Context, p SubmitEvidenceParams, signerKey ed25519.PrivateKey) (specv1.Evidence, error) {
	evidence := specv1.Evidence{
		Attempt:   p.Attempt,
		World:     p.World,
		Kind:      p.Kind,
		Producer:  p.Producer,
		Outcome:   p.Outcome,
		Footprint: p.Footprint,
		Payload:   p.Payload,
		ExpiresAt: p.ExpiresAt,
		CreatedAt: time.Now().UTC(),
	}

	// Evidence.ID has no mandated format (spec/v1/evidence.md just calls it a
	// "unique identifier"), but a content-derived ID keeps it consistent with
	// Intent's scheme rather than inventing a second convention.
	id, err := specv1.NewContentID("evd_", evidence)
	if err != nil {
		return specv1.Evidence{}, fmt.Errorf("graph: generate evidence id: %w", err)
	}
	evidence.ID = id

	signature, err := crypto.Sign(evidence, signerKey)
	if err != nil {
		return specv1.Evidence{}, fmt.Errorf("graph: sign evidence: %w", err)
	}
	evidence.Signature = signature

	if err := evidence.Validate(); err != nil {
		return specv1.Evidence{}, fmt.Errorf("%w: %v", ErrInvalidParams, err)
	}

	encoded, err := specv1.Canonicalize(evidence)
	if err != nil {
		return specv1.Evidence{}, fmt.Errorf("graph: encode evidence: %w", err)
	}
	if err := s.backend.Put(ctx, storage.KindEvidence, evidence.ID, encoded); err != nil {
		return specv1.Evidence{}, fmt.Errorf("graph: persist evidence: %w", err)
	}
	return evidence, nil
}
