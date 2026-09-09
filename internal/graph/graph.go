// Package graph provides typed persistence for the Meiosis object graph.
package graph

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	v1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage"
)

func New(backend storage.Store) (*Store, error) {
	if backend == nil {
		return nil, errors.New("graph storage backend must not be nil")
	}
	return &Store{backend: backend}, nil
}

func (s *Store) PutAttempt(ctx context.Context, attempt v1.Attempt) error {
	if err := attempt.Validate(); err != nil {
		return fmt.Errorf("validate attempt: %w", err)
	}
	data, err := json.Marshal(attempt)
	if err != nil {
		return fmt.Errorf("marshal attempt: %w", err)
	}
	return s.backend.Put(ctx, storage.KindAttempt, attempt.ID, data)
}

func (s *Store) GetAttempt(ctx context.Context, id string) (v1.Attempt, error) {
	data, err := s.backend.Get(ctx, storage.KindAttempt, id)
	if err != nil {
		return v1.Attempt{}, err
	}
	var attempt v1.Attempt
	if err := json.Unmarshal(data, &attempt); err != nil {
		return v1.Attempt{}, fmt.Errorf("unmarshal attempt %q: %w", id, err)
	}
	if err := attempt.Validate(); err != nil {
		return v1.Attempt{}, fmt.Errorf("validate stored attempt %q: %w", id, err)
	}
	return attempt, nil
}

// PutEvidence persists evidence only when its referenced attempt is active.
// The attempt check and evidence write occur in one transaction.
func (s *Store) PutEvidence(ctx context.Context, evidence v1.Evidence) error {
	if err := evidence.Validate(); err != nil {
		return fmt.Errorf("validate evidence: %w", err)
	}
	data, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	return s.backend.Transaction(ctx, func(tx storage.Tx) error {
		attemptData, err := tx.Get(ctx, storage.KindAttempt, evidence.Attempt)
		if err != nil {
			return fmt.Errorf("load attempt %q: %w", evidence.Attempt, err)
		}
		var attempt v1.Attempt
		if err := json.Unmarshal(attemptData, &attempt); err != nil {
			return fmt.Errorf("unmarshal attempt %q: %w", evidence.Attempt, err)
		}
		if err := attempt.Validate(); err != nil {
			return fmt.Errorf("validate attempt %q: %w", evidence.Attempt, err)
		}
		if attempt.Status != v1.AttemptStatusOpen {
			return fmt.Errorf("%w: %s", ErrInactiveAttempt, evidence.Attempt)
		}
		return tx.Put(ctx, storage.KindEvidence, evidence.ID, data)
	})
}

func (s *Store) GetEvidence(ctx context.Context, id string) (v1.Evidence, error) {
	data, err := s.backend.Get(ctx, storage.KindEvidence, id)
	if err != nil {
		return v1.Evidence{}, err
	}
	var evidence v1.Evidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return v1.Evidence{}, fmt.Errorf("unmarshal evidence %q: %w", id, err)
	}
	if err := evidence.Validate(); err != nil {
		return v1.Evidence{}, fmt.Errorf("validate stored evidence %q: %w", id, err)
	}
	return evidence, nil
}
