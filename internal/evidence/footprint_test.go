package evidence

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mindfire-test/meiosis/internal/graph"
	meiosiscrypto "github.com/mindfire-test/meiosis/pkg/crypto"
	v1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
	"github.com/mindfire-test/meiosis/pkg/storage"
	"github.com/mindfire-test/meiosis/pkg/storage/sqlite"
)

func TestReportFootprint(t *testing.T) {
	const root = "/home/ci/repo"
	tests := []struct {
		name   string
		format Format
		report string
		root   string
		want   []string
	}{
		{"cobertura relative filenames", FormatCobertura, string(fixture(t, "cobertura.xml")), "", []string{"src/a.go", "src/b.go"}},
		{"lcov relative paths", FormatLCOV, string(fixture(t, "lcov.info")), "", []string{"src/a.go", "src/b.go"}},
		{
			"cobertura joins the single source directory", FormatCobertura,
			cobertura([]string{root + "/internal"}, "auth/token.go", "auth/sign.go"), root,
			[]string{"internal/auth/sign.go", "internal/auth/token.go"},
		},
		{
			"cobertura relative source directory", FormatCobertura,
			cobertura([]string{"pkg"}, "crypto/sign.go"), "",
			[]string{"pkg/crypto/sign.go"},
		},
		{
			"cobertura with several sources is ambiguous", FormatCobertura,
			cobertura([]string{"a", "b"}, "x.go"), "", nil,
		},
		{"lcov absolute paths inside the root", FormatLCOV, lcov(root+"/src/a.go", root+"/src/b.go"), root, []string{"src/a.go", "src/b.go"}},
		{"lcov absolute path without a root", FormatLCOV, lcov(root + "/src/a.go"), "", nil},
		{"lcov path outside the root drops everything", FormatLCOV, lcov(root+"/src/a.go", "/usr/lib/go/src/fmt/print.go"), root, nil},
		{"lcov parent traversal drops everything", FormatLCOV, lcov("src/a.go", "../other/b.go"), "", nil},
		{"lcov file name with glob characters", FormatLCOV, lcov("app/[id]/page.tsx"), "", nil},
		{"sarif without artifacts gives no footprint", FormatSARIF, string(fixture(t, "sarif_warnings.json")), "", nil},
		{
			"sarif artifacts plus result locations", FormatSARIF,
			`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},
				"artifacts":[{"location":{"uri":"internal/auth/token.go","uriBaseId":"%SRCROOT%"}}],
				"results":[{"ruleId":"r","level":"warning","locations":[{"physicalLocation":{"artifactLocation":{"uri":"pkg/crypto/sign.go"}}}]}]}]}`,
			"",
			[]string{"internal/auth/token.go", "pkg/crypto/sign.go"},
		},
		{
			"sarif file URIs and defined base ids", FormatSARIF,
			`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},
				"originalUriBaseIds":{"SRC":{"uri":"file:///home/ci/repo/internal/"}},
				"artifacts":[{"location":{"uri":"auth/token.go","uriBaseId":"SRC"}},{"location":{"uri":"file:///home/ci/repo/main.go"}}],
				"results":[]}]}`,
			root,
			[]string{"internal/auth/token.go", "main.go"},
		},
		{
			"sarif run without artifacts drops everything", FormatSARIF,
			`{"version":"2.1.0","runs":[
				{"tool":{"driver":{"name":"a"}},"artifacts":[{"location":{"uri":"a.go"}}],"results":[]},
				{"tool":{"driver":{"name":"b"}},"results":[]}]}`,
			"", nil,
		},
		{
			"sarif non-file URI drops everything", FormatSARIF,
			`{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"t"}},"artifacts":[{"location":{"uri":"https://example.com/a.go"}}],"results":[]}]}`,
			"", nil,
		},
		{"junit gives no footprint", FormatJUnit, string(fixture(t, "junit_pass.xml")), "", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Parse(tt.format, []byte(tt.report), Options{RepoRoot: tt.root})
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if !reflect.DeepEqual(r.Footprint, tt.want) {
				t.Fatalf("Footprint = %q, want %q", r.Footprint, tt.want)
			}
		})
	}
}

