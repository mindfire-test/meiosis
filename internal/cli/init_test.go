package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mindfire-test/meiosis/internal/identity"
	"github.com/mindfire-test/meiosis/pkg/crypto"
)

func initRepo(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "repo")
}

func TestInitBootstrapsRepository(t *testing.T) {
	root := initRepo(t)

	output := execute(t, "init", "--repo", root, "--ide", "none")

	for _, path := range []string{
		".meiosis/config.yaml",
		".meiosis/human.private.key",
		".meiosis/human.public.key",
		".meiosis/agent.private.key",
		".agents/meiosis-capability.json",
		".agents/AGENTS.md",
	} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("init did not create %s: %v", path, err)
		}
	}
	if !strings.Contains(output, "[init] done.") {
		t.Fatalf("output = %q, want init summary", output)
	}

	token, err := identity.Decode([]byte(mustRead(t, filepath.Join(root, ".agents", "meiosis-capability.json"))))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	keys, err := crypto.LoadEncodedKeyPair(mustRead(t, filepath.Join(root, ".meiosis", "human.private.key")), "")
	if err != nil {
		t.Fatalf("LoadEncodedKeyPair() error = %v", err)
	}
	if err := identity.Verify(token, keys.PublicKey, nil, token.IssuedAt); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if token.Principal != "agent:meiosis" {
		t.Fatalf("token.Principal = %q, want agent:meiosis", token.Principal)
	}
}

func TestInitIsIdempotent(t *testing.T) {
	root := initRepo(t)
	execute(t, "init", "--repo", root, "--ide", "none")

	firstPriv := mustRead(t, filepath.Join(root, ".meiosis", "human.private.key"))
	firstToken := mustRead(t, filepath.Join(root, ".agents", "meiosis-capability.json"))

	output := execute(t, "init", "--repo", root, "--ide", "none")
	if !strings.Contains(output, "exists:") {
		t.Fatalf("second run output = %q, want only skips", output)
	}
	if got := mustRead(t, filepath.Join(root, ".meiosis", "human.private.key")); got != firstPriv {
		t.Fatal("second run regenerated the human private key")
	}
	if got := mustRead(t, filepath.Join(root, ".agents", "meiosis-capability.json")); got != firstToken {
		t.Fatal("second run reissued the capability token")
	}
}

func TestInitForceRegeneratesSecrets(t *testing.T) {
	root := initRepo(t)
	execute(t, "init", "--repo", root, "--ide", "none")

	firstToken := mustRead(t, filepath.Join(root, ".agents", "meiosis-capability.json"))

	execute(t, "init", "--repo", root, "--ide", "none", "--force")
	if got := mustRead(t, filepath.Join(root, ".agents", "meiosis-capability.json")); got == firstToken {
		t.Fatal("--force did not reissue the capability token")
	}
}

func TestInitDryRunWritesNothing(t *testing.T) {
	root := initRepo(t)
	output := execute(t, "init", "--repo", root, "--ide", "none", "--dry-run")

	if !strings.Contains(output, "[init] dry run") {
		t.Fatalf("output = %q, want dry-run summary", output)
	}
	if _, err := os.Stat(root); err != nil {
		return
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dry-run wrote files: %v", names)
	}
}

func TestInitScopesBootstrapToken(t *testing.T) {
	root := initRepo(t)
	execute(t, "init", "--repo", root, "--ide", "none",
		"--agent", "agent:impl-7",
		"--issuer", "human:lakin",
		"--allow", "src/**",
		"--deny", "src/secret/**",
	)

	token, err := identity.Decode([]byte(mustRead(t, filepath.Join(root, ".agents", "meiosis-capability.json"))))
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if token.Principal != "agent:impl-7" || token.IssuedBy != "human:lakin" {
		t.Fatalf("token = principal %q issued by %q, want impl-7/lakin", token.Principal, token.IssuedBy)
	}
	if len(token.Scope.Allow) != 1 || token.Scope.Allow[0] != "src/**" {
		t.Fatalf("scope.Allow = %v, want [src/**]", token.Scope.Allow)
	}
	if len(token.Scope.Deny) != 1 || token.Scope.Deny[0] != "src/secret/**" {
		t.Fatalf("scope.Deny = %v, want [src/secret/**]", token.Scope.Deny)
	}
}

func TestInitSkipsUnsupportedIDE(t *testing.T) {
	root := initRepo(t)
	command := New(nil)
	var output strings.Builder
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"init", "--repo", root, "--ide", "intellij"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute() expected an error for an unsupported IDE")
	}
}

func TestInitWritesVSCodeMCPConfig(t *testing.T) {
	root := initRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll(bin) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", initServerName), []byte("#!/bin/sh\nexit 0"), 0o755); err != nil {
		t.Fatalf("WriteFile(bin/meiosisd) error = %v", err)
	}
	execute(t, "init", "--repo", root, "--ide", "vscode")

	if _, err := os.Stat(filepath.Join(root, ".vscode", "mcp.json")); err != nil {
		t.Fatalf("init did not create .vscode/mcp.json: %v", err)
	}

	raw := mustRead(t, filepath.Join(root, ".vscode", "mcp.json"))
	for _, want := range []string{
		`"meiosisd"`,
		`"type": "stdio"`,
		`"-principal"`,
		`"-db"`,
		".meiosis/meiosisd.db",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("mcp.json missing %q: %s", want, raw)
		}
	}
	if want := filepath.Join(root, "bin", initServerName); !strings.Contains(raw, want) {
		t.Fatalf("mcp.json command should point at local %s, got: %s", want, raw)
	}
}

func TestInitWritesAGentsMDPointers(t *testing.T) {
	root := initRepo(t)
	execute(t, "init", "--repo", root, "--ide", "none")

	md := mustRead(t, filepath.Join(root, ".agents", "AGENTS.md"))
	if !strings.Contains(md, "intent_create") || !strings.Contains(md, "capability_token") {
		t.Fatalf("AGENTS.md missing tool/field guidance: %q", md)
	}
	if !strings.Contains(md, ".agents/meiosis-capability.json") {
		t.Fatalf("AGENTS.md should point at the capability token: %q", md)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return string(data)
}
