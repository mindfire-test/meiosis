package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mindfire-test/meiosis/internal/identity"
	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// newIntentCommand builds the "intent" command tree: the existing stub
// action plus "authorize", which issues signed capability tokens (FR-1.3).
func newIntentCommand(settings *Settings) *cobra.Command {
	intent := newActionCommand("intent", "Manage intents", settings)
	intent.AddCommand(newIntentAuthorizeCommand())
	return intent
}

// newIntentAuthorizeCommand implements "mei intent authorize": it signs a
// capability token with the issuer's Ed25519 key, granting principal
// time-bounded, path-scoped permission to act within intent (FR-1.3/FR-1.4).
// The issuer, not the holder, signs it, so a principal can never mint or
// widen its own authority (FR-1.5).
func newIntentAuthorizeCommand() *cobra.Command {
	var (
		principal     string
		intentID      string
		issuedBy      string
		issuerKeyPath string
		ttl           time.Duration
		mode          string
		out           string
		allow         []string
		deny          []string
	)

	cmd := &cobra.Command{
		Use:   "authorize",
		Short: "Issue a signed capability token scoping a principal's access to an intent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if principal == "" || intentID == "" || issuedBy == "" || issuerKeyPath == "" || len(allow) == 0 {
				return fmt.Errorf("--principal, --intent, --issuer, --issuer-key and at least one --allow are required")
			}

			keyData, err := os.ReadFile(issuerKeyPath)
			if err != nil {
				return fmt.Errorf("read issuer key: %w", err)
			}
			keys, err := crypto.LoadEncodedKeyPair(string(keyData), "")
			if err != nil {
				return fmt.Errorf("load issuer key: %w", err)
			}

			token, err := identity.Issue(identity.IssueParams{
				Principal: principal,
				Intent:    intentID,
				IssuedBy:  issuedBy,
				Scope:     specv1.Scope{Allow: allow, Deny: deny, Mode: specv1.ScopeMode(mode)},
				TTL:       ttl,
			}, keys.PrivateKey)
			if err != nil {
				return fmt.Errorf("issue token: %w", err)
			}

			encoded, err := identity.Encode(token)
			if err != nil {
				return fmt.Errorf("encode token: %w", err)
			}
			if out == "" {
				_, err := fmt.Fprintln(cmd.OutOrStdout(), string(encoded))
				return err
			}
			return os.WriteFile(out, encoded, 0o600)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&principal, "principal", "", "principal the capability is granted to, e.g. agent:impl-3")
	flags.StringVar(&intentID, "intent", "", "intent ID the capability is scoped to")
	flags.StringVar(&issuedBy, "issuer", "", "principal granting the capability, e.g. human:lakin")
	flags.StringVar(&issuerKeyPath, "issuer-key", "", "path to the issuer's Ed25519 private key (base64 or PEM)")
	flags.DurationVar(&ttl, "ttl", identity.DefaultTTL, "how long the token remains valid")
	flags.StringVar(&mode, "mode", string(specv1.ScopeModeEnforce), "scope mode: enforce or warn")
	flags.StringVar(&out, "out", "", "file to write the signed token to (default: stdout)")
	flags.StringArrayVar(&allow, "allow", nil, "glob path the token may touch (repeatable)")
	flags.StringArrayVar(&deny, "deny", nil, "glob path the token may not touch, even if allowed (repeatable)")

	return cmd
}
