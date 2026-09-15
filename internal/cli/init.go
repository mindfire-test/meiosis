package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mindfire-test/meiosis/internal/identity"
	"github.com/mindfire-test/meiosis/pkg/crypto"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// bootstrapIntent is the placeholder intent ID stamped on tokens issued by
// `mei init`. The daemon does not resolve it: a capability token is a grant,
// not a reference to a persisted intent, and the real intent's scope is still
// enforced separately when it is created over MCP.
const bootstrapIntent = "int_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// initTokenTTL is how long the bootstrap capability token lives. Longer than
// identity.DefaultTTL so a freshly initialized repo does not leave its agent
// locked out the next morning, but still finite for the same reason.
const initTokenTTL = 30 * 24 * time.Hour

// initDefaults keeps the few tunables of the bootstrap in one place.
const (
	initIDEDirName = ".gemini/antigravity"
	initServerName = "meiosisd"
)

type initOptions struct {
	repo   string
	ide    string
	agent  string
	issuer string
	force  bool
	dryRun bool
	ttl    time.Duration
	allow  []string
	deny   []string
}

// newInitCommand builds the "init" command tree: repo-local bootstrap
// (identity keys, capability token, .agents/AGENTS.md) plus, optionally,
// registration of meiosisd as an MCP server in a local IDE's global config.
// It is the git-init equivalent for a meiosis repo: one command, idempotent,
// nothing for the user to hand-edit.
func newInitCommand(settings *Settings) *cobra.Command {
	opts := &initOptions{ttl: initTokenTTL}

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a repository: identity keys, capability token, agent instructions, IDE MCP registration",
		Long: `init wires meiosis into a repository and the local agent runtime.

It generates fresh identity keys when none exist, issues a capability token
scoping the agent principal, writes .agents/AGENTS.md that any agent loads to
learn how to call the MCP tools, and registers meiosisd in the configured
IDE's global MCP config (currently: the Antigravity IDE).

Run it again to repair a partial setup. --force regenerates secrets.
`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.repo == "" {
				opts.repo = settings.Repo
			}
			return runInit(cmd.OutOrStdout(), opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.repo, "repo", "", "repository to bootstrap (default: configured repo)")
	flags.StringVar(&opts.ide, "ide", "", "IDE to register meiosisd in: vscode, antigravity, none, or empty to auto-detect (default: auto-detect)")
	flags.StringVar(&opts.agent, "agent", "", "agent principal the capability is granted to (default: agent:meiosis)")
	flags.StringVar(&opts.issuer, "issuer", "", "issuing human principal (default: human:<username>)")
	flags.BoolVar(&opts.force, "force", false, "regenerate secrets and overwrite generated files")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "print what would be written without writing anything")
	flags.DurationVar(&opts.ttl, "ttl", initTokenTTL, "capability token lifetime")
	flags.StringArrayVar(&opts.allow, "allow", nil, "glob the bootstrap token may touch (repeatable; default **)")
	flags.StringArrayVar(&opts.deny, "deny", nil, "glob the bootstrap token may not touch, even if allowed (repeatable)")

	return cmd
}

