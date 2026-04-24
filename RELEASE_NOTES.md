---
title: Release notes
---

# Release notes

## v0.165.3 - shutdown idempotence and sanchonet peer fix

- **Date:** 2026-04-24
- **Version:** 0.165.3

### Summary

This release keeps connection shutdown cleanup to a single pass so duplicate close paths no longer panic during setup failures or externally triggered closes, restores the default Sanchonet bootstrap peer so default network configuration can connect successfully again, and includes routine contributor guidance and release note maintenance.

### Bug Fixes

* Fixed connection shutdown cleanup to run exactly once so duplicate shutdown paths do not panic with `close of closed channel` errors during setup failures or externally triggered closes.
* Restored the default Sanchonet bootstrap peer to a working host and port so default network configuration can connect successfully again.

### Additional Changes

* Updated contributor guidance by refreshing `AGENTS.md` and adding `CLAUDE.md`.
* Added the prior `v0.165.2` release entry to `RELEASE_NOTES.md`.

## v0.165.2 - plutigo dependency and copyright update

- **Date:** 2026-04-19
- **Version:** 0.165.2

### Summary

This release updates `github.com/blinklabs-io/plutigo` to `v0.1.8`, refreshes repository copyright headers for 2026, and refreshes `RELEASE_NOTES.md` to include the prior `v0.165.1` release entry.

### Additional Changes

* Updated `github.com/blinklabs-io/plutigo` to `v0.1.8`.
* Refreshed repository copyright headers to `2026`.
* Added the prior `v0.165.1` release entry to `RELEASE_NOTES.md`.

## v0.165.1 - auxiliary data decode compatibility fix

- **Date:** 2026-04-18
- **Version:** 0.165.1

### Summary

This release keeps auxiliary data decoding compatible when future extension keys appear, so metadata at key `0` still loads correctly and raw auxiliary data still round trips as expected. It also refreshes `RELEASE_NOTES.md` for this version.

### Bug Fixes

* Improved auxiliary data decoding to ignore unknown integer keyed `CBOR` map entries, continue extracting metadata from key `0`, and prove through regression coverage that metadata and raw auxiliary data still round trip when a future era extension key such as key `6` is present.

## v0.165.0 - fee calculation and precision updates

- **Date:** 2026-04-17
- **Version:** 0.165.0

### Summary

This release improves consensus threshold precision for higher active slot coefficients, hardens minimum fee calculation with overflow detection and propagated errors, updates `github.com/blinklabs-io/plutigo` to `v0.1.7`, and updates `RELEASE_NOTES.md` to add the prior release entry.

### Bug Fixes

* Improved consensus threshold precision for higher active slot coefficients by using higher precision calculations with expanded validation so threshold results converge accurately.
* Hardened minimum fee calculation by detecting multiplication and addition overflow, returning descriptive errors, and propagating those failures through fee validation paths with test coverage.

### Additional Changes

* Updated `github.com/blinklabs-io/plutigo` from `v0.1.6` to `v0.1.7`.
* Refreshed `RELEASE_NOTES.md` to include the `v0.164.0` entry.

## v0.164.0 - messagesubmission versioning and handshake query

- **Date:** 2026-04-13
- **Version:** 0.164.0

### Summary

This release includes improved `MessageSubmission` protocol version selection, handshake query support, and dependency and CI updates.

### New Features

- Added versioned `MessageSubmission` state machines (`V1` and `V2`) with automatic selection based on the negotiated protocol version.
- Added a connection-level handshake query mode that terminates after the query reply and exposes the returned version map to callers.

### Security

- Updated `golang.org/x/crypto` to `v0.50.0` and `golang.org/x/sys` to `v0.43.0`.

### Additional Changes

- Updated `github.com/fxamacker/cbor/v2` to `v2.9.1`.
- Updated `github.com/blinklabs-io/plutigo` to `v0.1.6` and `github.com/ethereum/go-ethereum` to `v1.17.2`.
- Updated GitHub Actions workflows to use `actions/github-script@v9.0.0` and `actions/upload-artifact@v7.0.1`.
- Added a `nilaway` CI workflow and a `make nilaway` target to run nil safety analysis.
- Updated `RELEASE_NOTES.md` to include the `v0.163.5` entry.

## v0.163.5 - byron offsets and validation hardening

- **Date:** 2026-04-02
- **Version:** 0.163.5

### Summary

This release includes Byron block offset extraction, Byron validation hardening, and dependency and documentation updates.

### New Features

- Added Byron block detection and transaction and output offset extraction.

### Bug Fixes

- Fixed block ingestion to reject invalid Byron block body lengths and standardize Byron transaction validation errors.

### Additional Changes

- Updated `README.md` to add a DeepWiki badge link.
- Updated `plutigo` to `v0.1.0`.
- Updated `RELEASE_NOTES.md` to include the `v0.163.4` entry.

## v0.163.4 - blockfetch limits and conway validation fix

- **Date:** 2026-03-30
- **Version:** 0.163.4

### Summary

This release includes separate `BlockFetch` pending message limits for `Idle` and `Busy` states, a Conway-era transaction validation fix, and CI workflow updates.

### New Features

- Added separate `BlockFetch` pending message limits for `Idle` and `Busy` states.

### Bug Fixes

- Fixed Conway-era transaction validation to reject invalid data.

### Additional Changes

- Updated configuration and tests for the `BlockFetch` pending message limit split.
- Added a regression test to cover the Conway-era transaction validation fix.
- Updated GitHub Actions workflows to use `actions/setup-go@v6.4.0` (from `v6.3.0`).
- Updated `RELEASE_NOTES.md` to document dependency, toolchain, and documentation changes.

