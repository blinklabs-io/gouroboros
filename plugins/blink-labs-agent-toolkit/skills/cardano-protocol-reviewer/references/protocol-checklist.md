# Protocol review checklist

| Contract | Inspect | Evidence |
| --- | --- | --- |
| Ledger rule | `ledger/<era>/rules.go`, errors, registration, callers | Typed errors, era-specific tests, conformance vectors |
| CBOR | `cbor/`, `MarshalCBOR`, `UnmarshalCBOR`, `DecodeStoreCbor` | Raw-byte round trips, hash stability, map/order tests |
| Mini-protocol | `protocol/<name>/client.go`, `server.go`, `messages.go`, state | Handshake/state transition tests, malformed-message cases, protocol limits |
| Consensus | chain selection, Praos/VRF/KES/OpCert, mock scenarios | Deterministic vectors, fork/rollback cases, devnet or conformance |
| Plutus | `plutigo/cek`, builtins, cost models, syntax/FLAT parser | Conformance, fuzz, budget/error behavior, benchmarks where relevant |
| Governance | Conway rules, certificates, voting/ratification transitions | Epoch-boundary tests, governance vectors, exact spec references |
| Shared fixtures | `ouroboros-mock/fixtures`, `ledger`, `conformance`, `consensus` | Provenance, reproducibility, downstream reuse |

Guardrails:

- Read the function body before claiming era delegation or a missing check.
- Hash preserved CBOR bytes, not a newly encoded approximation.
- Treat `NOTE:` comments and reconciliation tables as load-bearing design
  decisions until verified against the specification.
- Never add a local mock when the shared fixture library can own the case.
