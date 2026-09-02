# Meiosis Format Specification (v1)

This document defines the language-agnostic `v1` format specification for Meiosis core objects. 

As defined in the SRS (§4.3), every object is serialized as JSON and **must** adhere to the [JSON Canonicalization Scheme (RFC 8785)](https://tools.ietf.org/html/rfc8785) for hashing and signing. Two compatible implementations must produce byte-identical canonical forms for the same logical object.

## 1. Core Types Overview

- `PrincipalID`: String format `"{type}:{name}"` (e.g. `"agent:planner-7"` or `"human:lakin"`).
- `IntentID`: String format `"int_{base32(blake3(canonical_json))}"`.
- `AttemptID`: String format `"att_{base32(blake3(canonical_json))}"`.
- `WorldHash`: 32-byte BLAKE3 root hash of a repository tree (represented as a hex string in JSON).
- `Sig`: Cryptographic signature (base64 encoded Ed25519 signature over the canonical JSON bytes).

## 2. Core Objects

The detailed definitions for each core object are broken down into the following specifications:

- [Intent](intent.md): The unit of desired change.
- [Attempt](attempt.md): One principal's implementation of an intent.
- [Evidence](evidence.md): A typed verification result strictly bound to a specific WorldHash.
- [Attestation](attestation.md): In-toto-compatible signed provenance of an attempt.
- [Verdict](verdict.md): A recorded merge decision and everything that produced it.

## 3. Relationships and Validation Rules

1. **Immutability:** Intents, Attempts, Evidence, Attestations, and Verdicts are strictly immutable once signed. Any modification is achieved by creating a new object that supersedes the old one.
2. **Signature Validation:** Every mutation carries a signature by the relevant principal. The system must reject unverifiable signatures. Signatures must evaluate over the RFC 8785 canonical bytes of the object.
3. **Evidence Independence:** Evidence is computationally "independent" if and only if `Evidence.producer != Attempt.author`.
4. **Evidence Freshness:** Evidence is tightly bound to `Evidence.world`. If the underlying world hash changes, or if a path defined in `Evidence.footprint` changes, the evidence automatically transitions to `stale`.
5. **Context Pin Invalidation:** When the BLAKE3 digest of a document pinned in `Intent.context_pins` changes, all attempts and merged intents depending on it automatically transition to `stale`.
6. **DAG Cycles:** Intents form a DAG via `depends_on`. Cyclical dependencies must be rejected by the system.