func runInit(out io.Writer, opts *initOptions) error {
	root, err := filepath.Abs(opts.repo)
	if err != nil {
		return fmt.Errorf("init: resolve repo path: %w", err)
	}

	agent := opts.agent
	if agent == "" {
		agent = "agent:meiosis"
	}
	issuer := opts.issuer
	if issuer == "" {
		name, nameErr := defaultHumanName()
		if nameErr != nil {
			return nameErr
		}
		issuer = "human:" + name
	}

	meiosisDir := filepath.Join(root, ".meiosis")
	agentsDir := filepath.Join(root, ".agents")

	humanPriv := filepath.Join(meiosisDir, "human.private.key")
	humanPub := filepath.Join(meiosisDir, "human.public.key")
	agentPriv := filepath.Join(meiosisDir, "agent.private.key")
	tokenPath := filepath.Join(agentsDir, "meiosis-capability.json")
	agentsMD := filepath.Join(agentsDir, "AGENTS.md")

	writer := &bootstrapWriter{out: out, dry: opts.dryRun}

	if writeErr := writeConfigFile(writer, filepath.Join(meiosisDir, "config.yaml"), opts.force); writeErr != nil {
		return writeErr
	}

	humanKeys, err := ensureKeyPair(writer, humanPriv, humanPub, opts.force)
	if err != nil {
		return err
	}
	agentKeys, err := ensureKeyPair(writer, agentPriv, "", opts.force)
	if err != nil {
		return err
	}
	_ = agentKeys

	token, err := ensureToken(writer, tokenPath, humanKeys, opts, agent, issuer)
	if err != nil {
		return err
	}

	if agentsErr := writeAgentsMD(writer, agentsMD, tokenPath, agentsDir, opts.force, opts); agentsErr != nil {
		return agentsErr
	}

	binPath, err := ensureDaemonBinary(writer, root)
	if err != nil {
		return err
	}

	if err := writeIDEMCPConfig(writer, opts, homeDir(), root, binPath, agent, agentPriv, humanPub); err != nil {
		return err
	}

	printInitSummary(out, opts.dryRun, opts.ide, root, issuer, agent, tokenPath, token)
	return nil
}

type bootstrapWriter struct {
	out io.Writer
	dry bool
}

func (w *bootstrapWriter) write(path string, data []byte, perm os.FileMode, force bool) error {
	if _, err := os.Stat(path); err == nil && !force {
		_, _ = fmt.Fprintf(w.out, "exists:  %s\n", relHome(path))
		return nil
	}
	if w.dry {
		_, _ = fmt.Fprintf(w.out, "would write: %s\n", relHome(path))
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("init: create directory for %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("init: write %s: %w", path, err)
	}
	_, _ = fmt.Fprintf(w.out, "wrote:   %s\n", relHome(path))
	return nil
}

func writeConfigFile(w *bootstrapWriter, path string, force bool) error {
	content := "repo: .\nformat: text\nverbose: false\n"
	if _, err := os.Stat(path); err == nil {
		if !force {
			_, _ = fmt.Fprintf(w.out, "exists:  %s\n", relHome(path))
			return nil
		}
		_, _ = fmt.Fprintf(w.out, "would rewrite: %s\n", relHome(path))
		if !w.dry {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return fmt.Errorf("init: rewrite %s: %w", path, err)
			}
			_, _ = fmt.Fprintf(w.out, "wrote:   %s\n", relHome(path))
		}
		return nil
	}
	return w.write(path, []byte(content), 0o644, force)
}

func ensureKeyPair(w *bootstrapWriter, privPath, pubPath string, force bool) (crypto.KeyPair, error) {
	if data, err := os.ReadFile(privPath); err == nil && !force {
		keys, err := crypto.LoadEncodedKeyPair(string(data), "")
		if err != nil {
			return crypto.KeyPair{}, fmt.Errorf("init: load existing key %s: %w", privPath, err)
		}
		_, _ = fmt.Fprintf(w.out, "exists:  %s\n", relHome(privPath))
		return keys, nil
	}
	if w.dry {
		_, _ = fmt.Fprintf(w.out, "would write: %s\n", relHome(privPath))
		return crypto.KeyPair{}, nil
	}
	keys, err := crypto.GenerateKeyPair()
	if err != nil {
		return crypto.KeyPair{}, fmt.Errorf("init: generate keypair: %w", err)
	}
	if err := w.write(privPath, []byte(keys.PrivateKeyBase64()), 0o600, force); err != nil {
		return crypto.KeyPair{}, err
	}
	if pubPath != "" {
		if err := w.write(pubPath, []byte(keys.PublicKeyBase64()), 0o644, force); err != nil {
			return crypto.KeyPair{}, err
		}
	}
	return keys, nil
}

