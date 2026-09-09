// Package evidence contains adapters for execution evidence sources.
package evidence

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mindfire-test/meiosis/internal/graph"
	meiosiscrypto "github.com/mindfire-test/meiosis/pkg/crypto"
	v1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// IngestGoTestJSON parses go test -json output, signs the resulting Evidence,
// and stores it with the active attempt as one atomic operation.
func IngestGoTestJSON(ctx context.Context, store *graph.Store, attemptID, producer string, world v1.WorldHash, privateKey ed25519.PrivateKey, input io.Reader) (v1.Evidence, error) {
	if store == nil {
		return v1.Evidence{}, fmt.Errorf("evidence graph store must not be nil")
	}
	events, createdAt, err := parseGoTestJSON(input)
	if err != nil {
		return v1.Evidence{}, err
	}
	outcome := v1.EvidenceOutcomeInconclusive
	for _, event := range events {
		if event.Action == "fail" {
			outcome = v1.EvidenceOutcomeFail
			break
		}
		if event.Action == "pass" {
			outcome = v1.EvidenceOutcomePass
		}
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	evidence := v1.Evidence{
		Attempt:   attemptID,
		World:     world,
		Kind:      v1.EvidenceKindTestRun,
		Producer:  producer,
		Outcome:   outcome,
		CreatedAt: createdAt,
	}
	evidence.Payload, err = json.Marshal(testPayload{Format: "go test -json", Events: events})
	if err != nil {
		return v1.Evidence{}, fmt.Errorf("encode evidence payload: %w", err)
	}
	digest, err := v1.HashCanonical(evidence)
	if err != nil {
		return v1.Evidence{}, fmt.Errorf("hash evidence: %w", err)
	}
	evidence.ID = "evi_" + v1.DigestHex(digest)
	signature, err := meiosiscrypto.Sign(evidence, privateKey)
	if err != nil {
		return v1.Evidence{}, fmt.Errorf("sign evidence: %w", err)
	}
	evidence.Signature = signature
	if err := evidence.Validate(); err != nil {
		return v1.Evidence{}, fmt.Errorf("validate ingested evidence: %w", err)
	}
	if err := store.PutEvidence(ctx, evidence); err != nil {
		return v1.Evidence{}, fmt.Errorf("persist evidence: %w", err)
	}
	return evidence, nil
}

func parseGoTestJSON(input io.Reader) ([]testEvent, time.Time, error) {
	if input == nil {
		return nil, time.Time{}, fmt.Errorf("test output must not be nil")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var events []testEvent
	var createdAt time.Time
	line := 0
	for scanner.Scan() {
		line++
		data := strings.TrimSpace(string(scanner.Bytes()))
		if len(data) == 0 {
			continue
		}
		var event testEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return nil, time.Time{}, fmt.Errorf("parse go test JSON line %d: %w", line, err)
		}
		if event.Action == "" {
			return nil, time.Time{}, fmt.Errorf("parse go test JSON line %d: missing Action", line)
		}
		if !validAction(event.Action) {
			return nil, time.Time{}, fmt.Errorf("parse go test JSON line %d: unsupported Action %q", line, event.Action)
		}
		if createdAt.IsZero() && !event.Time.IsZero() {
			createdAt = event.Time
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("read go test JSON: %w", err)
	}
	if len(events) == 0 {
		return nil, time.Time{}, fmt.Errorf("parse go test JSON: no events found")
	}
	return events, createdAt, nil
}

func validAction(action string) bool {
	switch action {
	case "start", "run", "pause", "cont", "pass", "fail", "skip", "output":
		return true
	default:
		return false
	}
}
