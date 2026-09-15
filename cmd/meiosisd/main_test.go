package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mindfire-test/meiosis/pkg/crypto"
)

func TestRunRejectsMissingFlags(t *testing.T) {
	if err := run(nil, strings.NewReader(""), &bytes.Buffer{}); err == nil {
		t.Fatal("run() expected error for missing required flags")
	}
}

func TestRunServesOneStdioRequest(t *testing.T) {
	issuer, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair() error = %v", err)
	}
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "issuer.key")
	if err := os.WriteFile(keyPath, []byte(issuer.PrivateKeyBase64()), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	dbPath := filepath.Join(dir, "meiosisd.db")

	stdin := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n")
	var stdout bytes.Buffer

	if err := run([]string{
		"--principal", "agent:impl-3",
		"--key", keyPath,
		"--db", dbPath,
	}, stdin, &stdout); err != nil {
		t.Fatalf("run() error = %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("json.Unmarshal(stdout) error = %v, output = %q", err, stdout.String())
	}
	if resp["error"] != nil {
		t.Fatalf("unexpected RPC error: %v", resp["error"])
	}
	result, _ := resp["result"].(map[string]any)
	if result["serverInfo"] == nil {
		t.Fatalf("initialize result = %#v, want a serverInfo field", resp["result"])
	}
}
