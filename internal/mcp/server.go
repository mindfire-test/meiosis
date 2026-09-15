package mcp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/mindfire-test/meiosis/internal/graph"
	"github.com/mindfire-test/meiosis/internal/identity"
)

// ToolHandler executes one MCP tool call and returns its result (marshaled
// as the tool's structured content) or an error.
type ToolHandler func(ctx context.Context, s *Server, args json.RawMessage) (any, error)

type tool struct {
	description string
	inputSchema map[string]any
	handler     ToolHandler
}

// Server serves the MCP protocol's JSON-RPC 2.0 surface over any
// io.Reader/io.Writer transport (stdio, or one accepted local-socket
// connection). One Server signs everything it creates as a single
// configured Principal, matching the normal MCP pattern of an agent runtime
// spawning its own meiosisd subprocess for the lifetime of its session.
type Server struct {
	Graph     *graph.Store
	Principal string
	Key       ed25519.PrivateKey

	// IssuerKey, when set, enables capability-token enforcement (FR-1.3's
	// "local interception pipeline"): every tool call must then carry a
	// capability_token signed by this key, verified (and, where a tool acts
	// on a specific path, scope-checked) before its handler runs. Left nil,
	// enforcement is disabled and tool calls proceed unauthenticated, as
	// they did before capability tokens existed.
	IssuerKey ed25519.PublicKey
	// Revocation is consulted during that check, if set; a nil checker
	// treats no token as revoked.
	Revocation identity.RevocationChecker

	tools map[string]tool
}

// NewServer returns a Server ready to serve requests, signing everything it
// creates with key on behalf of principal.
func NewServer(g *graph.Store, principal string, key ed25519.PrivateKey) *Server {
	s := &Server{Graph: g, Principal: principal, Key: key}
	s.registerTools()
	return s
}

// Serve reads newline-delimited JSON-RPC 2.0 requests from r and writes
// responses to w until r is exhausted, ctx is done, or a write fails.
func (s *Server) Serve(ctx context.Context, r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	encoder := json.NewEncoder(w)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		resp := s.handleLine(ctx, line)
		if resp == nil {
			continue // notifications get no response
		}
		if err := encoder.Encode(resp); err != nil {
			return fmt.Errorf("mcp: write response: %w", err)
		}
	}
	return scanner.Err()
}

func (s *Server) handleLine(ctx context.Context, line []byte) *Response {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return &Response{JSONRPC: "2.0", Error: &ResponseError{Code: ErrCodeParseError, Message: err.Error()}}
	}
	if len(req.ID) == 0 {
		return nil
	}

	result, err := s.dispatch(ctx, req)
	if err != nil {
		return &Response{JSONRPC: "2.0", ID: req.ID, Error: toResponseError(err)}
	}
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) dispatch(ctx context.Context, req Request) (any, error) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return s.handleToolsList(), nil
	case "tools/call":
		return s.handleToolsCall(ctx, req.Params)
	default:
		return nil, &rpcError{code: ErrCodeMethodNotFound, message: "method not found: " + req.Method}
	}
}

func toResponseError(err error) *ResponseError {
	var re *rpcError
	if errors.As(err, &re) {
		return &ResponseError{Code: re.code, Message: re.message}
	}
	return &ResponseError{Code: ErrCodeInternal, Message: err.Error()}
}

func (s *Server) handleInitialize() any {
	return map[string]any{
		"protocolVersion": "2025-06-18",
		"serverInfo":      map[string]any{"name": "meiosisd", "version": "0.1.0"},
		"capabilities":    map[string]any{"tools": map[string]any{}},
	}
}

func (s *Server) handleToolsList() any {
	tools := make([]map[string]any, 0, len(s.tools))
	for name, t := range s.tools {
		tools = append(tools, map[string]any{
			"name":        name,
			"description": t.description,
			"inputSchema": t.inputSchema,
		})
	}
	return map[string]any{"tools": tools}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolsCall(ctx context.Context, raw json.RawMessage) (any, error) {
	var params toolCallParams
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &rpcError{code: ErrCodeInvalidParams, message: "invalid tools/call params: " + err.Error()}
	}
	t, ok := s.tools[params.Name]
	if !ok {
		return nil, &rpcError{code: ErrCodeInvalidParams, message: "unknown tool: " + params.Name}
	}

	result, err := t.handler(ctx, s, params.Arguments)
	if err != nil {
		// Per MCP convention, a tool's own failure is reported inside the
		// result (isError: true), not as a JSON-RPC-level error — the RPC
		// call itself succeeded, the tool just didn't.
		return callToolErrorResult(err), nil
	}
	return callToolResult(result), nil
}

func callToolResult(v any) map[string]any {
	text, _ := json.Marshal(v)
	return map[string]any{
		"content":           []map[string]any{{"type": "text", "text": string(text)}},
		"structuredContent": v,
	}
}

// requireCapabilityToken is the MCP daemon's interception point for FR-1.3
// capability tokens. If capability-token enforcement is disabled
// (s.IssuerKey == nil), it always succeeds. Otherwise it decodes the
// capability_token field carried alongside a tool's own arguments and
// verifies it (signature, expiry, revocation). When path is non-empty, the
// token's own scope is additionally checked against that path — a second,
// independent check from whatever internal/scope evaluates against the
// intent's own declared scope, matching identity.Authorize's documented
// expectation that "a real write path is expected to run both". Tools that
// don't act on one specific path (e.g. intent_create) pass an empty path to
// skip that half of the check.
func (s *Server) requireCapabilityToken(args json.RawMessage, path string) error {
	if s.IssuerKey == nil {
		return nil
	}
	var envelope struct {
		CapabilityToken json.RawMessage `json:"capability_token"`
	}
	if err := json.Unmarshal(args, &envelope); err != nil {
		return &rpcError{code: ErrCodeInvalidParams, message: "invalid params: " + err.Error()}
	}
	if len(envelope.CapabilityToken) == 0 {
		return &rpcError{code: ErrCodeInvalidParams, message: "capability_token is required"}
	}
	token, err := identity.Decode(envelope.CapabilityToken)
	if err != nil && json.Valid(envelope.CapabilityToken) {
		// Some clients send the token object serialized as a JSON string
		// rather than embedded. Accept that form: re-parse the string.
		var quoted string
		if umErr := json.Unmarshal(envelope.CapabilityToken, &quoted); umErr == nil {
			token, err = identity.Decode([]byte(quoted))
		}
	}
	if err != nil {
		return &rpcError{code: ErrCodeInvalidParams, message: "capability_token: " + err.Error()}
	}
	if path == "" {
		if err := identity.Verify(token, s.IssuerKey, s.Revocation, time.Now()); err != nil {
			return &rpcError{code: ErrCodeInvalidParams, message: err.Error()}
		}
		return nil
	}
	if err := identity.Authorize(token, s.IssuerKey, s.Revocation, path, time.Now()); err != nil {
		return &rpcError{code: ErrCodeInvalidParams, message: err.Error()}
	}
	return nil
}

func callToolErrorResult(err error) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": err.Error()}},
		"isError": true,
	}
}