## v0.163.3 - conway rule 7 datum witness validation

- **Date:** 2026-03-24
- **Version:** 0.163.3

### Summary

This release includes an update to Conway transaction validation that permits `ScriptDataHash` when only datum witnesses are present and no redeemers are included.

### Bug Fixes

- Fixed Conway transaction validation to accept transactions with `ScriptDataHash` when the witness set includes datums but no redeemers.

### Additional Changes

- Added a regression test that uses a real `CBOR`-encoded transaction to confirm the Rule 7 acceptance criteria.

## v0.163.2 - dependency and documentation updates

- **Date:** 2026-03-23
- **Version:** 0.163.2

### Summary

This release includes Go toolchain and dependency updates and release notes maintenance.

### Additional Changes

- Updated `plutigo` and `gnark-crypto` module versions and checksums.
- Updated the Go toolchain and selected dependencies to patched versions.
- Added the `v0.163.1` entry to `RELEASE_NOTES.md`.

## v0.163.1 - leios notification loop lifecycle fixes

- **Date:** 2026-03-20
- **Version:** 0.163.1

### Summary

This release includes Leios notification loop lifecycle fixes, Conway genesis governance `JSON` decoding support, and documentation updates.

### New Features

- Added support for decoding Conway genesis governance from `JSON` so developers can load and validate genesis governance configuration in tooling and integrations.

### Bug Fixes

- Improved the Leios notification loop lifecycle so notifications behave consistently during synchronization and shutdown scenarios.

### Additional Changes

- Updated the release notes and documentation to reflect `JSON` decoding behavior and related documentation changes.

## v0.163.0 - Conway genesis governance decoding

- **Date:** 2026-03-19
- **Version:** 0.163.0

### Summary

This release includes Conway genesis JSON decoding support for pre-configured governance representatives and governance anchor data.

### New Features

- Added support for initial `DReps` and `GovAnchor` fields when decoding Conway genesis JSON so governance configuration loads at startup.

## v0.162.0 - drep and credential json decoding

- **Date:** 2026-03-18
- **Version:** 0.162.0

### Summary

This release includes `JSON` decoding support for `Drep` and `Credential` values and documentation updates.

### New Features

- Added `JSON` unmarshalling for `Drep` and `Credential`, with tests that validate `JSON`-to-structure decoding across representative inputs.

### Additional Changes

- Added the `v0.161.1` entry to `RELEASE_NOTES.md`.

## v0.161.1 - peer address cbor encoding and cip-0008 test coverage

- **Date:** 2026-03-18
- **Version:** 0.161.1

### Summary

This release includes explicit `PeerAddress` `CBOR` encoding for peer sharing, CIP-0008 `COSE`/`ed25519` message signing test coverage, and documentation updates.

### New Features

- Added explicit `CBOR` encoding for `PeerAddress`, with tests for `SharePeers` and IPv6 round-trip validation.
- Added CIP-0008 `COSE`/`ed25519` message signing and verification test coverage across expected inputs and edge cases.

### Additional Changes

- Updated documentation to keep release notes in a single location.

## v0.161.0 - typed query results and transaction cbor preservation

- **Date:** 2026-03-17
- **Version:** 0.161.0

### Summary

This release includes typed reward and delegation query results, preserves Alonzo/Babbage/Conway transaction `CBOR` round-trips, and updates the Go toolchain baseline, CI automation, and dependencies.

### New Features

- Added structured result types for rewards and delegations queries, including `RewardInfoPoolsResult`, `RewardProvenanceResult`, and `ShelleyFilteredDelegationsAndRewardAccounts`, with `CBOR` encoding and decoding support and client integration.

### Breaking Changes

- Updated the minimum supported Go version to `Go 1.25` and updated CI workflows to run tooling, tests, benchmarks, and fuzzing on `Go 1.26`, requiring local development and CI toolchain upgrades.

### Bug Fixes

- Fixed Alonzo, Babbage, and Conway transaction round-tripping to preserve byte-for-byte transaction `CBOR` encoding and serialized size.

### Performance

- Improved Alonzo, Babbage, and Conway transaction processing by reusing raw component bytes during `CBOR` reconstruction to avoid unnecessary re-encoding.

### Security

- Updated `golang.org/x/crypto` to `v0.49.0` and `golang.org/x/sys` to `v0.42.0`.

### Additional Changes

- Updated CI workflows for new Go versions and updated `RELEASE_NOTES.md` to include the `v0.160.3` entry.

## v0.160.3 - cbor and transaction performance improvements

- **Date:** 2026-03-16
- **Version:** 0.160.3

### Summary

This release includes CBOR and transaction processing performance improvements, plus CI and dependency maintenance.

### Performance

- Improved CBOR handling by adding slice-backed references, stream-based and in-place decoding helpers, and tighter decode paths to reduce memory usage and avoid unnecessary copying.
- Improved transaction processing and RPC conversion by optimizing multi-asset value checks, witness and script handling, tx-to-RPC conversion, and `EncodeLangViews` encoding and unsupported-version errors.

### Additional Changes

- Added the `v0.160.2` entry to `RELEASE_NOTES.md`.
- Updated the benchmark workflow to use `actions/download-artifact@v8.0.1` (from `v8.0.0`).
- Updated dependencies, including `plutigo` and `go-ethereum`, to keep the build aligned with upstream libraries and expected checksums.

## v0.160.2 - muxer synchronization and encoding fixes

