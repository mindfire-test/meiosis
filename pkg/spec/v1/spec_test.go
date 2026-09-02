package v1

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWorldHashJSON(t *testing.T) {
	h, err := ParseWorldHash("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("ParseWorldHash() error = %v", err)
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if got, want := string(b), `"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"`; got != want {
		t.Fatalf("Marshal() = %s, want %s", got, want)
	}
}

func TestVerdictValidate(t *testing.T) {
	for _, decision := range []VerdictDecision{VerdictDecisionMerge, VerdictDecisionReject, VerdictDecisionEscalate} {
		tt := Verdict{Attempt: validAttemptID(), Decision: decision, DecidedBy: "human:lakin", PolicyRef: "policy @ hash", PolicyIn: json.RawMessage(`{"allow":true}`), Rationale: "reviewed", DecidedAt: time.Unix(1, 0), Signature: "sig"}
		if err := tt.Validate(); err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	}
	if err := (Verdict{Attempt: validAttemptID(), Decision: "bad", DecidedBy: "human:lakin", PolicyRef: "p", PolicyIn: json.RawMessage(`{}`), Rationale: "r", DecidedAt: time.Unix(1, 0), Signature: "s"}).Validate(); err == nil {
		t.Fatal("Validate() expected error")
	}
}

func TestPrincipalValidate(t *testing.T) {
	if err := (Principal{ID: "agent:planner-7", Kind: PrincipalKindAgent}).Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if err := (Principal{Kind: PrincipalKindAgent}).Validate(); err == nil {
		t.Fatal("Validate() expected error")
	}
}

func TestAttemptValidateAndJSON(t *testing.T) {
	h := MustParseWorldHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	a := Attempt{ID: validAttemptID(), Intent: validIntentID(), Author: "human:lakin", World: h, BaseWorld: h, Status: AttemptStatusOpen, CreatedAt: time.Unix(1, 0), Signature: "sig"}
	if err := a.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if len(b) == 0 {
		t.Fatal("Marshal() produced empty output")
	}
}

func TestEvidenceValidate(t *testing.T) {
	h := MustParseWorldHash("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	e := Evidence{ID: "e1", Attempt: validAttemptID(), Kind: EvidenceKindTestRun, Producer: "human:lakin", World: h, Outcome: EvidenceOutcomePass, Payload: json.RawMessage(`{}`), CreatedAt: time.Unix(2, 0), Signature: "sig"}
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestWorldHashRejectsInvalidJSON(t *testing.T) {
	var h WorldHash
	if err := json.Unmarshal([]byte(`"not-a-hash"`), &h); err == nil {
		t.Fatal("Unmarshal() expected error")
	}
}

func TestIntentValidate(t *testing.T) {
	i := Intent{
		ID: validIntentID(), Repo: "github.com/example/repo", Title: "Add feature", Goal: "Implement feature",
		Acceptance: []Criterion{{Text: "tests pass"}}, Scope: Scope{Allow: []string{"pkg/**"}, Mode: ScopeModeEnforce},
		CreatedBy: "human:lakin", CreatedAt: time.Unix(1, 0), Status: IntentStatusOpen, Signature: "sig",
	}
	if err := i.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	i.Scope.Allow = []string{"["}
	if err := i.Validate(); err == nil {
		t.Fatal("Validate() expected invalid glob error")
	}
}

func validIntentID() string  { return "int_" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }
func validAttemptID() string { return "att_" + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" }