func ensureToken(w *bootstrapWriter, tokenPath string, human crypto.KeyPair, opts *initOptions, agent, issuer string) (specv1.CapabilityToken, error) {
	if data, err := os.ReadFile(tokenPath); err == nil && !opts.force {
		token, err := identity.Decode(data)
		if err != nil {
			return specv1.CapabilityToken{}, fmt.Errorf("init: decode existing token %s: %w", tokenPath, err)
		}
		_, _ = fmt.Fprintf(w.out, "exists:  %s\n", relHome(tokenPath))
		return token, nil
	}

	if opts.dryRun {
		_, _ = fmt.Fprintf(w.out, "would write: %s\n", relHome(tokenPath))
		return specv1.CapabilityToken{}, nil
	}

	allow := opts.allow
	if len(allow) == 0 {
		allow = []string{"**"}
	}
	token, err := identity.Issue(identity.IssueParams{
		Principal: agent,
		Intent:    bootstrapIntent,
		IssuedBy:  issuer,
		Scope:     specv1.Scope{Allow: allow, Deny: opts.deny, Mode: specv1.ScopeModeEnforce},
		TTL:       opts.ttl,
	}, human.PrivateKey)
	if err != nil {
		return specv1.CapabilityToken{}, fmt.Errorf("init: issue capability token: %w", err)
	}

	encoded, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return specv1.CapabilityToken{}, fmt.Errorf("init: encode token: %w", err)
	}
	if err := w.write(tokenPath, encoded, 0o600, opts.force); err != nil {
		return specv1.CapabilityToken{}, err
	}
	return token, nil
}

func writeAgentsMD(w *bootstrapWriter, path, tokenPath, agentsDir string, force bool, opts *initOptions) error {
	allow := strings.Join(opts.allow, ", ")
	if allow == "" {
		allow = "**"
	}
	deny := strings.Join(opts.deny, ", ")
	if deny == "" {
		deny = "(none)"
	}
	scope := fmt.Sprintf("allows: %s ; denies: %s", allow, deny)

	// Written with a {{TOKEN_FIELD}} placeholder because the rendered text
	// contains backticks, which cannot appear inside a raw string literal.
	content := fmt.Sprintf(`# Meiosis MCP tools

This repository is governed by meiosis — source control where evidence
expires. The daemon (meiosisd) exposes its tools over MCP; any agent in this
workspace may call them.

## MCP server

- server: %s (registered in the IDE global MCP config)
- tools: intent_create, intent_check_path, evidence_submit

## Mandatory rule: every tool call must carry a capability token

The daemon runs with capability enforcement ON. A call whose arguments object
lacks a {{TOKEN_FIELD}} field is rejected. Read the token from
%s (relative to the repo root) and embed it as a JSON object, not a string.

For intent_check_path and evidence_submit the token must also cover the exact
path/paths you pass. This token's scope — %s — is enforced there and at any
real write path. If a call is rejected for a path, do not retry: it is out of
scope by design. Ask the human to issue a wider token with:
    mei intent authorize --issuer human:<you> --allow '<glob>' ...

## Example

Call intent_create with:

{
  "repo": "%s",
  "title": "Refactor UI component",
  "goal": "Update the Button component safely",
  "acceptance": [{"text": "Component tests pass"}],
  "scope": {"allow": ["src/components/**"], "deny": ["pkg/auth/**"], "mode": "enforce"},
  "{{TOKEN_FIELD}}": <object from %s>
}
`, initServerName, relHome(tokenPath), scope, ".", tokenPath)
	content = strings.ReplaceAll(content, "{{TOKEN_FIELD}}", "`capability_token`")

	if _, err := os.Stat(path); err == nil && !force {
		_, _ = fmt.Fprintf(w.out, "exists:  %s\n", relHome(path))
		return nil
	}
	return w.write(path, []byte(content), 0o644, force)
}

