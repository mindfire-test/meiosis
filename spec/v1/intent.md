# Intent (v1)

An **Intent** is the primary unit of desired change within Meiosis. Instead of reviewing diffs in isolation, Meiosis tracks the explicitly declared goal, scope, and acceptance criteria *before* or *alongside* the code changes.

## Purpose & Lifecycle
- **Immutability:** An Intent is immutable once created and signed. Any modifications to the goal, scope, or criteria are made by creating a new Intent with a `SupersededBy` pointer on the predecessor (FR-3.2).
- **Status:** The lifecycle is tracked via status transitions: `open` -> `attempting` -> `merged` (or `abandoned` / `stale`).

## Object Definition

### Core Fields
- `id` (String, required): IntentID in the format `"int_{base32(blake3(canonical_json))}"`.
- `repo` (String, required): The repository identifier this intent applies to.
- `title` (String, required): Human-readable title of the intent.
- `goal` (String, required): Detailed description of the goal to be achieved.
- `acceptance` (Array of `Criterion`, required): The conditions that must be met for this intent to be merged.
- `non_goals` (Array of String, optional): Explicit non-goals to bound the work.
- `scope` (`Scope`, required): Declared paths the attempt is allowed to touch.
- `context_pins` (Array of `ContextPin`, optional): Pinned context documents that the author consumed.
- `parent` (String, optional): IntentID of the parent intent (if this is a sub-task).
- `depends_on` (Array of String, optional): IntentIDs that must be merged before this intent can be merged.
- `created_by` (String, required): PrincipalID of the creator (human or agent).
- `created_at` (Timestamp, required): ISO8601 timestamp of creation.
- `status` (String, required): Current status. Allowed values: `"open"`, `"attempting"`, `"merged"`, `"abandoned"`, `"stale"`.
- `signature` (Sig, required): Ed25519 signature of the canonical object bytes.

### Sub-objects

#### Criterion
Criteria can be textual or machine-checkable.
- `text` (String, required): Human-readable description.
- `check` (Object, optional): Machine-checkable definition (e.g., `{"kind":"test","selector":"./pkg/auth/..."}`).

#### Scope
Scope enforces what files an attempt is allowed to modify (FR-3.4).
- `allow` (Array of String, required): Glob patterns allowed to be modified.
- `deny` (Array of String, optional): Glob patterns strictly denied.
- `mode` (String, required): `"enforce"` (reject the write if it violates scope) or `"warn"` (allow the commit but flag as a violation).

#### ContextPin
Pins the exact version of documentation/issues the author used to write the code.
- `ref` (String, required): Path, URL, or issue ID.
- `digest` (String, required): BLAKE3 digest of the content at the time of consumption.
- `kind` (String, required): Allowed values: `"spec"`, `"adr"`, `"issue"`, `"design"`, `"convention"`.

## Validation Rules & Relationships
1. **DAG Cycles:** Intents can form a Directed Acyclic Graph via the `depends_on` array. The system MUST reject any cycles.
2. **Scope Enforcement:** Depending on `scope.mode`, attempts that modify files outside the `allow` globs must be either hard-rejected at push time, or recorded with a scope violation.
3. **Context Freshness (FR-3.6):** If a document referenced in `context_pins` changes (its content digest no longer matches `digest`), all attempts and merged intents depending on this Intent transition to `stale`.
