package evidence

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mindfire-test/meiosis/internal/graph"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// IngestParams describes one report to turn into evidence.
type IngestParams struct {
	Attempt  string
	Producer string
	// World is the world hash the report was produced against. It is required:
	// evidence not bound to a world is rejected (FR-4.1).
	World specv1.WorldHash
	// Format of the report; empty means detect it from the content.
	Format  Format
	Options Options
	// Footprint is what the caller says the evidence depends on. The files the
	// report itself covers are added to it (see Report.Footprint).
	Footprint  []string
	Supersedes string
	// ExpiresAt, when set, is when the evidence expires whatever the code does
	// (FR-4.11).
	ExpiresAt time.Time
}

// Ingest parses a report and records it as signed evidence bound to p.World.
// The attempt check, independence computation and write happen in one
// transaction inside the graph store; the outcome comes from the report, never
// from the caller.
func Ingest(ctx context.Context, store *graph.Store, p IngestParams, key ed25519.PrivateKey, report []byte) (specv1.Evidence, Report, error) {
	if store == nil {
		return specv1.Evidence{}, Report{}, errors.New("evidence graph store must not be nil")
	}
	format := p.Format
	if format == "" {
		detected, err := Detect(report)
		if err != nil {
			return specv1.Evidence{}, Report{}, err
		}
		format = detected
	}
	parsed, err := Parse(format, report, p.Options)
	if err != nil {
		return specv1.Evidence{}, Report{}, err
	}
	if parsed.Footprint, err = checkInWorld(ctx, store, p.World, parsed.Footprint); err != nil {
		return specv1.Evidence{}, Report{}, err
	}
	footprint := p.Footprint
	if len(parsed.Footprint) > 0 && len(p.Footprint)+len(parsed.Footprint) <= specv1.MaxFootprintEntries {
		footprint = append(append([]string{}, p.Footprint...), parsed.Footprint...)
	}
	evidence, err := store.SubmitEvidence(ctx, graph.SubmitEvidenceParams{
		Attempt:    p.Attempt,
		Producer:   p.Producer,
		World:      p.World,
		Kind:       parsed.Kind,
		Outcome:    parsed.Outcome,
		Payload:    parsed.Payload,
		Footprint:  footprint,
		ExpiresAt:  p.ExpiresAt,
		Supersedes: p.Supersedes,
	}, key)
	if err != nil {
		return specv1.Evidence{}, Report{}, fmt.Errorf("persist evidence: %w", err)
	}
	return evidence, parsed, nil
}

// checkInWorld keeps a report's footprint only if every file in it exists in
// the world the evidence is bound to. A report run from another directory names
// paths that are not in the repository; such a footprint would never match a
// change, and the evidence would stay carried for ever. An unknown world cannot
// be checked, so it also drops the footprint.
func checkInWorld(ctx context.Context, store *graph.Store, world specv1.WorldHash, footprint []string) ([]string, error) {
	if len(footprint) == 0 {
		return nil, nil
	}
	files, err := store.WorldFiles(ctx, world)
	if errors.Is(err, graph.ErrUnknownWorld) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("check report footprint: %w", err)
	}
	for _, p := range footprint {
		if _, ok := files[p]; !ok {
			return nil, nil
		}
	}
	return footprint, nil
}

// IngestGoTestJSON parses go test -json output, signs the resulting Evidence,
// and stores it with the active attempt as one atomic operation.
func IngestGoTestJSON(ctx context.Context, store *graph.Store, attemptID, producer string, world specv1.WorldHash, privateKey ed25519.PrivateKey, input io.Reader) (specv1.Evidence, error) {
	data, err := ReadReport(input)
	if err != nil {
		return specv1.Evidence{}, err
	}
	evidence, _, err := Ingest(ctx, store, IngestParams{Attempt: attemptID, Producer: producer, World: world, Format: FormatGoTest}, privateKey, data)
	return evidence, err
}