// ensureDaemonBinary builds bin/meiosisd when it is missing and a Go module
// is present. A missing toolchain (or no module, e.g. in tests) skips build.
func ensureDaemonBinary(w *bootstrapWriter, root string) (string, error) {
	binPath := filepath.Join(root, "bin", initServerName)
	if fi, err := os.Stat(binPath); err == nil && fi.Mode().IsRegular() {
		_, _ = fmt.Fprintf(w.out, "exists:  %s\n", relHome(binPath))
		return binPath, nil
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		if resolved, err := exec.LookPath(initServerName); err == nil {
			abs, err := filepath.Abs(resolved)
			if err != nil {
				abs = resolved
			}
			_, _ = fmt.Fprintf(w.out, "using:   %s (installed binary)\n", relHome(abs))
			return abs, nil
		}
		return "", fmt.Errorf("init: %s not found in %s or on PATH — run `make install` before init in a non-Go directory",
			initServerName, relHome(filepath.Join(root, "bin")))
	}
	if w.dry {
		_, _ = fmt.Fprintf(w.out, "would build: %s\n", relHome(binPath))
		return binPath, nil
	}
	_, _ = fmt.Fprintf(w.out, "building %s...\n", relHome(binPath))
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/meiosisd")
	cmd.Dir = root
	cmd.Stdout = w.out
	cmd.Stderr = w.out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("init: build meiosisd: %w (run `make build` to get the binaries)", err)
	}
	_, _ = fmt.Fprintf(w.out, "wrote:   %s\n", relHome(binPath))
	return binPath, nil
}

func writeIDEMCPConfig(w *bootstrapWriter, opts *initOptions, home, root, binPath, agent, agentPriv, humanPub string) error {
	ide := opts.ide

	if ide == "" {
		antigravityDir := filepath.Join(home, initIDEDirName)
		if fi, err := os.Stat(antigravityDir); err == nil && fi.IsDir() {
			ide = "antigravity"
		} else if _, err := os.Stat(filepath.Join(root, ".vscode")); err == nil {
			ide = "vscode"
		} else {
			_, _ = fmt.Fprintf(w.out, "no IDE detected — rerun with --ide vscode or --ide antigravity to register\n")
			return nil
		}
	}
	if ide == "none" {
		_, _ = fmt.Fprintln(w.out, "skipping IDE MCP registration (--ide none)")
		return nil
	}

	switch ide {
	case "antigravity":
		return writeAntigravityConfig(w, opts, home, root, binPath, agent, agentPriv, humanPub)
	case "vscode":
		return writeVSCodeConfig(w, opts, root, binPath, agent, agentPriv, humanPub)
	default:
		return fmt.Errorf("init: unsupported IDE %q (supported: vscode, antigravity)", ide)
	}
}

func writeAntigravityConfig(w *bootstrapWriter, opts *initOptions, home, root, binPath, agent, agentPriv, humanPub string) error {
	ideDir := filepath.Join(home, initIDEDirName)
	cfgPath := filepath.Join(ideDir, "mcp_config.json")
	server := map[string]any{
		"command": binPath,
		"args": []string{
			"-principal", agent,
			"-key", agentPriv,
			"-db", filepath.Join(root, ".meiosis", "meiosisd.db"),
			"-issuer-pubkey", humanPub,
		},
	}

	cfg := map[string]any{"mcpServers": map[string]any{}}
	if existing, err := os.ReadFile(cfgPath); err == nil && len(existing) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(existing, &parsed); err != nil {
			return fmt.Errorf("init: %s exists but is not valid JSON — merge the server manually: %w", cfgPath, err)
		}
		cfg = parsed
	}
	servers, ok := cfg["mcpServers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	servers[initServerName] = server
	cfg["mcpServers"] = servers

	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("init: encode MCP config: %w", err)
	}
	return w.write(cfgPath, encoded, 0o644, opts.force)
}

