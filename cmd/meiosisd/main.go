// Command meiosisd is the local MCP daemon (issue #16): it serves
// intent_create, intent_check_path and evidence_submit as MCP tools over
// stdio and, optionally, a local UNIX socket. It is intentionally a thin
// wrapper — internal/mcp owns the protocol, internal/graph owns persistence
// — mirroring cmd/mei's own "thin entrypoint" style.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/mindfire-test/meiosis/internal/graph"
	"github.com/mindfire-test/meiosis/internal/mcp"
	"github.com/mindfire-test/meiosis/pkg/crypto"
	"github.com/mindfire-test/meiosis/pkg/storage/sqlite"
)

// version is stamped at build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "meiosisd:", err)
		os.Exit(1)
	}
}

func run(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("meiosisd", flag.ContinueOnError)
	principal := fs.String("principal", "", "principal this daemon signs as, e.g. agent:impl-3")
	keyPath := fs.String("key", "", "path to this principal's Ed25519 private key (base64 or PEM)")
	dbPath := fs.String("db", "meiosisd.db", "path to the SQLite database")
	socketPath := fs.String("socket", "", "optional UNIX socket path to also listen on, in addition to stdio")
	issuerPubKeyPath := fs.String("issuer-pubkey", "", "path to a trusted issuer's Ed25519 public key (base64 or PEM); when set, every tool call must carry a capability token signed by this issuer (FR-1.3)")
	printVersion := fs.Bool("version", false, "print the meiosisd version and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *printVersion {
		fmt.Fprintln(stdout, "meiosisd "+version)
		return nil
	}
	if *principal == "" || *keyPath == "" {
		return fmt.Errorf("--principal and --key are required")
	}

	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		return fmt.Errorf("read key: %w", err)
	}
	keys, err := crypto.LoadEncodedKeyPair(string(keyData), "")
	if err != nil {
		return fmt.Errorf("load key: %w", err)
	}

	backend, err := sqlite.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open storage: %w", err)
	}
	defer backend.Close()

	server := mcp.NewServer(graph.NewStore(backend), *principal, keys.PrivateKey)

	if *issuerPubKeyPath != "" {
		pubKeyData, err := os.ReadFile(*issuerPubKeyPath)
		if err != nil {
			return fmt.Errorf("read issuer public key: %w", err)
		}
		issuerKey, err := crypto.LoadEncodedPublicKey(string(pubKeyData))
		if err != nil {
			return fmt.Errorf("load issuer public key: %w", err)
		}
		server.IssuerKey = issuerKey
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if *socketPath != "" {
		go func() {
			if err := serveSocket(ctx, server, *socketPath); err != nil {
				fmt.Fprintln(os.Stderr, "meiosisd: socket listener:", err)
			}
		}()
	}

	return server.Serve(ctx, stdin, stdout)
}

// serveSocket accepts connections on a UNIX domain socket at path, serving
// each one with the same JSON-RPC loop stdio uses — this is what satisfies
// the "listens cleanly over stdio or local socket" acceptance criterion
// without a second protocol implementation.
func serveSocket(ctx context.Context, server *mcp.Server, path string) error {
	_ = os.Remove(path) // best-effort: clear a stale socket left by a prior run
	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", path, err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go func() {
			defer conn.Close()
			_ = server.Serve(ctx, conn, conn)
		}()
	}
}
