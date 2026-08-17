---
name: cardano-protocol-reviewer
description: Review Blink Labs Cardano ledger, CBOR, Plutus, and Ouroboros protocol changes for semantic, wire-format, compatibility, and conformance risks. Use when reviewing or changing gouroboros, ouroboros-mock, plutigo, Dingo protocol code, ledger-era rules, protocol state machines, serialization, or consensus fixtures.
---

# Cardano Protocol Reviewer

Use this skill for protocol and ledger changes where a passing unit test is not
enough evidence of compatibility. Read the target repository's local guidance
first, then use the [protocol checklist](references/protocol-checklist.md) to
choose the relevant invariants and validation.

## Workflow

1. Establish the repository and module boundary. For downstream behavior,
   inspect the owning `gouroboros` or `plutigo` implementation before reviewing
   the adapter.
2. Classify the change as ledger-era validation, CBOR/wire encoding, an
   Ouroboros mini-protocol, consensus, Plutus evaluation, governance, or test
   fixture/conformance data.
3. Trace the public interface, callers, state transitions, and error paths.
   Do not assume later-era rules delegate to earlier eras; inspect the actual
   function body.
4. Preserve original CBOR bytes and protocol ordering. For values embedding
   `DecodeStoreCbor`, call `SetCbor(nil)` before marshaling a mutation that must
   be encoded.
5. Reuse `ouroboros-mock` fixtures and vectors. Add missing shared fixtures
   there rather than duplicating them in Dingo, Adder, Shai, or another app.
6. Run deterministic targeted tests first, then race, conformance, fuzz,
   benchmark, or devnet checks according to the changed contract.
7. Report behavioral, wire/API, conformance, architecture, and documentation
   findings before style suggestions. Identify skipped live validation.

Never trade protocol correctness for a convenient local replacement or a
re-encoded fixture. Follow the workspace's bot-first, human-required review
sequence when this skill is used for a pull request.