func writeVSCodeConfig(w *bootstrapWriter, opts *initOptions, root, binPath, agent, agentPriv, humanPub string) error {
	vscodeDir := filepath.Join(root, ".vscode")
	cfgPath := filepath.Join(vscodeDir, "mcp.json")

	dbPath := filepath.Join(".meiosis", "meiosisd.db")

	server := map[string]any{
		"type":    "stdio",
		"command": binPath,
		"args": []string{
			"-principal", agent,
			"-key", agentPriv,
			"-db", dbPath,
			"-issuer-pubkey", humanPub,
		},
	}

	cfg := map[string]any{"inputs": []any{}, "servers": map[string]any{}}
	if existing, err := os.ReadFile(cfgPath); err == nil && len(existing) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(existing, &parsed); err != nil {
			return fmt.Errorf("init: %s exists but is not valid JSON — merge the server manually: %w", cfgPath, err)
		}
		cfg = parsed
	}
	servers, ok := cfg["servers"].(map[string]any)
	if !ok {
		servers = map[string]any{}
	}
	servers[initServerName] = server
	cfg["servers"] = servers

	encoded, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("init: encode MCP config: %w", err)
	}

	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return fmt.Errorf("init: create %s: %w", vscodeDir, err)
	}
	return w.write(cfgPath, encoded, 0o644, opts.force)
}

func printInitSummary(out io.Writer, dry bool, ide, root, issuer, agent, tokenPath string, token specv1.CapabilityToken) {
	if dry {
		_, _ = fmt.Fprint(out, "\n[init] dry run: nothing was written.\n")
		return
	}
	_, _ = fmt.Fprint(out, "\n[init] done.\n")
	_, _ = fmt.Fprintf(out, "  repository : %s\n", relHome(root))
	_, _ = fmt.Fprintf(out, "  agent      : %s  (daemon principal, MCP)\n", agent)
	_, _ = fmt.Fprintf(out, "  issuer     : %s  (human, signs capability tokens)\n", issuer)
	_, _ = fmt.Fprintf(out, "  token      : %s\n", relHome(tokenPath))
	if !token.ExpiresAt.IsZero() {
		_, _ = fmt.Fprintf(out, "  token ttl  : expires %s\n", token.ExpiresAt.Format(time.RFC3339))
	}
	switch ide {
	case "none", "":
		_, _ = fmt.Fprint(out, "\nmeiosisd was not registered with an IDE (use --ide vscode or --ide antigravity).\n")
	case "vscode":
		_, _ = fmt.Fprintf(out, "\nOpen this repository in VS Code: the meiosisd server is registered in\n%s. Approve/enable the MCP server when prompted, then try:\n", relHome(filepath.Join(root, ".vscode", "mcp.json")))
		_, _ = fmt.Fprint(out, "  \"declare an intent to refactor the Button component, then check whether\n")
		_, _ = fmt.Fprint(out, "   pkg/auth/login.go is inside its scope\"\n")
	default:
		_, _ = fmt.Fprint(out, "\nOpen this repository in the Antigravity IDE: the meiosisd server and its\n")
		_, _ = fmt.Fprintf(out, "three tools (intent_create, intent_check_path, evidence_submit) will be\navailable. Approve them when prompted, then try:\n")
		_, _ = fmt.Fprint(out, "  \"declare an intent to refactor the Button component, then check whether\n")
		_, _ = fmt.Fprint(out, "   pkg/auth/login.go is inside its scope\"\n")
	}
}

func defaultHumanName() (string, error) {
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		return sanitizePrincipalPart(u.Username), nil
	}
	if env := os.Getenv("USER"); strings.TrimSpace(env) != "" {
		return sanitizePrincipalPart(env), nil
	}
	return "", fmt.Errorf("init: cannot determine a human principal (pass --issuer)")
}

func sanitizePrincipalPart(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := b.String()
	if out == "" {
		return "user"
	}
	return out
}

func homeDir() string {
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return ""
}

// relHome prints a path as given when it is outside $HOME (the common case
// for repo files) or as ~/... when it is inside it (IDE config, keys).
func relHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if relative, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~/" + relative
	}
	return path
}
