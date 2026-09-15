package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mindfire-test/meiosis/internal/graph"
	"github.com/mindfire-test/meiosis/internal/scope"
	specv1 "github.com/mindfire-test/meiosis/pkg/spec/v1"
)

// mcpCapabilityTokenProperty is the JSON Schema fragment listing the
// capability_token field. The daemon requires it on every tool call while
// enforcement is on, so it is declared as required in each tool schema. It
// accepts both an embedded object (the daemon's preferred form, per
// AGENTS.md) and a serialized JSON string: agents routinely stringify the
// token, and letting those calls reach the daemon yields a clear corrective
// error instead of a client-side schema rejection that aborts the agent turn.
const mcpCapabilityTokenProperty = `"capability_token":{"description":"capability token granting this call (see .agents/meiosis-capability.json)","type":["object","string"]}`

// toolInputSchema parses an authored JSON Schema string. These literals are
// written by hand at compile time and must stay valid JSON, so a parse
// failure panics rather than silently serving a null schema.
func toolInputSchema(s string) map[string]any {
	var schema map[string]any
	if err := json.Unmarshal([]byte(s), &schema); err != nil {
		panic("mcp: invalid tool input schema: " + err.Error())
	}
	return schema
}

func (s *Server) registerTools() {
	s.tools = map[string]tool{
		"intent_create": {
			description: "Declare a new intent (goal, acceptance criteria, scope) and receive a signed Intent.",
			inputSchema: toolInputSchema(`{
				"type": "object",
				"properties": {
					"repo":       {"type": "string", "description": "repository this intent applies to"},
					"title":      {"type": "string"},
					"goal":       {"type": "string"},
					"acceptance": {"type": "array", "items": {"type": "object", "properties": {"text": {"type": "string"}, "check": {"type": "object"}}, "required": ["text"]}},
					"scope":      {"type": "object", "properties": {"allow": {"type": "array", "items": {"type": "string"}}, "deny": {"type": "array", "items": {"type": "string"}}, "mode": {"type": "string", "enum": ["enforce", "warn"]}}, "required": ["allow", "mode"]},
					` + mcpCapabilityTokenProperty + `
				},
				"required": ["repo", "title", "goal", "acceptance", "scope", "capability_token"]
			}`),
			handler: handleIntentCreate,
		},
		"intent_check_path": {
			description: "Check whether a path is allowed under an existing intent's declared scope.",
			inputSchema: toolInputSchema(`{
				"type": "object",
				"properties": {
					"intent": {"type": "string", "description": "intent ID returned by intent_create"},
					"path":   {"type": "string", "description": "repo-relative path to check against the intent's scope"},
					` + mcpCapabilityTokenProperty + `
				},
				"required": ["intent", "path", "capability_token"]
			}`),
			handler: handleIntentCheckPath,
		},
		"evidence_submit": {
			description: "Submit a signed evidence record (e.g. a test run's outcome) bound to a world hash.",
			inputSchema: toolInputSchema(`{
				"type": "object",
				"properties": {
					"attempt":   {"type": "string", "description": "attempt ID the evidence is bound to"},
					"producer":  {"type": "string"},
					"world":     {"type": "string", "description": "world hash the evidence was produced against"},
					"kind":      {"type": "string", "enum": ["test-run", "coverage", "type-check", "static-analysis", "benchmark", "mutation"]},
					"outcome":   {"type": "string", "enum": ["pass", "fail", "inconclusive"]},
					"payload":   {"type": "object"},
					"footprint": {"type": "array", "items": {"type": "string"}},
					` + mcpCapabilityTokenProperty + `
				},
				"required": ["attempt", "world", "kind", "outcome", "payload", "capability_token"]
			}`),
			handler: handleEvidenceSubmit,
		},
	}
}

type intentCreateArgs struct {
	Repo       string             `json:"repo"`
	Title      string             `json:"title"`
	Goal       string             `json:"goal"`
	Acceptance []specv1.Criterion `json:"acceptance"`
	Scope      specv1.Scope       `json:"scope"`
}

func handleIntentCreate(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
	var args intentCreateArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, &rpcError{code: ErrCodeInvalidParams, message: "intent_create: " + err.Error()}
	}
	if err := s.requireCapabilityToken(raw, ""); err != nil {
		return nil, err
	}

	intent, err := s.Graph.CreateIntent(ctx, graph.CreateIntentParams{
		Repo:       args.Repo,
		Title:      args.Title,
		Goal:       args.Goal,
		Acceptance: args.Acceptance,
		Scope:      args.Scope,
		CreatedBy:  s.Principal,
	}, s.Key)
	if err != nil {
		return nil, fmt.Errorf("intent_create: %w", err)
	}
	return intent, nil
}

type intentCheckPathArgs struct {
	Intent string `json:"intent"`
	Path   string `json:"path"`
}

func handleIntentCheckPath(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
	var args intentCheckPathArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, &rpcError{code: ErrCodeInvalidParams, message: "intent_check_path: " + err.Error()}
	}
	if err := s.requireCapabilityToken(raw, args.Path); err != nil {
		return nil, err
	}

	intent, err := s.Graph.GetIntent(ctx, args.Intent)
	if err != nil {
		return nil, fmt.Errorf("intent_check_path: %w", err)
	}
	allowed, err := scope.New(intent.Scope).IsPathAllowed(args.Path)
	if err != nil {
		return nil, fmt.Errorf("intent_check_path: %w", err)
	}
	return map[string]any{
		"path":    args.Path,
		"allowed": allowed,
		"mode":    intent.Scope.Mode,
	}, nil
}

type evidenceSubmitArgs struct {
	Attempt   string                 `json:"attempt"`
	Producer  string                 `json:"producer,omitempty"`
	World     string                 `json:"world"`
	Kind      specv1.EvidenceKind    `json:"kind"`
	Outcome   specv1.EvidenceOutcome `json:"outcome"`
	Payload   json.RawMessage        `json:"payload"`
	Footprint []string               `json:"footprint,omitempty"`
}

func handleEvidenceSubmit(ctx context.Context, s *Server, raw json.RawMessage) (any, error) {
	var args evidenceSubmitArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, &rpcError{code: ErrCodeInvalidParams, message: "evidence_submit: " + err.Error()}
	}
	world, err := specv1.ParseWorldHash(args.World)
	if err != nil {
		return nil, &rpcError{code: ErrCodeInvalidParams, message: "evidence_submit: " + err.Error()}
	}

	// Evidence with no footprint touches no specific path, so only the
	// token itself is verified; evidence with one authorizes each path it
	// claims to have touched.
	if len(args.Footprint) == 0 {
		if err := s.requireCapabilityToken(raw, ""); err != nil {
			return nil, err
		}
	}
	for _, path := range args.Footprint {
		if err := s.requireCapabilityToken(raw, path); err != nil {
			return nil, err
		}
	}

	// Default the producer to the daemon's own principal (e.g. an agent
	// submitting its own test-run results); an explicit producer lets a
	// human-review or third-party-runner record name someone else, though
	// nothing here verifies the daemon is actually entitled to sign on that
	// principal's behalf.
	producer := args.Producer
	if producer == "" {
		producer = s.Principal
	}

	evidence, err := s.Graph.SubmitEvidence(ctx, graph.SubmitEvidenceParams{
		Attempt:   args.Attempt,
		Producer:  producer,
		World:     world,
		Kind:      args.Kind,
		Outcome:   args.Outcome,
		Payload:   args.Payload,
		Footprint: args.Footprint,
	}, s.Key)
	if err != nil {
		return nil, fmt.Errorf("evidence_submit: %w", err)
	}
	return evidence, nil
}
