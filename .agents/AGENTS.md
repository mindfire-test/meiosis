# Meiosis MCP tools

This repository is governed by meiosis — source control where evidence
expires. The daemon (meiosisd) exposes its tools over MCP; any agent in this
workspace may call them.

## MCP server

- server: meiosisd (registered in the IDE global MCP config)
- tools: intent_create, intent_check_path, evidence_submit

## Mandatory rule: every tool call must carry a capability token

The daemon runs with capability enforcement ON. A call whose arguments object
lacks a `capability_token` field is rejected. Read the token from
~/Projects/meiosis/.agents/meiosis-capability.json (relative to the repo root) and embed it as a JSON object, not a string.

For intent_check_path and evidence_submit the token must also cover the exact
path/paths you pass. This token's scope — allows: ** ; denies: (none) — is enforced there and at any
real write path. If a call is rejected for a path, do not retry: it is out of
scope by design. Ask the human to issue a wider token with:
    mei intent authorize --issuer human:<you> --allow '<glob>' ...

## Example

Call intent_create with:

{
  "repo": ".",
  "title": "Refactor UI component",
  "goal": "Update the Button component safely",
  "acceptance": [{"text": "Component tests pass"}],
  "scope": {"allow": ["src/components/**"], "deny": ["pkg/auth/**"], "mode": "enforce"},
  "`capability_token`": <object from /home/subhranshus/Projects/meiosis/.agents/meiosis-capability.json>
}
