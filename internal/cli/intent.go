package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mindfire-test/meiosis/internal/graph"
	"github.com/mindfire-test/meiosis/internal/identity"
	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage/sqlite"
)

// newIntentCommand builds the "intent" command tree: "declare", which signs
// and persists a new intent as the human operator, and "authorize", which
// issues signed capability tokens scoping an agent to that intent (FR-1.3).
func newIntentCommand(settings *Settings) *cobra.Command {
	intent := newActionCommand("intent", "Manage intents", settings)
	intent.AddCommand(newIntentDeclareCommand(settings))
	intent.AddCommand(newIntentAuthorizeCommand())
	return intent
}

// newIntentDeclareCommand implements "mei intent declare <title>": it builds,
// signs and persists a new intent using the repository's human key
// (.meiosis/human.private.key) into the same SQLite store the meiosisd daemon
// serves, so intents declared on the CLI are immediately visible to agents.
func newIntentDeclareCommand(settings *Settings) *cobra.Command {
	var (
		goal       string
		acceptance []string
		files      []string
		deny       []string
		mode       string
		createdBy  string
		keyPath    string
		dbPath     string
	)

	cmd := &cobra.Command{
		Use:     "declare <title>",
		Short:   "Declare a new intent, signed by the repository human key",
		Args:    cobra.ExactArgs(1),
		Example: "  mei intent declare \"Refactor the Button component\" --files \"src/components/Button.tsx\"",
		RunE: func(cmd *cobra.Command, args []string) error {
			title := args[0]
			if strings.TrimSpace(title) == "" {
				return fmt.Errorf("intent title must not be empty")
			}

			root := settings.Repo
			if root == "" {
				root = "."
			}
			absRoot, err := filepath.Abs(root)
			if err != nil {
				return fmt.Errorf("resolve repo path: %w", err)
			}

			if keyPath == "" {
				keyPath = filepath.Join(absRoot, ".meiosis", "human.private.key")
			}
			if dbPath == "" {
				dbPath = filepath.Join(absRoot, ".meiosis", "meiosisd.db")
			}

			keyData, err := os.ReadFile(keyPath)
			if err != nil {
				return fmt.Errorf("read human key %s: %w (run `mei init` to bootstrap the repository)", keyPath, err)
			}
			keys, err := crypto.LoadEncodedKeyPair(string(keyData), "")
			if err != nil {
				return fmt.Errorf("load human key: %w", err)
			}

			store, err := sqlite.Open(dbPath)
			if err != nil {
				return fmt.Errorf("open store %s: %w", dbPath, err)
			}
			defer func() { _ = store.Close() }()
			graphStore, err := graph.New(store)
			if err != nil {
				return fmt.Errorf("open graph: %w", err)
			}

			if goal == "" {
				goal = title
			}
			criteria := make([]specv1.Criterion, 0, len(acceptance))
			for _, text := range acceptance {
				criteria = append(criteria, specv1.Criterion{Text: text})
			}
			if len(criteria) == 0 {
				criteria = append(criteria, specv1.Criterion{Text: title})
			}
			allow := files
			if len(allow) == 0 {
				allow = []string{"**"}
			}
			by := createdBy
			if by == "" {
				name, nameErr := defaultHumanName()
				if nameErr != nil {
					return nameErr
				}
				by = "human:" + name
			}

			intent, createErr := graphStore.CreateIntent(context.Background(), graph.CreateIntentParams{
				Repo:       absRoot,
				Title:      title,
				Goal:       goal,
				Acceptance: criteria,
				Scope:      specv1.Scope{Allow: allow, Deny: deny, Mode: specv1.ScopeMode(mode)},
				CreatedBy:  by,
			}, keys.PrivateKey)
			if createErr != nil {
				return fmt.Errorf("declare intent: %w", createErr)
			}

			if settings.Format == "json" {
				payload := map[string]any{
					"id":     intent.ID,
					"title":  intent.Title,
					"goal":   intent.Goal,
					"repo":   intent.Repo,
					"scope":  intent.Scope,
					"status": intent.Status,
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(payload)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "intent %s declared and signed: %s\n  scope: allow %s (mode %s)%s\n",
				intent.ID, intent.Title, strings.Join(allow, ", "), intent.Scope.Mode, denyClause(deny))
			return err
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&goal, "goal", "", "the change's goal (default: the title)")
	flags.StringArrayVar(&acceptance, "acceptance", nil, "acceptance criterion (repeatable; default: the title)")
	flags.StringArrayVar(&files, "files", nil, "path/glob the intents lets agents touch (repeatable; default: **)")
	flags.StringArrayVar(&deny, "deny", nil, "path/glob agents may not touch even if allowed (repeatable)")
	flags.StringVar(&mode, "mode", string(specv1.ScopeModeEnforce), "scope mode: enforce or warn")
	flags.StringVar(&createdBy, "created-by", "", "declaring principal (default: human:<username>)")
	flags.StringVar(&keyPath, "key", "", "path to the human signing key (default: <repo>/.meiosis/human.private.key)")
	flags.StringVar(&dbPath, "db", "", "path to the store database (default: <repo>/.meiosis/meiosisd.db)")

	return cmd
}

func denyClause(deny []string) string {
	if len(deny) == 0 {
		return ""
	}
	return "; deny " + strings.Join(deny, ", ")
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
