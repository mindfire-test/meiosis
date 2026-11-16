package graph

import (
	"sort"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// Standing is where one evidence record stands against the current world
// (docs/plan-m2.md §3.5). It is computed on read and never stored.
type Standing string

// Standing states, in the order EvaluateStanding checks them.
const (
	// StandingSuperseded: a later record by the same producer corrects this
	// one (FR-4.12).
	StandingSuperseded Standing = "superseded"
	// StandingExpired: expires_at has passed, whatever the code did (FR-4.11).
	StandingExpired Standing = "expired"
	// StandingFresh: produced against the current world.
	StandingFresh Standing = "fresh"
	// StandingStale: something the evidence depends on changed (FR-4.7).
	StandingStale Standing = "stale"
	// StandingCarried: the world moved, but nothing in the footprint changed.
	StandingCarried Standing = "carried"
)

// Counts reports whether evidence in this standing may satisfy a policy:
// only fresh and carried evidence does.
func (s Standing) Counts() bool {
	return s == StandingFresh || s == StandingCarried
}

// ReasonWorldChanged is the Change of the single reason given when evidence
// with an empty footprint (it depends on the whole world) goes stale.
const ReasonWorldChanged = "world"

// StandingReason explains a stale standing: a path the evidence depends on
// and how it changed (ChangeAdded, ChangeModified, ChangeDeleted), or, for a
// whole-world footprint, an empty Path and ReasonWorldChanged.
type StandingReason struct {
	Path   string `json:"path,omitempty"`
	Change string `json:"change"`
}

// StandingInput is everything EvaluateStanding needs. It holds no store or
// clock, so the same input always gives the same answer.
type StandingInput struct {
	Evidence     specv1.Evidence
	SupersededBy string
	Current      specv1.WorldHash
	Changes      []FileChange
	Now          time.Time
}

// EvaluateStanding classifies one evidence record. The rules are checked in
// order and the first match wins:
//
//  1. superseded: SupersededBy is set
//  2. expired: ExpiresAt is set and Now is after it
//  3. fresh: Evidence.World equals Current
//  4. stale: the footprint is empty (whole world), or an entry matches a
//     changed path
//  5. carried: the world moved, but no footprint entry matches a changed path
//
// Reasons are returned only for stale, sorted by path, and there is always at
// least one.
func EvaluateStanding(in StandingInput) (Standing, []StandingReason) {
	e := in.Evidence
	switch {
	case in.SupersededBy != "":
		return StandingSuperseded, nil
	case !e.ExpiresAt.IsZero() && in.Now.After(e.ExpiresAt):
		return StandingExpired, nil
	case e.World == in.Current:
		return StandingFresh, nil
	case len(e.Footprint) == 0:
		return StandingStale, []StandingReason{{Change: ReasonWorldChanged}}
	}

	var reasons []StandingReason
	for _, c := range in.Changes {
		if footprintMatches(e.Footprint, c.Path) {
			reasons = append(reasons, StandingReason{Path: c.Path, Change: c.Status})
		}
	}
	if len(reasons) == 0 {
		return StandingCarried, nil
	}
	sort.Slice(reasons, func(i, j int) bool { return reasons[i].Path < reasons[j].Path })
	return StandingStale, reasons
}

// footprintMatches reports whether any footprint entry matches path. An exact
// path matches itself as a pattern. An entry that is not a valid pattern
// (possible in records stored before footprints were validated) counts as a
// match: when in doubt, the evidence goes stale rather than being carried.
func footprintMatches(footprint []string, path string) bool {
	for _, entry := range footprint {
		ok, err := doublestar.Match(entry, path)
		if err != nil || ok {
			return true
		}
	}
	return false
}