- **Date:** 2026-03-13
- **Version:** 0.160.2

### Summary

This release includes muxer protocol instance synchronization, `EncodeLangViews` validation hardening, and documentation updates.

### New Features

- Added synchronized accessors for protocol instances in the muxer and connection paths to support thread-safe protocol selection and lifecycle management.

### Bug Fixes

- Fixed `EncodeLangViews` to reject invalid inputs earlier and behave consistently across edge cases.

### Performance

- Improved allocation efficiency in `EncodeLangViews` by reducing allocation churn during encode and validate operations.

### Additional Changes

- Updated `RELEASE_NOTES.md` to include the `v0.160.1` entry with categorized details.

## v0.160.1 - validation and security updates

- **Date:** 2026-03-10
- **Version:** 0.160.1

### Summary

This release includes block and VRF validation updates, ChainSync decoding corrections, and security and CI refinements.

### Breaking Changes

- Enforced Byron non-genesis header linkage by requiring `PrevHeaderHash` where applicable and updating consensus `prev-hash` validation tests.
- Updated VRF input helpers to return `error` instead of panicking, and updated callers, tests, and benchmarks to propagate failures.

### Bug Fixes

- Fixed an execution-unit overflow edge case and added regression tests for Alonzo, Babbage, and Conway UTxO `ExUnits` validation.
- Fixed ChainSync unmarshalling to reject CBOR tag payloads that are not `[]byte`, with tests covering invalid types.

### Performance

- Improved allocation efficiency by preallocating `KeyValuePairs` slices based on input sizes in two internal helpers.

### Security

- Improved `SumXKesSig.Verify` to use constant-time comparison for public key equality.
- Updated the network magic mismatch refusal message to avoid exposing implementation details, and added a regression test.

### Additional Changes

- Updated the benchmark CI workflow to use `actions/download-artifact@v8.0.0` (from `v4.3.0`).
- Updated `RELEASE_NOTES.md` to include the `v0.160.0` entry.

## v0.160.0 - protocol flow control and validation fixes

- **Date:** 2026-03-03
- **Version:** 0.160.0

### Summary

This release includes network protocol flow control updates, consensus validation corrections, and CI and documentation refinements.

### New Features

- Added CI workflows to run benchmarking and profiling, including allocation-regression tests for core validation paths, comment-triggered benchmark runs with an authorization gate, and automated posting of benchmark results to pull requests.
- Added GitHub Actions workflows that trigger on issue closure to log issue metadata and update the project item `Closed Date` field.

### Breaking Changes

- Updated network protocol flow control to apply backpressure instead of disconnecting by increasing the max pending message bytes for NtN, removing byte limits from NtC `ChainSync`, and modifying the protocol read loop when pending bytes or queues fill.

### Bug Fixes

- Fixed JSON marshaling for `LazyValue` when underlying CBOR is missing or empty and added test coverage.
- Fixed nonce and header validation to follow era-specific rules by making VRF input and nonce handling era-specific, switching KES verification to use stored header-body CBOR, tightening validation logic, and expanding tests.
- Fixed rolling nonce derivation to hash `prevBlockNonce` concatenated with raw VRF output bytes and refreshed related tests and comments.
- Fixed connection shutdown to treat normal protocol completion as graceful so stop paths avoid spurious errors and avoid sending `Done` or `ClientDone` when protocols are already finished.
- Fixed transaction fee sizing and era-specific semantics by centralizing fee size calculation via `TxSizeForFee`, correcting datum-hash nil semantics, normalizing Conway redeemer handling, eagerly caching CBOR for Babbage transactions, and adding a Conway Plutus V3 reproduction test using `plutigo` v0.0.26.

### Additional Changes

- Updated `RELEASE_NOTES.md` by backfilling entries for versions `0.128.0` through `0.159.2`, including detailed notes for `0.140.0` through `0.159.1` and a dedicated entry for `0.159.2`.
- Updated release note language for clearer scanning while preserving technical meaning.
- Updated GitHub Actions dependencies to `actions/setup-go` v6.3.0 (from v6.2.0) and `actions/upload-artifact` v7.0.0.

## v0.159.2 - transaction validation updates

- **Date:** 2026-02-27
- **Version:** 0.159.2

### Summary

This release includes Conway Plutus transaction validation updates, strict CBOR metadata decoding, and developer documentation refinements.

### Breaking Changes

- Updated Conway Plutus transaction validation to require `ScriptDataHash` only when redeemers or witness datums are present (script references are treated as inert) and to return a typed input-resolution error when a UTxO lookup fails.
- Updated callers to omit `ScriptDataHash` unless a transaction includes Plutus redeemers or witness datums.

### Bug Fixes

- Fixed metadata decoding for generic CBOR maps to fail fast on any key or value decode error by using `*cbor.Value` map keys.

### Additional Changes

- Updated `AGENTS.md` to clarify delegation behavior, document `TransactionBuilder` and `MockTransaction` usage, and expand validation and review guidelines.

## v0.159.1 - transaction handling and CBOR conformance

- **Date:** 2026-02-26
- **Version:** 0.159.1

### Summary

This release includes updates to transaction preparation and CBOR serialization conformance.

### New Features

- Updated transaction preparation to export input sorting for redeemer mapping and apply consistent datum handling across validation paths.

### Bug Fixes

- Fixed certificate amount CBOR encoding and script context serialization in test vectors to match expected on-chain representation.

## v0.159.0 - asynchronous Leios notifications

- **Date:** 2026-02-25
- **Version:** 0.159.0

### Summary

This release adds asynchronous Leios notifications so applications can receive updates without blocking other work.

