package graph_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mindfire-test/meiosis/internal/graph"
	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage/sqlite"
)

func newTestStore(t *testing.T) *graph.Store {
	t.Helper()
	backend, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return graph.NewStore(backend)
}

func TestCreateIntentThenGetIntentRoundTrips(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	keys, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	params := graph.CreateIntentParams{
		Repo:       "github.com/example/repo",
		Title:      "Add feature",
		Goal:       "Implement the feature",
		Acceptance: []specv1.Criterion{{Text: "tests pass"}},
		Scope:      specv1.Scope{Allow: []string{"pkg/**"}, Mode: specv1.ScopeModeEnforce},
		CreatedBy:  "agent:impl-3",
	}

	created, err := store.CreateIntent(ctx, params, keys.PrivateKey)
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}
	if validateErr := created.Validate(); validateErr != nil {
		t.Fatalf("created Intent.Validate() error = %v", validateErr)
	}
	ok, err := crypto.Verify(created, created.Signature, keys.PublicKey)
	if err != nil || !ok {
		t.Fatalf("crypto.Verify() = %v, %v, want true, nil", ok, err)
	}

	fetched, err := store.GetIntent(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetIntent() error = %v", err)
	}
	if fetched.ID != created.ID || fetched.Title != created.Title {
		t.Fatalf("GetIntent() = %+v, want %+v", fetched, created)
	}

	all, err := store.ListIntents(ctx)
	if err != nil {
		t.Fatalf("ListIntents() error = %v", err)
	}
	if len(all) != 1 || all[0].ID != created.ID {
		t.Fatalf("ListIntents() = %+v, want exactly the created intent", all)
	}
}

func TestCreateIntentIDIsContentDerived(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	keys, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	params := graph.CreateIntentParams{
		Repo: "github.com/example/repo", Title: "Add feature", Goal: "Implement the feature",
		Acceptance: []specv1.Criterion{{Text: "tests pass"}},
		Scope:      specv1.Scope{Allow: []string{"pkg/**"}, Mode: specv1.ScopeModeEnforce},
		CreatedBy:  "agent:impl-3",
	}

	first, err := store.CreateIntent(ctx, params, keys.PrivateKey)
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}

	params.Title = "A different feature"
	second, err := store.CreateIntent(ctx, params, keys.PrivateKey)
	if err != nil {
		t.Fatalf("CreateIntent() error = %v", err)
	}

	if first.ID == second.ID {
		t.Fatal("CreateIntent() produced the same ID for different content")
	}
}

func TestSubmitEvidenceThenGetEvidenceRoundTrips(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	keys, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	params := graph.SubmitEvidenceParams{
		Attempt:  "att_" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Producer: "agent:ci-runner",
		World:    specv1.MustParseWorldHash("0000000000000000000000000000000000000000000000000000000000000001"),
		Kind:     specv1.EvidenceKindTestRun,
		Outcome:  specv1.EvidenceOutcomePass,
		Payload:  json.RawMessage(`{"passed":42,"failed":0}`),
	}

	created, err := store.SubmitEvidence(ctx, params, keys.PrivateKey)
	if err != nil {
		t.Fatalf("SubmitEvidence() error = %v", err)
	}
	if validateErr := created.Validate(); validateErr != nil {
		t.Fatalf("created Evidence.Validate() error = %v", validateErr)
	}
	ok, err := crypto.Verify(created, created.Signature, keys.PublicKey)
	if err != nil || !ok {
		t.Fatalf("crypto.Verify() = %v, %v, want true, nil", ok, err)
	}

	fetched, err := store.GetEvidence(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetEvidence() error = %v", err)
	}
	if fetched.ID != created.ID || fetched.Attempt != created.Attempt {
		t.Fatalf("GetEvidence() = %+v, want %+v", fetched, created)
	}
}

func TestSubmitEvidenceRejectsMalformedAttempt(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	keys, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}

	params := graph.SubmitEvidenceParams{
		Attempt:  "not-an-attempt-id",
		Producer: "agent:ci-runner",
		World:    specv1.MustParseWorldHash("0000000000000000000000000000000000000000000000000000000000000001"),
		Kind:     specv1.EvidenceKindTestRun,
		Outcome:  specv1.EvidenceOutcomePass,
		Payload:  json.RawMessage(`{"passed":42,"failed":0}`),
	}

	if _, err := store.SubmitEvidence(ctx, params, keys.PrivateKey); err == nil {
		t.Fatal("SubmitEvidence() expected error for malformed attempt ID")
	}
}
