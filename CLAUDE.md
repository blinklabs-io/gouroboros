# Gouroboros — Claude Code Guide

Companion to `AGENTS.md`; read that first. This file: Claude-specific facts and workflow rules.

## Conformance snapshot (2026-04-09)

| Metric | Value |
|--------|-------|
| Test functions | 2531 (100% passing) |
| Ledger rule vectors | 314/314 (via Amaru) |
| VRF | 29 vectors + 15 unit tests |
| KES | 14 (input-output-hk/kes) |
| Consensus | 212 |
| Byron blocks | 15 |
| Fuzz targets | 75 (nightly CI) |

Update this table when counts change.

## Tool preferences

- Grep tool, not `grep`/`rg` via Bash.
- Glob tool, not `find` via Bash.
- Read with `offset`/`limit` for known regions; never speculative reads > 500 lines.
- Explore subagent when a question needs > 3 Grep/Read rounds.
- Parallel tool calls for independent reads.

## Test commands

These are commands for a human to run in a local terminal; the `grep -cE` lines summarize conformance output and do not override the "Grep tool, not `grep`/`rg` via Bash" rule above for Claude's own searches.

```bash
make test
make lint
make format

go test -v ./ledger/...
go test -v ./protocol/...

# Conformance with capture for grep-back
go test -v ./internal/test/conformance/... 2>&1 | tee /tmp/conformance.txt
grep -cE "^    --- PASS: TestRulesConformanceVectors/" /tmp/conformance.txt
grep -cE "^    --- FAIL: TestRulesConformanceVectors/" /tmp/conformance.txt
```

## Rules Claude gets wrong without being told

1. Era delegation is not universal. Conway `UtxoValidateWithdrawals` has its own body with an intentional spec deviation (`NOTE:`). Grep the function before describing behavior.
2. `DecodeStoreCbor` requires a custom `UnmarshalCBOR` that calls `SetCbor(cborData)`. Missing → `Cbor()` returns nil → hashing breaks.
3. Hash from preserved `.Cbor()` bytes. Never re-encode and hash.
4. CBOR `EncMode`/`DecMode` are globally cached via `sync.Once` in `cbor/{encode,decode}.go`. Don't construct per call.
5. Pure Go, no CGO. Crypto from `golang.org/x/crypto`.
6. GPG signing required (exception: sibling `skunkworks/` repo).
7. `NOTE:` comments mark load-bearing intentional decisions. Don't "clean them up".
8. `NewTransactionBuilder()` returns `*MockTransaction`; use the concrete type for `WithWithdrawals`, `WithCollateral`, `WithReferenceInputs`, etc. The `TransactionBuilder` interface doesn't expose them.
9. All mock fixtures live in `github.com/blinklabs-io/ouroboros-mock`. Never inline local mocks. Missing fixture → add it upstream, bump the dep.

## Performance

CBOR decode dominates pipeline time (wrapper < 1%). Target CBOR, validation parallelism, or lazy parsing. Detail: `~/.claude/projects/-home-wolf31o2-Repos-blink-gouroboros/memory/MEMORY.md`.

## Security

- Constant-time crypto comparisons.
- CBOR depth limit: `MaxNestedLevels: 256`.
- Transaction size limit enforced at decode.
- Last external audit: December 2025, no critical findings.

## Related

`AGENTS.md`, `README.md`, `internal/test/conformance/README.md`.