### New Features

- Added asynchronous Leios notifications with a callback-driven loop, configurable pipeline depth, and clean shutdown support.

### Breaking Changes

- Replaced the blocking request-next flow with a callback-based notification model.

## v0.158.4 - ledger validation and protocol pipelining

- **Date:** 2026-02-25
- **Version:** 0.158.4

### Summary

This release expands ledger validation safety checks and improves protocol pipelining behavior.

### New Features

- Added overflow-safe execution unit accumulation, a no-duplicate-inputs UTxO rule across all eras, and centralized withdrawal validation with an `IsValid` short-circuit.

### Bug Fixes

- Fixed an issue where protocol pipelining state transitions could be applied incorrectly, preventing stalls and out-of-order blocks during pipelined sync.

### Additional Changes

- Updated `AGENTS.md` to demonstrate creating mock ledgers using `ouroboros-mock` ledger builders.
- Updated `filippo.io/edwards25519` from v1.1.1 to v1.2.0.

## v0.158.3 - transaction JSON serialization

- **Date:** 2026-02-22
- **Version:** 0.158.3

### Summary

This release stabilizes JSON serialization for transactions and script-related types across all eras so output is deterministic, human-readable, and round-trippable.

### Bug Fixes

- Fixed redeemer, datum, address, hash, and governance type JSON encoding to produce deterministic output with proper text marshaling and CIP-129 validation.
- Fixed transaction JSON output for Byron through Conway with stable field ordering.

## v0.158.2 - per-connection protocol state

- **Date:** 2026-02-19
- **Version:** 0.158.2

### Summary

This release prevents shared protocol state mutation on servers so each connection gets its own isolated state.

### New Features

- Added configurable idle timeout for ChainSync per connection.

### Bug Fixes

- Fixed protocol servers to use per-connection state maps, preventing shared state mutations across connections.

### Additional Changes

- Updated `filippo.io/edwards25519` and `plutigo` dependencies.

## v0.158.1 - spec-aligned protocol timeouts

- **Date:** 2026-02-18
- **Version:** 0.158.1

### Summary

This release splits ChainSync and Handshake into separate Node-to-Node and Node-to-Client state maps so timeouts align with the Ouroboros network specification.

### Breaking Changes

- ChainSync and Handshake now use separate NtN/NtC state maps, defaulting to NtC unless explicitly configured for NtN.

### Bug Fixes

- Aligned ChainSync and TxSubmission timeouts with the Ouroboros network specification to prevent premature disconnects.

## v0.158.0 - protocol hardening and governance queries

- **Date:** 2026-02-17
- **Version:** 0.158.0

### Summary

This release hardens protocol and cryptographic handling, adds Conway governance queries for ratification state and committee members, and caps the protocol read buffer at 16MB to prevent unbounded memory growth.

### New Features

- Added ratification state and committee member state queries for Conway-era governance.

### Bug Fixes

- Tightened VRF verification to reject non-canonical scalars and zero sensitive buffers after use.
- Fixed muxer diffusion mode race condition and nil committee decoding in governance queries.
- Required issuer verification key for operational certificate validation.
- Fixed CBOR block encoding to emit empty arrays instead of null for nil list fields.
- Capped protocol read buffer at 16MB to prevent memory exhaustion from oversized messages.

### Additional Changes

- Updated `golang.org/x/crypto` and `plutigo` dependencies.

## v0.157.0 - security hardening

- **Date:** 2026-02-15
- **Version:** 0.157.0

### Summary

This release hardens the muxer against denial-of-service attacks, adds a strict CBOR decoder for untrusted inputs, and corrects the rolling nonce calculation to match the Ouroboros Praos specification.

### New Features

- Added DRep stake distribution query for Conway era.

### Bug Fixes

- Hardened the muxer against slowloris-style attacks by enforcing read deadlines and making error propagation non-blocking.
- Added a strict CBOR decoder with tighter limits for network-facing inputs.
- Corrected the rolling nonce calculation to use XOR per the Ouroboros Praos specification.
- Fixed Conway redeemer iteration for legacy-mode transactions.

## v0.156.0 - ChainSync state isolation and governance queries

- **Date:** 2026-02-14
- **Version:** 0.156.0

### Summary

This release isolates ChainSync client and server state so pipelining on one side cannot interfere with the other, and adds Conway governance queries for vote delegatees and proposals.

### New Features

- Added filtered vote delegatees and governance proposals queries for Conway era.

### Bug Fixes

- Isolated ChainSync client and server state to prevent pipelining interference.
- Fixed a muxer deadlock in protocol unregistration.

## v0.155.0 - CBOR improvements and governance state

- **Date:** 2026-02-12
- **Version:** 0.155.0

### Summary

This release adds DRep state and proposals queries, raises CBOR decode limits for mainnet-scale data, and refactors constructor handling for safer encoding.

### New Features

- Added DRep state and governance proposals queries for Conway era.

### Bug Fixes

- Raised CBOR decoder map limits to handle large mainnet stake distribution snapshots.
- Added bounds checking to prevent panics on empty or malformed CBOR input.
- Updated Leios protocol message shapes to align with the latest CIP-0164 specification.

### Additional Changes

- Refactored CBOR constructor handling for safer, byte-accurate encoding and decoding.
- Added architecture documentation, Go 1.26.x CI support, and memory profiling benchmarks.

## v0.154.0 - block processing pipeline

- **Date:** 2026-02-10
- **Version:** 0.154.0

### Summary

