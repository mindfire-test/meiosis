package scope

import (
	"fmt"
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// IsPathAllowed evaluates whether path is permitted by the Intent.
// A path is allowed if it matches at least one Allow rule and does not match any Deny rule.
// If Allow is empty, every path is denied by default.
func (i *Intent) IsPathAllowed(filePath string) (bool, error) {
	if err := validatePath(filePath); err != nil {
		return false, err
	}

	// Deny rules take precedence. If a path matches any deny rule, block it immediately.
	for _, denyPattern := range i.Deny {
		match, err := doublestar.Match(denyPattern, filePath)
		if err != nil {
			return false, fmt.Errorf("invalid deny pattern '%s': %w", denyPattern, err)
		}
		if match {
			return false, nil
		}
	}

	// In strict security contexts, default is deny. If no Allow rules exist, return false.
	if len(i.Allow) == 0 {
		return false, nil
	}

	// The path must match at least one Allow rule to be permitted.
	for _, allowPattern := range i.Allow {
		match, err := doublestar.Match(allowPattern, filePath)
		if err != nil {
			return false, fmt.Errorf("invalid allow pattern '%s': %w", allowPattern, err)
		}
		if match {
			return true, nil
		}
	}

	return false, nil
}

// IsPathAllowed evaluates filePath using intent's allow and deny rules.
func IsPathAllowed(intent Intent, filePath string) (bool, error) {
	return intent.IsPathAllowed(filePath)
}

// IsAllowed is retained as a compatibility alias for IsPathAllowed.
func (i *Intent) IsAllowed(filePath string) (bool, error) {
	return i.IsPathAllowed(filePath)
}

func validatePath(filePath string) error {
	if filePath == "" || strings.HasPrefix(filePath, "/") || strings.Contains(filePath, "\\") {
		return fmt.Errorf("invalid file path %q: paths must be relative slash-separated paths", filePath)
	}
	clean := path.Clean(filePath)
	if clean != filePath || clean == "." || strings.HasPrefix(clean, "../") || clean == ".." {
		return fmt.Errorf("invalid file path %q: path traversal is not allowed", filePath)
	}
	return nil
}
