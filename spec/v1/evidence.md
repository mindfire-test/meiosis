# Evidence (v1)

**Evidence** is a typed verification result (e.g., a test run, type check, or human review). The central design decision of Meiosis is that Evidence is **strictly bound to a specific WorldHash**, rather than a branch name.

## Purpose & Lifecycle
- **Automatic Expiration:** When the code changes (resulting in a new WorldHash), the proof goes stale automatically unless re-verified or incrementally mapped.
- **Immutability (FR-4.12):** Evidence records are immutable. Corrections or re-runs are submitted as new records superseding the old ones.

## Object Definition

### Core Fields
- `id` (String, required): Unique identifier for the evidence record.
- `attempt` (String, required): The `AttemptID` this evidence is associated with.
- `world` (String, required): The `WorldHash` of the exact tree state that was verified. Evidence *not* bound to a world hash is rejected (FR-4.1).
- `kind` (String, required): The type of evidence. See "Allowed Kinds" below.
- `producer` (String, required): PrincipalID of the entity that ran the verification (e.g., CI runner, human reviewer).
- `outcome` (String, required): The result. Allowed values: `"pass"`, `"fail"`, `"inconclusive"`.
- `footprint` (Array of String, optional): The explicit set of paths or symbols this evidence depends on.
- `payload` (JSON, required): Kind-specific data (e.g., test coverage percentages, linter output).
- `expires_at` (Timestamp, optional): A TTL after which the evidence expires regardless of footprint (FR-4.11).
- `created_at` (Timestamp, required): ISO8601 timestamp.
- `signature` (Sig, required): Ed25519 signature by the `producer`.

### Allowed Kinds (FR-4.2)
Built-in values for `kind` include:
- `"test-run"`: Unit/Integration tests.
- `"coverage"`: Code coverage reports.
- `"type-check"`: Compiler type checking.
- `"static-analysis"`: Linting (e.g., `golangci-lint`).
- `"benchmark"`: Performance benchmarks.
- `"mutation"`: Mutation testing results.
- `"security-scan"`: SAST or secret scanning.
- `"replay-match"`: A zygote bundle reproduced exactly (FR-4.9).
- `"human-review"`: A manual human code review.

## Validation Rules & Relationships
1. **Independence (FR-4.4):** A core Meiosis property is that an agent cannot grade its own homework. Independence is dynamically computed as: `independent = (Evidence.producer != Attempt.author)`. Policy can mandate independent evidence.
2. **Footprint Invalidation (FR-4.7):** If a footprint is provided, Meiosis tracks it. When any file or symbol in the `footprint` changes in subsequent worlds, dependent evidence becomes `stale` automatically without requiring a full re-run.
3. **World Exclusivity:** Evidence is useless if the `world` hash does not match the Attempt's target or base worlds (or a direct ancestor).