This release introduces a concurrent block processing pipeline that parallelizes decode and validate stages while applying blocks in slot order, and caches CBOR encoder/decoder modes for faster serialization.

### New Features

- Added a concurrent block processing pipeline with configurable workers, backpressure, prefetch, and safe rollback draining, wired into ChainSync and BlockFetch.

### Performance

- Cached CBOR encoder/decoder modes with thread-safe lazy initialization, reducing allocations in hot paths.

### Additional Changes

- Updated `plutigo` dependency.

## v0.153.1 - performance optimizations and CPRAOS

- **Date:** 2026-02-06
- **Version:** 0.153.1

### Summary

This release delivers broad performance optimizations across VRF, KES, consensus, and ledger paths, switches leader election to CPRAOS, and enforces VRF key uniqueness starting with protocol version 11.

### Bug Fixes

- Switched consensus to CPRAOS leader election to match the current Cardano protocol.
- Enforced VRF key uniqueness across pools starting with protocol version 11.
- Fixed genesis config to accept alternate key names used by different Cardano node versions.

### Performance

- Reduced allocations across VRF, KES, consensus, and ledger hot paths by switching to native scalar operations, stack-allocated buffers, and pre-allocated slices.

### Additional Changes

- Added comprehensive VRF and KES benchmarks.

## v0.153.0 - protocol version 11 and conformance testing

- **Date:** 2026-02-05
- **Version:** 0.153.0

### Summary

This release adds protocol version 11 (VanRossem) support, migrates conformance tests to the ouroboros-mock framework, and implements the member rewards query.

### New Features

- Added protocol version 11 support with VRF key uniqueness, CC voting restrictions, and Plutus V1/V2 reference input handling.
- Added member rewards query for per-stake/per-pool reward lookups.

### Bug Fixes

- Fixed CC voting restriction validation to properly reject non-Conway protocol parameters.

### Additional Changes

- Migrated conformance tests to the ouroboros-mock framework, removing ~3k lines of bespoke test code.
- Added unit tests for Leios protocols, CBOR streams, muxer lifecycle, and CIP-0019 addresses.

## v0.152.2 - UTxO overhead constant fix

- **Date:** 2026-02-02
- **Version:** 0.152.2

### Summary

This release corrects the minimum coin calculation to include the 160-byte UTxO overhead per CIP-0055.

### Bug Fixes

- Fixed minimum coin calculation to include the CIP-0055 160-byte UTxO overhead constant for Babbage and Conway.

## v0.152.1 - CBOR offset fixes

- **Date:** 2026-02-01
- **Version:** 0.152.1

### Summary

This release fixes CBOR offset calculations for indefinite-length arrays and adds per-protocol documentation.

### Bug Fixes

- Fixed CBOR offset calculations for indefinite-length arrays used in transaction bodies, witnesses, and outputs.

### Additional Changes

- Added per-protocol documentation for all Ouroboros mini-protocols with state machines, timeouts, and usage examples.

## v0.152.0 - CBOR byte offsets and Byron consensus

- **Date:** 2026-02-01
- **Version:** 0.152.0

### Summary

This release adds single-pass CBOR byte offset extraction so callers can locate individual transaction components within a block, implements initial Byron consensus validation, and fixes several Plutus script context issues.

### New Features

- Added single-pass CBOR byte offset extraction for per-transaction components within blocks.
- Added Byron OBFT consensus validation covering protocol magic, slot leader rotation, signature verification, and body hash checks.

### Bug Fixes

- Fixed Plutus script context encoding for V2 transactions and mint value handling.
- Fixed Plutus evaluation to return consumed budget even on script failure.
- Deduplicated datum entries in the Plutus script context.

### Additional Changes

- Standardized test mocks on `ouroboros-mock/ledger` across all tests.
- Added package-level documentation and developer guides.

## v0.151.2 - Plutus mint encoding fix

- **Date:** 2026-01-28
- **Version:** 0.151.2

### Summary

This release fixes Plutus data conversion for mint values so V1/V2 script contexts match the expected on-chain representation.

### Bug Fixes

- Fixed mint-to-Plutus-data conversion to match expected CBOR encoding and avoid wrapping in a zero-ADA value.

### Additional Changes

- Updated CI action dependencies.

## v0.151.1 - Plutus V2 cost model fix

- **Date:** 2026-01-25
- **Version:** 0.151.1

### Summary

This release fixes Plutus V2 cost model selection in Conway and adds a datum list type that preserves original CBOR encoding.

### Bug Fixes

- Fixed Conway to select the correct cost model for Plutus V2 scripts.
- Fixed validity interval logic for transaction TTL checks.

### New Features

- Added a datum list type that preserves original CBOR encoding for exact script data hash computation.

## v0.151.0 - transaction errors and fuzz testing

- **Date:** 2026-01-25
- **Version:** 0.151.0

### Summary

This release adds era-aware transaction error decoding for Babbage and Conway, fixes cost model merging, and introduces fuzz testing across the codebase.

### New Features

- Added era-aware transaction error decoding with full Babbage and Conway support.

### Bug Fixes

- Fixed cost model merging to properly combine incoming models with existing protocol parameters instead of overwriting them.

### Additional Changes

- Added fuzz targets across CBOR, ledger, KES, and VRF with nightly CI and seeded corpora.
- Updated `plutigo` dependency.

## v0.150.0 - governance queries and protocol hardening

- **Date:** 2026-01-20
- **Version:** 0.150.0

### Summary

This release adds a full suite of Conway-era governance state queries, provides cost models for Plutus script evaluation, and hardens protocol client lifecycles so they can be safely restarted.

