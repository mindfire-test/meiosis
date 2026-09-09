package evidence

import (
	"context"
	"crypto/ed25519"
	"strings"
	"testing"
	"time"

	"github.com/mindfire-test/meiosis/internal/graph"
	meiosiscrypto "github.com/mindfire-test/meiosis/pkg/crypto"
	v1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage/sqlite"
)

func TestIngestGoTestJSON(t *testing.T) {
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	}()
	graphStore, err := graph.New(store)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := meiosiscrypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	attempt := testAttempt()
	if putErr := graphStore.PutAttempt(context.Background(), attempt); putErr != nil {
		t.Fatal(putErr)
	}
	evidence, err := IngestGoTestJSON(context.Background(), graphStore, attempt.ID, "agent:ci", attempt.World, keys.PrivateKey, strings.NewReader(`{"Time":"2026-09-09T10:00:00Z","Action":"start","Package":"example"}
{"Time":"2026-09-09T10:00:01Z","Action":"pass","Package":"example","Elapsed":1}`))
	if err != nil {
		t.Fatalf("IngestGoTestJSON() error = %v", err)
	}
	if evidence.Outcome != v1.EvidenceOutcomePass || evidence.Signature == "" {
		t.Fatalf("evidence = %+v, want pass with signature", evidence)
	}
	valid, err := meiosiscrypto.Verify(evidence, evidence.Signature, keys.PublicKey)
	if err != nil || !valid {
		t.Fatalf("signature verification = %v, %v", valid, err)
	}
	if _, err := graphStore.GetEvidence(context.Background(), evidence.ID); err != nil {
		t.Fatalf("stored evidence unavailable: %v", err)
	}
}

func TestIngestGoTestJSONRejectsMalformedOutput(t *testing.T) {
	keys := make(ed25519.PrivateKey, ed25519.PrivateKeySize)
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	}()
	graphStore, _ := graph.New(store)
	for name, input := range map[string]string{
		"bad JSON":       "not-json",
		"missing action": `{"Package":"example"}`,
		"empty":          "",
		"unknown action": `{"Action":"unknown"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := IngestGoTestJSON(context.Background(), graphStore, "att_missing", "agent:ci", validWorld(), keys, strings.NewReader(input)); err == nil {
				t.Fatal("IngestGoTestJSON() accepted malformed output")
			}
		})
	}
}

func TestIngestGoTestJSONRejectsInactiveAttempt(t *testing.T) {
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Errorf("close store: %v", closeErr)
		}
	}()
	graphStore, _ := graph.New(store)
	attempt := testAttempt()
	attempt.Status = v1.AttemptStatusMerged
	if err := graphStore.PutAttempt(context.Background(), attempt); err != nil {
		t.Fatal(err)
	}
	keys, _ := meiosiscrypto.GenerateKeyPair()
	if _, err := IngestGoTestJSON(context.Background(), graphStore, attempt.ID, "agent:ci", attempt.World, keys.PrivateKey, strings.NewReader(`{"Action":"pass"}`)); err == nil {
		t.Fatal("IngestGoTestJSON() accepted inactive attempt")
	}
}

func testAttempt() v1.Attempt {
	hash := validWorld()
	return v1.Attempt{ID: "att_" + strings.Repeat("a", 52), Intent: "int_" + strings.Repeat("b", 52), Author: "agent:planner", World: hash, BaseWorld: hash, Status: v1.AttemptStatusOpen, CreatedAt: time.Unix(1, 0), Signature: "attempt-signature"}
}

func validWorld() v1.WorldHash {
	return v1.MustParseWorldHash(strings.Repeat("a", 64))
}
