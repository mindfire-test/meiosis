package graph_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/mindfire-test/meiosis/internal/graph"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

func TestEvaluateStanding(t *testing.T) {
	w1, w2 := specv1.WorldHash{1}, specv1.WorldHash{2}
	now := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	modified := func(path string) graph.FileChange {
		return graph.FileChange{Path: path, Status: graph.ChangeModified}
	}
	evidence := func(world specv1.WorldHash, footprint ...string) specv1.Evidence {
		return specv1.Evidence{World: world, Footprint: footprint}
	}
	expiring := func(at time.Time, footprint ...string) specv1.Evidence {
		e := evidence(w1, footprint...)
		e.ExpiresAt = at
		return e
	}

	tests := []struct {
		name    string
		in      graph.StandingInput
		want    graph.Standing
		reasons []graph.StandingReason
	}{
		{
			name: "same world is fresh",
			in:   graph.StandingInput{Evidence: evidence(w1, "internal/auth/**"), Current: w1, Now: now},
			want: graph.StandingFresh,
		},
		{
			name: "same world with empty footprint is fresh",
			in:   graph.StandingInput{Evidence: evidence(w1), Current: w1, Now: now},
			want: graph.StandingFresh,
		},
		{
			name: "same world ignores changes",
			in:   graph.StandingInput{Evidence: evidence(w1, "a.go"), Current: w1, Changes: []graph.FileChange{modified("a.go")}, Now: now},
			want: graph.StandingFresh,
		},
		{
			name: "moved world, unrelated change, is carried",
			in: graph.StandingInput{
				Evidence: evidence(w1, "internal/auth/**"), Current: w2,
				Changes: []graph.FileChange{modified("README.md")}, Now: now,
			},
			want: graph.StandingCarried,
		},
		{
			name: "moved world, no changed paths, is carried",
			in:   graph.StandingInput{Evidence: evidence(w1, "a.go"), Current: w2, Now: now},
			want: graph.StandingCarried,
		},
		{
			name: "glob matches a changed path",
			in: graph.StandingInput{
				Evidence: evidence(w1, "internal/auth/**"), Current: w2,
				Changes: []graph.FileChange{modified("internal/auth/token.go"), modified("README.md")}, Now: now,
			},
			want:    graph.StandingStale,
			reasons: []graph.StandingReason{{Path: "internal/auth/token.go", Change: graph.ChangeModified}},
		},
		{
			name: "exact path matches a changed path",
			in: graph.StandingInput{
				Evidence: evidence(w1, "pkg/crypto/sign.go"), Current: w2,
				Changes: []graph.FileChange{modified("pkg/crypto/sign.go")}, Now: now,
			},
			want:    graph.StandingStale,
			reasons: []graph.StandingReason{{Path: "pkg/crypto/sign.go", Change: graph.ChangeModified}},
		},
		{
			name: "exact path does not match a sibling",
			in: graph.StandingInput{
				Evidence: evidence(w1, "pkg/crypto/sign.go"), Current: w2,
				Changes: []graph.FileChange{modified("pkg/crypto/sign_test.go")}, Now: now,
			},
			want: graph.StandingCarried,
		},
		{
			name: "added and deleted files count",
			in: graph.StandingInput{
				Evidence: evidence(w1, "internal/auth/**"), Current: w2,
				Changes: []graph.FileChange{
					{Path: "internal/auth/old.go", Status: graph.ChangeDeleted},
					{Path: "internal/auth/new.go", Status: graph.ChangeAdded},
				},
				Now: now,
			},
			want: graph.StandingStale,
			reasons: []graph.StandingReason{
				{Path: "internal/auth/new.go", Change: graph.ChangeAdded},
				{Path: "internal/auth/old.go", Change: graph.ChangeDeleted},
			},
		},
		{
			name: "reasons sorted by path",
			in: graph.StandingInput{
				Evidence: evidence(w1, "**"), Current: w2,
				Changes: []graph.FileChange{modified("z.go"), modified("a.go"), modified("m.go")}, Now: now,
			},
			want: graph.StandingStale,
			reasons: []graph.StandingReason{
				{Path: "a.go", Change: graph.ChangeModified},
				{Path: "m.go", Change: graph.ChangeModified},
				{Path: "z.go", Change: graph.ChangeModified},
			},
		},
		{
			name:    "empty footprint is the whole world: stale once the world moves",
			in:      graph.StandingInput{Evidence: evidence(w1), Current: w2, Now: now},
			want:    graph.StandingStale,
			reasons: []graph.StandingReason{{Change: graph.ReasonWorldChanged}},
		},
		{
			name: "invalid stored pattern counts as a match",
			in: graph.StandingInput{
				Evidence: evidence(w1, "internal/[bad"), Current: w2,
				Changes: []graph.FileChange{modified("README.md")}, Now: now,
			},
			want:    graph.StandingStale,
			reasons: []graph.StandingReason{{Path: "README.md", Change: graph.ChangeModified}},
		},
		{
			name: "past expires_at is expired, even in the same world",
			in:   graph.StandingInput{Evidence: expiring(now.Add(-time.Second), "a.go"), Current: w1, Now: now},
			want: graph.StandingExpired,
		},
		{
			name: "exactly at expires_at is not yet expired",
			in:   graph.StandingInput{Evidence: expiring(now, "a.go"), Current: w1, Now: now},
			want: graph.StandingFresh,
		},
		{
			name: "expiry beats stale",
			in: graph.StandingInput{
				Evidence: expiring(now.Add(-time.Hour), "a.go"), Current: w2,
				Changes: []graph.FileChange{modified("a.go")}, Now: now,
			},
			want: graph.StandingExpired,
		},
		{
			name: "superseded beats everything",
			in: graph.StandingInput{
				Evidence: expiring(now.Add(-time.Hour), "a.go"), SupersededBy: "evd_next", Current: w1, Now: now,
			},
			want: graph.StandingSuperseded,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reasons := graph.EvaluateStanding(tt.in)
			if got != tt.want {
				t.Fatalf("standing = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(reasons, tt.reasons) {
				t.Fatalf("reasons = %+v, want %+v", reasons, tt.reasons)
			}
			if got == graph.StandingStale && len(reasons) == 0 {
				t.Fatal("stale without a reason")
			}
		})
	}
}

func TestEvaluateStandingDoesNotModifyChanges(t *testing.T) {
	changes := []graph.FileChange{
		{Path: "b.go", Status: graph.ChangeModified},
		{Path: "a.go", Status: graph.ChangeModified},
	}
	graph.EvaluateStanding(graph.StandingInput{
		Evidence: specv1.Evidence{World: specv1.WorldHash{1}, Footprint: []string{"**"}},
		Current:  specv1.WorldHash{2},
		Changes:  changes,
		Now:      time.Now(),
	})
	if changes[0].Path != "b.go" || changes[1].Path != "a.go" {
		t.Fatalf("changes reordered to %+v", changes)
	}
}

func TestStandingCounts(t *testing.T) {
	want := map[graph.Standing]bool{
		graph.StandingFresh:      true,
		graph.StandingCarried:    true,
		graph.StandingStale:      false,
		graph.StandingExpired:    false,
		graph.StandingSuperseded: false,
	}
	for s, counts := range want {
		if s.Counts() != counts {
			t.Errorf("%s.Counts() = %t, want %t", s, s.Counts(), counts)
		}
	}
}