### New Features

- Added Conway-era governance queries: constitution, gov state, DRep state/distribution, committee members, vote delegatees, and SPO distribution.
- Added cost model support for Plutus script evaluation under Conway.

### Bug Fixes

- Fixed genesis cost model decoding to support both list and map JSON formats.
- Made blockfetch and txsubmission clients idempotent and safely restartable.
- Fixed chainsync pipeline defaults to prevent sync stalls from zero-value configuration.
- Fixed a muxer data race that could cause shutdown panics.
- Added bounds checking to prevent panics on malformed address, CBOR rational, and list inputs.

### Additional Changes

- Updated `plutigo` and `golang.org/x/crypto` dependencies.

## v0.149.0 - VRF, KES, and consensus packages

- **Date:** 2026-01-16
- **Version:** 0.149.0

### Summary

This release introduces standalone pure-Go packages for VRF, KES, and consensus so these cryptographic primitives can be used independently of the ledger, and adds operational certificate verification.

### New Features

- Added pure-Go VRF, KES, and consensus packages with conformance tests against official test vectors.
- Added operational certificate verification and creation.

### Breaking Changes

- VRF and KES verification functions moved from `ledger` to dedicated `vrf` and `kes` packages.

## v0.148.0 - script data hash validation and conformance tests

- **Date:** 2026-01-13
- **Version:** 0.148.0

### Summary

This release adds script data hash validation to catch malformed reference scripts and invalid redeemers, implements stake snapshot and pool state queries, and enables ChainSync client reuse on the same connection.

### New Features

- Added script data hash validation with malformed reference script detection and redeemer bounds checking.
- Added stake snapshot and pool state queries with detailed per-pool data.
- Enabled ChainSync client reuse on the same connection without reconnecting.

### Bug Fixes

- Fixed block body hash calculation to include invalid transaction indices.

### Additional Changes

- Added conformance tests using Amaru test vectors.

## v0.147.0 - validation overhaul and Plutus script contexts

- **Date:** 2026-01-11
- **Version:** 0.147.0

### Summary

This release delivers a major validation overhaul reaching 302 of 314 conformance tests, adds PlutusV1 and V2 script context building, and switches monetary amounts to arbitrary precision to prevent overflow.

### New Features

- Added PlutusV1 and V2 script context building.
- Added governance state and CIP-0005 bech32 string representations.
- Added stake pool parameter queries.

### Breaking Changes

- Switched monetary amounts and multi-asset outputs to arbitrary-precision integers.

### Bug Fixes

- Overhauled validation across delegation, withdrawals, native scripts, script data hashes, and governance with proper error types.
- Fixed numerous Plutus script context, Conway proposal, multi-asset conservation, and protocol race condition issues.

### Additional Changes

- Added CIP compliance tests and removed legacy binaries.

## v0.146.0 - signature validation and CIP-0137

- **Date:** 2026-01-03
- **Version:** 0.146.0

### Summary

This release adds full transaction signature validation, implements the CIP-0137 distributed message queue protocol, and fixes Byron witness validation so bootstrap addresses are verified correctly.

### New Features

- Added transaction signature validation, CIP-0137 distributed message queue support, TX auxiliary data decoding across all eras, and handshake query reply handling.
- Added witness validation rules, metadata validation, and collateral/cost model checks.

### Bug Fixes

- Fixed Byron witness validation with correct address root computation.
- Fixed numerous metadata, script data hash, cost model, and CBOR handling issues across eras.
- Fixed chainsync pipeline deadlock and muxer shutdown race condition.

### Additional Changes

- Added CIP conformance tests and centralized validation error handling.
- Updated Leios implementation per CIP-0164.

## v0.145.0 - structured validation errors

- **Date:** 2025-12-13
- **Version:** 0.145.0

### Summary

This release introduces structured validation errors so callers can programmatically inspect failure reasons, and adds pool distribution queries with CIP-129 voter string representations.

### New Features

- Added typed validation errors with categories and contextual details like era, slot, and block number.
- Added pool distribution queries and CIP-129 bech32 voter identifiers.

### Bug Fixes

- Included era context in block verification errors for easier troubleshooting.

## v0.144.0 - stake pool validation

- **Date:** 2025-12-09
- **Version:** 0.144.0

### Summary

This release adds stake pool validation to block verification so blocks are checked against registered pool data and VRF keys.

### New Features

- Added stake pool validation to block verification across Shelley through Conway with opt-out support.
- Centralized header field extraction across eras.

### Bug Fixes

- Fixed metadata handling to use raw bytes.

## v0.143.0 - transaction validation and KES verification

- **Date:** 2025-12-05
- **Version:** 0.143.0

### Summary

This release wires transaction validation into block verification, extends KES verification to all post-Shelley eras, and adds public network roots for mainnet and testnet discovery.

### New Features

- Added transaction validation to block verification with era-specific UTxO rules and configurable skip options.
- Added block body hash validation with opt-out support.
- Added public network roots for mainnet and testnet.

### Bug Fixes

- Extended KES verification to all Shelley-descendant eras instead of Babbage only.
- Fixed a panic on Byron Epoch Boundary Blocks and a race condition in concurrent verification.
- Fixed value conservation to account for treasury withdrawals.

### Additional Changes

- Added muxer unit tests.

## v0.142.0 - metadata refactor and output pointer fixes

- **Date:** 2025-12-01
- **Version:** 0.142.0

### Summary

This release fixes output pointer aliasing that caused all items to reference the same value, prevents transaction body mutation during fee calculation, and refactors metadata for polymorphic era support.