func TestReportFootprintTooLargeIsDropped(t *testing.T) {
	paths := make([]string, v1.MaxFootprintEntries+1)
	for i := range paths {
		paths[i] = fmt.Sprintf("f%d.go", i)
	}
	r, err := Parse(FormatLCOV, []byte(lcov(paths...)), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Footprint != nil {
		t.Fatalf("a footprint over the cap was kept (%d entries)", len(r.Footprint))
	}
}

func TestIngestFootprintAndExpiry(t *testing.T) {
	ctx := context.Background()
	store, attempt, key := newWorldEnv(t, "src/a.go", "src/b.go", "docs/x.md")
	ingest := func(t *testing.T, p IngestParams, report string) v1.Evidence {
		t.Helper()
		p.Attempt, p.Producer = attempt.ID, "agent:ci"
		if p.World == (v1.WorldHash{}) {
			p.World = attempt.World
		}
		ev, _, err := Ingest(ctx, store, p, key, []byte(report))
		if err != nil {
			t.Fatalf("Ingest: %v", err)
		}
		return ev
	}

	t.Run("report files that exist become the footprint", func(t *testing.T) {
		ev := ingest(t, IngestParams{}, lcov("src/a.go", "src/b.go"))
		if want := []string{"src/a.go", "src/b.go"}; !reflect.DeepEqual(ev.Footprint, want) {
			t.Fatalf("Footprint = %q, want %q", ev.Footprint, want)
		}
	})
	t.Run("a file missing from the world drops the report footprint", func(t *testing.T) {
		ev := ingest(t, IngestParams{}, lcov("src/a.go", "src/gone.go"))
		if ev.Footprint != nil {
			t.Fatalf("Footprint = %q, want whole world", ev.Footprint)
		}
	})
	t.Run("caller footprint and report files are merged and normalised", func(t *testing.T) {
		ev := ingest(t, IngestParams{Footprint: []string{"docs/**", "src/a.go"}}, lcov("src/a.go", "src/b.go", "docs/x.md"))
		if want := []string{"docs/**", "src/a.go", "src/b.go"}; !reflect.DeepEqual(ev.Footprint, want) {
			t.Fatalf("Footprint = %q, want %q", ev.Footprint, want)
		}
	})
	t.Run("an unknown world keeps only the caller footprint", func(t *testing.T) {
		ev := ingest(t, IngestParams{World: v1.WorldHash{9}, Footprint: []string{"docs/**"}}, lcov("src/a.go"))
		if want := []string{"docs/**"}; !reflect.DeepEqual(ev.Footprint, want) {
			t.Fatalf("Footprint = %q, want %q", ev.Footprint, want)
		}
	})
	t.Run("expiry is recorded and signed", func(t *testing.T) {
		at := time.Now().UTC().Add(72 * time.Hour).Truncate(time.Second)
		ev := ingest(t, IngestParams{ExpiresAt: at}, string(fixture(t, "junit_pass.xml")))
		if !ev.ExpiresAt.Equal(at) {
			t.Fatalf("ExpiresAt = %v, want %v", ev.ExpiresAt, at)
		}
		stored, err := store.GetEvidence(ctx, ev.ID)
		if err != nil || !stored.ExpiresAt.Equal(at) {
			t.Fatalf("stored ExpiresAt = %v (%v), want %v", stored.ExpiresAt, err, at)
		}
	})
}

// newWorldEnv is newIngestEnv with a stored manifest for the attempt's world,
// so footprint files can be checked against it.
func newWorldEnv(t *testing.T, files ...string) (*graph.Store, v1.Attempt, ed25519.PrivateKey) {
	t.Helper()
	backend, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	store, err := graph.New(backend)
	if err != nil {
		t.Fatal(err)
	}
	attempt := testAttempt()
	manifest := map[string]string{}
	for _, f := range files {
		manifest[f] = strings.Repeat("0", 64)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = backend.Put(ctx, storage.KindWorld, attempt.World.String(), encoded); err != nil {
		t.Fatal(err)
	}
	if err = store.PutAttempt(ctx, attempt); err != nil {
		t.Fatal(err)
	}
	keys, err := meiosiscrypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	return store, attempt, keys.PrivateKey
}

func lcov(paths ...string) string {
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "SF:%s\nDA:1,1\nend_of_record\n", p)
	}
	return b.String()
}

func cobertura(sources []string, filenames ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><coverage><sources>`)
	for _, s := range sources {
		fmt.Fprintf(&b, "<source>%s</source>", s)
	}
	b.WriteString(`</sources><packages><package name="p"><classes>`)
	for _, f := range filenames {
		fmt.Fprintf(&b, `<class name="c" filename="%s"><lines><line number="1" hits="1"/></lines></class>`, f)
	}
	b.WriteString(`</classes></package></packages></coverage>`)
	return b.String()
}
