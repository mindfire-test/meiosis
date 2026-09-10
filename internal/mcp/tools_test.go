package mcp

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mindfire-test/meiosis/internal/graph"
	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage/sqlite"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	backend, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	keys, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	return NewServer(graph.NewStore(backend), "agent:impl-3", keys.PrivateKey)
}

func TestHandleIntentCreateReturnsSignedIntent(t *testing.T) {
	s := newTestServer(t)
	args := `{"repo":"github.com/example/repo","title":"Add feature","goal":"Implement it",
		"acceptance":[{"text":"tests pass"}],"scope":{"allow":["pkg/**"],"mode":"enforce"}}`

	result, err := handleIntentCreate(context.Background(), s, json.RawMessage(args))
	if err != nil {
		t.Fatalf("handleIntentCreate() error = %v", err)
	}
	intent, ok := result.(specv1.Intent)
	if !ok {
		t.Fatalf("handleIntentCreate() returned %T, want specv1.Intent", result)
	}
	if err := intent.Validate(); err != nil {
		t.Fatalf("returned Intent.Validate() error = %v", err)
	}
	if intent.CreatedBy != s.Principal {
		t.Fatalf("CreatedBy = %q, want %q (server's own principal)", intent.CreatedBy, s.Principal)
	}
}

func TestHandleIntentCheckPathReflectsScope(t *testing.T) {
	s := newTestServer(t)
	createArgs := `{"repo":"github.com/example/repo","title":"Add feature","goal":"Implement it",
		"acceptance":[{"text":"tests pass"}],"scope":{"allow":["internal/auth/**"],"deny":["internal/auth/secrets/**"],"mode":"enforce"}}`
	created, err := handleIntentCreate(context.Background(), s, json.RawMessage(createArgs))
	if err != nil {
		t.Fatalf("handleIntentCreate() error = %v", err)
	}
	id := created.(specv1.Intent).ID

	allowedResult, err := handleIntentCheckPath(context.Background(), s, json.RawMessage(`{"intent":"`+id+`","path":"internal/auth/handler.go"}`))
	if err != nil {
		t.Fatalf("handleIntentCheckPath() error = %v", err)
	}
	m := allowedResult.(map[string]any)
	if allowed, _ := m["allowed"].(bool); !allowed {
		t.Fatalf("intent_check_path allowed = %v, want true", m["allowed"])
	}

	deniedResult, err := handleIntentCheckPath(context.Background(), s, json.RawMessage(`{"intent":"`+id+`","path":"internal/auth/secrets/keys.pem"}`))
	if err != nil {
		t.Fatalf("handleIntentCheckPath() error = %v", err)
	}
	m = deniedResult.(map[string]any)
	if allowed, _ := m["allowed"].(bool); allowed {
		t.Fatalf("intent_check_path allowed = %v, want false for a denied path", m["allowed"])
	}
}

func TestHandleEvidenceSubmitDefaultsProducerToServerPrincipal(t *testing.T) {
	s := newTestServer(t)
	args := `{"attempt":"att_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"world":"0000000000000000000000000000000000000000000000000000000000000001",
		"kind":"test-run","outcome":"pass","payload":{"passed":1}}`

	result, err := handleEvidenceSubmit(context.Background(), s, json.RawMessage(args))
	if err != nil {
		t.Fatalf("handleEvidenceSubmit() error = %v", err)
	}
	evidence, ok := result.(specv1.Evidence)
	if !ok {
		t.Fatalf("handleEvidenceSubmit() returned %T, want specv1.Evidence", result)
	}
	if evidence.Producer != s.Principal {
		t.Fatalf("Producer = %q, want %q (server's own principal)", evidence.Producer, s.Principal)
	}
}

func TestHandleIntentCreateRejectsInvalidArgs(t *testing.T) {
	s := newTestServer(t)
	if _, err := handleIntentCreate(context.Background(), s, json.RawMessage(`{"repo":`)); err == nil {
		t.Fatal("handleIntentCreate() expected error for malformed JSON")
	}
}

func TestHandleIntentCheckPathRejectsUnknownIntent(t *testing.T) {
	s := newTestServer(t)
	unknownID := "int_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := handleIntentCheckPath(context.Background(), s, json.RawMessage(`{"intent":"`+unknownID+`","path":"x"}`)); err == nil {
		t.Fatal("handleIntentCheckPath() expected error for an intent that was never created")
	}
}