### Bug Fixes

- Fixed output and reference input accessors to return unique pointers.
- Fixed fee calculation to operate on a copy instead of mutating the original transaction body.
- Added custom NativeScript CBOR marshaler using stored raw bytes.

### Additional Changes

- Refactored metadata for polymorphic auxiliary data across all eras.

## v0.141.0 - TX metadata, rewards, and deterministic CBOR

- **Date:** 2025-11-29
- **Version:** 0.141.0

### Summary

This release adds proper transaction metadata encoding, deterministic CBOR for multi-asset values, initial rewards calculation formulas, and Leios endorser block bodies.

### New Features

- Added proper transaction metadata type supporting all CDDL datum kinds.
- Added CIP-0005 bech32 hash representations and deterministic multi-asset CBOR encoding.
- Added Leios endorser block body support and initial rewards calculation formulas.

### Bug Fixes

- Fixed InvalidTransactions CBOR encoding and metadata null handling.
- Fixed protocol state timeout activation ordering.

### Additional Changes

- Updated CI to Go 1.25.x and updated benchmark infrastructure.

## v0.140.0 - Apex Fusion networks and certificate validation

- **Date:** 2025-11-18
- **Version:** 0.140.0

### Summary

This release adds Apex Fusion Prime network support and tightens certificate deposit validation per CIP-0094.

### New Features

- Added Apex Fusion Prime Mainnet and Testnet networks with bootstrap peers.

### Bug Fixes

- Fixed Conway certificate validation to use amount-based checks per CIP-0094.
- Fixed Leios endorser block and certificate shapes.
- Fixed certificate consistency and collateral error CBOR encoding.

### Performance

- Pre-allocated transaction input/output slices for faster processing.
- Increased default protocol queue sizes for better throughput.

## v0.139.0 - protocol limits and timeouts

- **Date:** 2025-11-10
- **Version:** 0.139.0

### Summary

This release adds Ouroboros Network Specification compliance with protocol state timeouts, message byte limits, and pipeline limits so connections are properly bounded and cleaned up.

### New Features

- Added protocol state timeouts for all 11 mini-protocols to prevent resource leaks.
- Added per-protocol message byte limits and pipeline/queue limits with connection teardown on violation.
- Added Leios protocol parameters.

### Bug Fixes

- Fixed zero-value hash encoding to use proper zero-filled bytestrings instead of CBOR null.

### Additional Changes

- Updated for utxorpc spec 0.18.1 compatibility.

## v0.138.0 - CBOR round-trip fidelity

- **Date:** 2025-11-05
- **Version:** 0.138.0

### Summary

This release adds block-level CBOR marshal functions for multiple eras so blocks can be serialized with byte-for-byte fidelity, and tightens protocol parameter validation.

### New Features

- Added block CBOR marshaling with round-trip fidelity for Mary, Alonzo, and Babbage eras.
- Added minimum pool cost to protocol parameters.

### Bug Fixes

- Fixed Byron transaction payload encoding for proper CBOR round-trips.
- Tightened Shelley rational parameter validation to reject out-of-range values.
- Fixed error messages to use the correct era name instead of a hardcoded default.

## v0.137.1 - memory optimization

- **Date:** 2025-10-31
- **Version:** 0.137.1

### Summary

This release optimizes memory layout and allocation patterns, and implements the proposed protocol parameter updates result type.

### Bug Fixes

- Implemented proposed protocol parameter updates as a typed result instead of untyped interface.
- Fixed a nil guard for debug logging when ChainSync is not active.

### Performance

- Optimized struct field alignment and allocation patterns across CBOR and command packages.

## v0.137.0 - LeiosFetch protocol

- **Date:** 2025-10-24
- **Version:** 0.137.0

### Summary

This release implements the LeiosFetch protocol (CIP-0164) for fetching blocks, transactions, and votes, and adds raw script byte access to the Script interface.

### New Features

- Added LeiosFetch protocol with client/server endpoints for blocks, transactions, votes, and block ranges.
- Added raw script byte access to the Script interface so callers can get underlying bytes without type casting.

## v0.136.0 - Leios ledger primitives

- **Date:** 2025-10-13
- **Version:** 0.136.0

### Summary

This release introduces initial Leios protocol support with ledger primitives, the LeiosNotify protocol, genesis structures, and a Conway transaction CBOR round-trip test.

### New Features

- Added LeiosNotify protocol (CIP-0164), Leios ledger primitives, genesis structures, and hash functions.

### Bug Fixes

- Fixed transaction identity handling for Leios endorser blocks and Conway parameter pre-allocation.

### Additional Changes

- Added Conway transaction CBOR round-trip test.

## v0.135.2 - nil safety enforcement

- **Date:** 2025-09-18
- **Version:** 0.135.2

### Summary

This release enforces nilaway static analysis in CI and fixes nil pointer dereference issues found during analysis.

### Bug Fixes

- Fixed Byron genesis to accept alternate seed formats and resolved nil pointer issues across the codebase.

## v0.135.1 - ScriptPurpose refactor

- **Date:** 2025-09-13
- **Version:** 0.135.1

### Summary

This release splits ScriptPurpose into separate purpose and info types for clearer separation of concerns, and adds nilaway static analysis to CI.

### Breaking Changes

- Split `ScriptPurpose` into `ScriptPurpose` and `ScriptInfo`.

### Bug Fixes

- Fixed a nil panic when redeemers are absent.

## v0.135.0 - TX output string representation

- **Date:** 2025-09-11
- **Version:** 0.135.0

### Summary

