package scope

// Intent defines the boundaries for codebase edits declared by an agent or human
// before any files are modified. Allow and Deny use doublestar glob syntax (e.g. pkg/auth/**).
// Deny rules always take precedence over Allow rules.
type Intent struct {
	// Allow is the list of glob patterns that are permitted to be modified.
	Allow []string
	// Deny is the list of glob patterns explicitly forbidden from modification.
	Deny []string
}
