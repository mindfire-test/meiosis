# Attempt (v1)

An **Attempt** represents one principal's specific implementation of an `Intent`. It is materialized as a specific state of the repository, represented by a `WorldHash`.

## Purpose & Lifecycle
- **Competitive Attempts (FR-6.6):** Meiosis supports multiple concurrent attempts resolving the same Intent. They are ranked by evidence under policy. When one merges, the others are archived with their evidence intact.
- **Immutability:** Attempts are append-only. When a developer pushes a new commit to an attempt, it technically generates a new `Attempt` object (or updates a mutable state table referencing immutable attempts).

## Object Definition

### Core Fields
- `id` (String, required): AttemptID in the format `"att_{base32(blake3(canonical_json))}"`.
- `intent` (String, required): The `IntentID` this attempt is trying to resolve.
- `author` (String, required): PrincipalID of the author (human or agent).
- `world` (String, required): The 32-byte BLAKE3 `WorldHash` of the implementation's tree state.
- `base_world` (String, required): The `WorldHash` of the base tree this attempt was branched from.
- `git_commit` (String, optional): The Git commit hash corresponding to this world state (for Git interop).
- `scope_violations` (Array of Object, optional): If the Intent's scope mode was `"warn"`, any modifications outside the allowed scope are logged here.
- `status` (String, required): Current status. Allowed values include `"open"`, `"merged"`, `"abandoned"`.
- `created_at` (Timestamp, required): ISO8601 timestamp.
- `signature` (Sig, required): Ed25519 signature of the canonical object bytes by the `author`.

## Validation Rules & Relationships
1. **Relationship to Intent:** Every Attempt MUST map to exactly one valid `Intent`.
2. **World Binding:** The `world` field is the definitive identifier of the code's state. Evidence is bound directly to this hash.
3. **Scope Validation:** Upon creation, the server MUST calculate the diff between `base_world` and `world`. It then evaluates this diff against the parent `Intent`'s `Scope`. If the diff violates the scope and the mode is `"enforce"`, the Attempt is rejected. If `"warn"`, the violations are appended to `scope_violations`.