This release adds human-readable string representations for transaction outputs so validation errors are easier to understand.

### New Features

- Added string representations for transaction outputs across all eras.

### Bug Fixes

- Added PlutusData conversion for pre-Alonzo transaction outputs.

## v0.134.2 - dependency update

- **Date:** 2025-09-04
- **Version:** 0.134.2

### Summary

Dependency update release.

### Additional Changes

- Updated `plutigo` to v0.0.11.

## v0.134.1 - datum CBOR preservation

- **Date:** 2025-09-03
- **Version:** 0.134.1

### Summary

This release preserves original datum CBOR bytes on decode so re-encoding does not alter the on-chain representation.

### Bug Fixes

- Fixed datum decoding to retain original CBOR bytes.

## v0.134.0 - Plutus script context expansion

- **Date:** 2025-09-03
- **Version:** 0.134.0

### Summary

This release expands the Plutus script context to cover certificates, stake withdrawals, voting, and proposing so smart contracts can access full transaction context during validation.

### New Features

- Added certificate, withdrawal, voting, and proposing support in the Plutus script context.
- Added pool deposit collection when a pool is retired and re-registered in the same transaction.
- Added slot-to-time queries to the ledger state interface.

### Bug Fixes

- Fixed address PlutusData representations for stake-only and pointer addresses.

## v0.133.0 - PlutusV3 script evaluation

- **Date:** 2025-08-26
- **Version:** 0.133.0

### Summary

This release adds PlutusV3 script evaluation support and switches to typed script types in witness sets for better type safety.

### New Features

- Added PlutusV3 script evaluation with full script context support including datum options, required signers, and multi-policy minting.

### Breaking Changes

- Switched execution unit fields to signed integers to represent budget overruns.
- Replaced raw byte slices with typed script types in transaction witness sets.

### Bug Fixes

- Fixed CBOR marshaling for UTxO, address PlutusData, and script references.

## v0.132.0 - initial Plutus script context

- **Date:** 2025-08-19
- **Version:** 0.132.0

### Summary

This release introduces initial Plutus script context support for building V3 contexts from transactions and decodes datums as typed PlutusData.

### New Features

- Added initial Plutus V3 script context building from transactions.
- Added script hash generation helper.

### Breaking Changes

- Datums are now decoded as typed PlutusData instead of raw CBOR.
- Removed the unused `DetermineBlockData` function.

### Bug Fixes

- Fixed native script CBOR unmarshal to retain original data.

## v0.131.0 - UTxO whole query and genesis pools

- **Date:** 2025-08-12
- **Version:** 0.131.0

### Summary

This release adds a query for complete UTxO sets and creates genesis pools from Shelley genesis configuration.

### New Features

- Added UTxO whole result query with JSON output support.
- Added genesis pool creation from Shelley genesis with pool lookup by ID.

### Additional Changes

- Consolidated UTxO result types and CBOR debug utilities.

## v0.130.1 - redeemer iteration improvements

- **Date:** 2025-08-01
- **Version:** 0.130.1

### Summary

This release makes redeemers easier to iterate and consolidates genesis rational types.

### Additional Changes

- Improved redeemer iteration with shared key/value types across eras.
- Consolidated genesis rational types into a single common type.

## v0.130.0 - PlutusData for governance types

- **Date:** 2025-07-31
- **Version:** 0.130.0

### Summary

This release adds PlutusData conversion functions for governance proposals, votes, transaction outputs, and multi-asset values so they can be passed to Plutus scripts.

### New Features

- Added PlutusData conversion for governance proposals, votes, transaction outputs, and multi-asset values.

### Bug Fixes

- Fixed address PlutusData structure.

### Additional Changes

- Added utxorpc block function tests across all eras.

## v0.129.0 - PlutusData helpers

- **Date:** 2025-07-17
- **Version:** 0.129.0

### Summary

This release adds PlutusData conversion helpers for core types and block CBOR round-trip tests across eras.

### New Features

- Added PlutusData conversion for hashes, addresses, and transaction inputs.

### Additional Changes

- Added block CBOR round-trip tests for Shelley, Allegra, and Conway.
- Updated Go version requirement to 1.24 and bumped CBOR, crypto, and utxorpc dependencies.

## v0.128.2 - Conway CBOR marshaling fixes

- **Date:** 2025-07-12
- **Version:** 0.128.2

### Summary

This release fixes several Conway CBOR marshaling issues that could produce incorrect serialization for redeemers, witness sets, and generic values.

### Bug Fixes

- Fixed Conway redeemer and witness set CBOR marshaling, value type marshaling, and TxSubmission ack count reset on protocol restart.
- Fixed redeemers to preserve legacy ordering when present in on-chain data.

## v0.128.1 - nil pparam update guard

- **Date:** 2025-07-11
- **Version:** 0.128.1

### Summary

This release fixes a crash when processing transactions with no protocol parameter updates.

### Bug Fixes

- Added nil check for transaction protocol parameter updates.

## v0.128.0 - TxSubmission lifecycle and CBOR set tags

- **Date:** 2025-07-10
- **Version:** 0.128.0

### Summary

This release adds TxSubmission client completion support so applications can cleanly signal they are done submitting transactions, and introduces CBOR set tags for spec-compliant transaction body encoding.

### New Features

- Added client Done and MsgDone callback support for TxSubmission, and clean server shutdown.

### Bug Fixes

- Fixed Byron address and TX input CBOR encoding, and excluded empty datum hashes from Alonzo outputs.

### Additional Changes

- Added CBOR set tag support for transaction body fields and block test data for all eras.