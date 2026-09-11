// Package scope evaluates a file path against an Intent's declared Allow/Deny
// glob rules (FR-3.1, spec/v1.Scope).
//
// This is a placeholder implementation for issue #11
// (https://github.com/mindfire-test/meiosis/issues/11), which owns the
// canonical internal/scope engine and is tracked separately. It's built here
// because issue #16 (the MCP daemon) needs a working scope check for its
// intent_check_path tool and #11 hadn't landed yet. Reconcile — most likely
// by deleting this package and re-pointing internal/mcp at the real one —
// once #11's PR merges.
package scope

import (
	"github.com/bmatcuk/doublestar/v4"

	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// Engine evaluates paths against one Intent's Scope.
type Engine struct {
	scope specv1.Scope
}

// New returns an Engine bound to the given scope.
func New(scope specv1.Scope) Engine {
	return Engine{scope: scope}
}

// IsPathAllowed reports whether path is permitted by the engine's scope.
// Deny rules are checked first and take immediate precedence over Allow
// rules, matching FR-3.1. A path that matches no Allow rule is rejected by
// default — there is no implicit allow.
func (e Engine) IsPathAllowed(path string) (bool, error) {
	for _, pattern := range e.scope.Deny {
		matched, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, err
		}
		if matched {
			return false, nil
		}
	}
	for _, pattern := range e.scope.Allow {
		matched, err := doublestar.Match(pattern, path)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
