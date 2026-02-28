---
title: Release notes
---

# Release notes

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

## v0.159.1 - ledger validation fixes

- **Date:** 2026-02-26
- **Version:** 0.159.1

### Summary

This release fixes several ledger validation issues discovered during testing, including correct empty-redeemer encoding, canonical redeemer index mapping, and datum justification.

### Bug Fixes

- Fixed Conway empty redeemers to encode as `0xa0` (empty map) instead of CBOR null.
- Exported `SortInputs` and applied it across PlutusV1/V2/V3, reference inputs, and validation for canonical redeemer index mapping.
- Fixed supplemental datum validation to include output datum hashes when justifying witness datums.
- Corrected stake registration/deregistration certificate encoding to include optional amount instead of a placeholder constructor.

## v0.159.0 - async LeiosNotify

- **Date:** 2026-02-25
- **Version:** 0.159.0

### Summary

This release adds asynchronous notification processing to the LeiosNotify protocol with a callback-driven loop and configurable pipelining.

### New Features

- Added `Sync()` method that starts a background notification loop, delivering messages via `NotificationFunc`.
- Added configurable `PipelineLimit` (default 10, max 100) for pipelining notification requests.
- Added `ErrStopNotificationProcess` for handlers to signal a clean shutdown.

### Breaking Changes

- Replaced `RequestNext()` with `Sync()` and `WithNotificationFunc` for notification processing.

## v0.158.4 - validation gaps and pipelining

- **Date:** 2026-02-25
- **Version:** 0.158.4

### Summary

This release fills validation gaps across Shelley through Conway eras, fixes protocol state transitions for pipelined ChainSync messages, and bumps the edwards25519 dependency.

### Bug Fixes

- Fixed protocol state transitions to queue pipelined ChainSync messages correctly, preventing stalls and out-of-order blocks during pipelined sync.
- Added overflow-safe `AddInt64Checked` when summing redeemer `ExUnits` in Alonzo, Babbage, and Conway.
- Skipped withdrawal validation for `IsValid=false` transactions in Alonzo, Babbage, and Conway.

### New Features

- Added duplicate input detection within regular, collateral, and reference input sets across all eras.

### Additional Changes

- Updated `ouroboros-mock` dependency to v0.9.1.
- Bumped `filippo.io/edwards25519` from 1.1.1 to 1.2.0.
- Updated agent documentation for mock ledger state requirements.

## v0.158.3 - transaction JSON serialization

- **Date:** 2026-02-22
- **Version:** 0.158.3

### Summary

This release fixes and stabilizes JSON serialization for transactions and script-related types across all eras, with deterministic, round-trippable output.

### Bug Fixes

- Fixed redeemer JSON encoding with deterministic sorting, duplicate key detection, and correct field names across Alonzo and Conway.
- Fixed datum JSON to encode as CBOR hex with null when absent.
- Added `MarshalText`/`UnmarshalText` for `Address`, `Blake2b160/224/256`, `Voter`, and `GovActionId` with bech32 and CIP-129 validation.
- Fixed transaction JSON output for Byron through Conway with stable `Body`/`WitnessSet`/`TxIsValid` keys.

## v0.158.2 - per-connection state maps

- **Date:** 2026-02-19
- **Version:** 0.158.2

### Summary

This release fixes shared protocol state mutation on servers by introducing per-connection state maps and bumps dependencies.

### Bug Fixes

- Fixed protocol servers to use per-connection state maps, preventing shared state mutations across connections.
- Removed non-spec `IdleTimeout` from TxSubmission.

### New Features

- Added configurable `IdleTimeout` for ChainSync per connection.

### Additional Changes

- Bumped `filippo.io/edwards25519` from 1.1.0 to 1.1.1.
- Bumped `plutigo` to v0.0.25.

## v0.158.1 - split ChainSync N2N/N2C state maps

- **Date:** 2026-02-18
- **Version:** 0.158.1

### Summary

This release splits ChainSync and Handshake into separate Node-to-Node and Node-to-Client state maps with spec-aligned timeouts.

### Breaking Changes

- Split ChainSync and Handshake into NtN/NtC state maps; defaults to NtC (no timeouts) unless `Mode=NtN`.
- TxSubmission timeouts set per spec: `Init`/`Idle`/`TxIdsBlocking=0`; `Nonblocking`/`Txs=10s`.

### Bug Fixes

- Fixed ChainSync `MustReply` to use a randomized timeout range (135–269s) per spec.
- Fixed TxSubmission blocking timeouts to prevent premature disconnects.

## v0.158.0 - protocol hardening and governance queries

- **Date:** 2026-02-17
- **Version:** 0.158.0

### Summary

This release hardens protocol and cryptographic handling with security fixes for VRF, muxer, and consensus, adds Conway governance queries, and enforces a 16MB protocol buffer limit.

### New Features

- Added `GetRatifyState` to LocalStateQuery for Conway-era ratification state including enactment details, enacted/expired actions, and delay status.
- Added committee member state query with member status, threshold, current epoch, and governance proposals with votes.

### Bug Fixes

- Fixed VRF verification to reject non-canonical s scalars and zero sensitive key-derived buffers after use.
- Fixed muxer diffusion mode race condition using atomic reads/writes.
- Fixed consensus to require issuer verification key for OpCert validation.
- Fixed CBOR block encoding to emit empty arrays instead of null for nil list fields across all eras.
- Fixed nil committee handling in governance state queries to decode CBOR null correctly.
- Added 16MB cap to protocol read buffer to prevent unbounded memory growth.
- Made KES insecure mode more explicit in protocol configuration.

### Additional Changes

- Added `SimpleVRFSigner.Destroy()` for secure key disposal.
- Bumped `golang.org/x/crypto` from 0.47.0 to 0.48.0.
- Bumped `plutigo` from 0.0.23 to 0.0.24.

## v0.157.0 - security hardening

- **Date:** 2026-02-15
- **Version:** 0.157.0

### Summary

This release hardens the muxer against denial-of-service attacks, adds strict CBOR input validation, fixes the rolling nonce calculation, and adds the DRep stake distribution query.

### New Features

- Added DRep stake distribution query to LocalStateQuery for Conway era with typed results supporting key, script, abstain, and no-confidence DReps.

### Bug Fixes

- Hardened the muxer against slowloris-style attacks by enforcing 120s read deadlines and making error propagation non-blocking.
- Added strict CBOR decoder for untrusted inputs with smaller limits (`MaxMapPairs`/`MaxArrayElements`: 131,072) and `ExtraDecErrorUnknownField`.
- Fixed rolling nonce calculation to use `eta_v XOR blake2b_256(vrfOutput)` per Ouroboros Praos, replacing the previous hash-of-concat approach.
- Fixed Conway redeemer iteration to delegate to legacy redeemers when legacy mode is enabled.

## v0.156.0 - ChainSync state split and governance queries

- **Date:** 2026-02-14
- **Version:** 0.156.0

### Summary

This release splits ChainSync client and server state to fix pipelining interference, adds filtered vote delegatees and governance proposals queries, and fixes a muxer deadlock.

### New Features

- Added filtered vote delegatees query returning `map[StakeCredential]Drep` for Conway era.
- Added governance proposals query returning active proposals with committee/DRep/SPO votes.

### Bug Fixes

- Split ChainSync client and server `StateContext` objects so pipelining on one side cannot affect the other.
- Fixed muxer `UnregisterProtocol` deadlock by deferring mutex unlock immediately after lock.

## v0.155.0 - CBOR improvements and governance state

- **Date:** 2026-02-12
- **Version:** 0.155.0

### Summary

This release refactors CBOR constructor handling, increases decode limits for large mainnet maps, adds DRep state and proposals queries, and adds bounds checking to prevent panics.

### New Features

- Added DRep state query to LocalStateQuery for Conway era with typed `map[StakeCredential]DRepStateEntry` results.
- Added governance proposals query returning active proposals with votes and epoch windows.

### Bug Fixes

- Increased CBOR decoder `MaxMapPairs` to 10,000,000 to support large stake distribution maps in mainnet snapshots.
- Added bounds checking to `ListLength` and `DecodeIdFromList` to handle empty or short CBOR input safely.
- Updated Leios protocol message shapes to align with spec: blocks use `pcommon.Point`, TX bitmaps use `map[uint16]uint64`.

### Additional Changes

- Refactored CBOR constructor handling with `ConstructorEncoder`/`ConstructorDecoder` for safer, byte-accurate encoding/decoding.
- Added comprehensive architecture documentation (`ARCHITECTURE.md`).
- Added Go 1.26.x to the CI test matrix.
- Added memory profiling and benchmark suite with allocation regression tests.

## v0.154.0 - block processing pipeline

- **Date:** 2026-02-10
- **Version:** 0.154.0

### Summary

This release introduces a concurrent block processing pipeline with parallel decode/validate stages and slot-ordered apply, caches CBOR encoder/decoder modes for significant performance gains, and bumps plutigo.

### New Features

- Added concurrent block processing pipeline with parallel decode/validate and slot-ordered apply, wired into ChainSync and BlockFetch with prefetch, backpressure, and safe rollback draining.
- Configurable workers, buffers, per-block lifecycle, and latency metrics with a default pending limit of k=2160.

### Performance

- Cached CBOR `EncMode`/`DecMode` with thread-safe lazy initialization (`sync.Once`), reducing allocations and improving encode/decode performance in hot paths.

### Additional Changes

- Bumped `plutigo` from 0.0.22 to 0.0.23.

## v0.153.1 - performance optimizations and CPRAOS

- **Date:** 2026-02-06
- **Version:** 0.153.1

### Summary

This release delivers substantial performance optimizations across VRF, KES, consensus, and ledger paths, switches leader election to CPRAOS, enforces VRF key uniqueness in PV11, and adds support for alternate genesis config keys.

### Bug Fixes

- Switched consensus to CPRAOS leader election by hashing VRF output with BLAKE2b-256 ("L" prefix) and comparing against 2^256 threshold, with mode-aware APIs for TPraos compatibility.
- Enforced VRF key uniqueness across pools starting with protocol version 11; same-pool reuse is still allowed.
- Fixed Alonzo genesis config to accept alternate key names (`exUnitsMem`/`exUnitsSteps` or `memory`/`steps`; `prSteps`/`prMem` or `priceSteps`/`priceMemory`).
- Fixed Conway committee threshold to use `GenesisRat` (rational) for proper decoding.

### Performance

- Switched VRF scalar math to edwards25519 native operations with fixed-size arrays and zero-alloc proof parsing.
- Optimized KES with stack buffers, `blake2b.Sum256` in `HashPair`/`expandSeed`, public key caching, and in-place right subtree key generation.
- Replaced `slices.Concat` with fixed `[64]byte` buffers for nonce hashing with 32-byte input validation.
- Pre-allocated slice capacity for Plutus map pairs and block body hash calculations.
- Used fixed 65-byte buffer in `VrfLeaderValue` to avoid allocations.
- Reduced `big.Rat` allocations in consensus threshold math by reusing constants and temporaries.
- Pre-allocated leaf and branch buffers for Byron Merkle root hashing.

### Additional Changes

- Added comprehensive VRF and KES benchmarks covering key generation, signing, verification, and parallel workloads.

## v0.153.0 - protocol version 11 and conformance testing

- **Date:** 2026-02-05
- **Version:** 0.153.0

### Summary

This release adds protocol version 11 (VanRossem) support, migrates conformance tests to the ouroboros-mock framework, implements the NonMyopicMemberRewards query, and adds comprehensive protocol and address tests.

### New Features

- Added protocol version 11 support enforcing unique VRF keys across pools, CC voting restrictions, and skipping `NonDisjointRefInputs` for Plutus V1/V2.
- Added `IsProtocolVersionAtLeast` helper and protocol version constants.
- Implemented `NonMyopicMemberRewards` query accepting stakes input for per-stake/per-pool reward mapping.

### Bug Fixes

- Fixed `UtxoValidateCCVotingRestrictions` to return an error on non-Conway protocol parameters instead of silently skipping validation.

### Additional Changes

- Migrated conformance tests to ouroboros-mock framework (v0.9.0) using shared harness and embedded test vectors, removing ~3k lines of bespoke test code.
- Added unit tests for Leios Fetch/Notify protocols, CBOR stream operations, muxer lifecycle, and CIP-0019 address types.
- Pinned GitHub Actions to specific commit SHAs.

## v0.152.2 - UTxO overhead constant fix

- **Date:** 2026-02-02
- **Version:** 0.152.2

### Summary

This release fixes the minimum coin calculation per CIP-0055 and adds pool retire/register test coverage.

### Bug Fixes

- Fixed `MinCoinTxOut` to include the CIP-0055 160-byte UTxO overhead constant: `coinsPerUTxOByte * (160 + serializedOutputSize)`. Applies to Babbage and Conway.

### Additional Changes

- Added Conway test verifying that pool retire and re-register in the same transaction succeeds.

## v0.152.1 - CBOR offset fixes

- **Date:** 2026-02-01
- **Version:** 0.152.1

### Summary

This release fixes CBOR offset calculations for indefinite-length arrays and adds per-protocol documentation.

### Bug Fixes

- Fixed CBOR offset calculations for indefinite-length arrays, correctly handling `0x9f` arrays with 1-byte headers for transaction bodies, witnesses, and outputs.

### Additional Changes

- Added protocol overview and per-protocol documentation for all Ouroboros mini-protocols with state machines, messages, timeouts, limits, and Go usage examples.

## v0.152.0 - CBOR byte offsets and Byron consensus

- **Date:** 2026-02-01
- **Version:** 0.152.0

### Summary

This release adds single-pass CBOR byte offset extraction for block components, implements initial Byron consensus validation, fixes Plutus script context encoding, and standardizes mock ledger state usage.

### New Features

- Added `StreamingBlockDecoder` for single-pass per-transaction CBOR offset extraction covering bodies, witnesses, metadata, outputs, datums, redeemers, and scripts.
- Added `NewBlockFromCborWithOffsets` helper for offset-aware block decoding.
- Implemented Byron OBFT consensus validation: protocol magic, epoch boundary, slot/block sequence, slot leader rotation, signature verification, and body hash checks.

### Bug Fixes

- Fixed Plutus script context `TxInInfo`/`TxOutRef` encoding per Plutus V2; value encoding now omits ADA for `txInfoMint`.
- Fixed Plutus evaluation to return consumed budget even on failure.
- Fixed duplicate datum hash entries in Plutus script context by deduplicating by datum hash.
- Added `WithLeiosFetchConfig` and `WithLeiosNotifyConfig` connection functions for Leios protocol configuration.

### Additional Changes

- Replaced internal `MockLedgerState` with `ouroboros-mock/ledger` across all tests using `NewLedgerStateBuilder()`.
- Bumped `ouroboros-mock` to v0.6.0.
- Added package-level documentation for `cbor`, `ledger/common`, `blockfetch`, and `localstatequery`.
- Added development and agent documentation (`DEVELOPMENT.md`, expanded `AGENTS.md`).

## v0.151.2 - Plutus mint encoding fix

- **Date:** 2026-01-28
- **Version:** 0.151.2

### Summary

This release fixes Plutus data conversion for mint values in V1/V2 script contexts and bumps CI action dependencies.

### Bug Fixes

- Fixed mint-to-Plutus-data conversion for TxInfoV1/V2 to use `Mint.ToPlutusData()` directly, matching expected CBOR encoding and avoiding zero-ADA wrapper.

### Additional Changes

- Bumped `actions/checkout` from v4 to v6, `actions/upload-artifact` from v4 to v6, `actions/setup-go` from v5 to v6.
- Bumped `webiny/action-conventional-commits` from 1.3.0 to 1.3.1.
- Bumped `ouroboros-mock` from 0.4.0 to 0.5.0.

## v0.151.1 - Plutus V2 cost model fix

- **Date:** 2026-01-25
- **Version:** 0.151.1

### Summary

This release fixes Plutus V2 cost model indexing in Conway and adds a CBOR-preserving datum list type for exact script data hash computation.

### Bug Fixes

- Fixed Conway ledger rules to use `CostModels[1]` for Plutus V2 scripts instead of an incorrect index.
- Fixed `InvalidHereafter` TTL logic to treat as "at or before" slot.

### New Features

- Added `PlutusDataList` type preserving original CBOR encoding of Plutus datums for exact `ScriptDataHash` computation in Alonzo and Babbage.

## v0.151.0 - transaction errors and cost model merging

- **Date:** 2026-01-25
- **Version:** 0.151.0

### Summary

This release adds era-aware transaction error decoding for Babbage and Conway, fixes cost model merging from protocol parameters, and introduces fuzz testing across the codebase.

### New Features

- Added era-aware decoding for UTXOW/UTXO predicate failures with full Babbage and Conway transaction error support and wrapper types for nested era failures.

### Bug Fixes

- Fixed cost model merging to merge incoming `CostModels` into existing protocol parameter maps instead of overwriting.
- Switched Plutus evaluation to use `cek.EvalContext` for V1/V2/V3.

### Additional Changes

- Added fuzz targets across CBOR, ledger (blocks, headers, transactions, addresses), KES, and VRF with a nightly GitHub Actions workflow and seeded corpora.
- Added unit tests for connection options, CBOR generic encode/decode, and defensive nil-checks for script data hash comparison.
- Bumped `plutigo` to v0.0.22.

## v0.150.0 - governance queries and protocol hardening

- **Date:** 2026-01-20
- **Version:** 0.150.0

### Summary

This release adds Conway-era governance state queries, provides cost models for Plutus script evaluation, hardens protocol client lifecycles and muxer concurrency, and fixes several CBOR and ledger edge cases.

### New Features

- Added Conway-era governance queries to LocalStateQuery: `GetConstitution`, `GetGovState`, `GetDRepState`, `GetDRepStakeDistr`, `GetCommitteeMembersState`, `GetFilteredVoteDelegatees`, and `GetSPOStakeDistr`.
- Added cost model support for Plutus script evaluation, passing cost models to the CEK machine for V1/V2/V3 under Conway.

### Bug Fixes

- Fixed Alonzo genesis cost model decoding to support both list and map JSON formats with ordered `[]int64` per Plutus version.
- Fixed blockfetch and txsubmission client lifecycles to align with ChainSync-style logic; `Start`/`Stop` are now idempotent and restartable without races or leaks.
- Fixed chainsync pipeline defaults when bare `Config{}` is passed, preventing sync stalls from zero `PipelineLimit`.
- Fixed muxer data race in receiver channels that could cause shutdown panics by wrapping with mutex and setting to nil after close.
- Fixed bootstrap witnesses to be included in native script validation.
- Gated disjoint reference inputs rule to post-Babbage.
- Added bounds checking to prevent panics: address data length validation, CBOR `Rat` zero-denominator guard, CBOR type checks before slice operations.

### Additional Changes

- Added unit tests for blockfetch and txsubmission message round-trips.
- Bumped `plutigo` to v0.0.21, `golang.org/x/crypto` from 0.46.0 to 0.47.0.
- Standardized formatting across consensus, ledger, KES, protocol, and test packages.

## v0.149.0 - VRF, KES, and consensus packages

- **Date:** 2026-01-16
- **Version:** 0.149.0

### Summary

This release introduces standalone pure-Go packages for VRF, KES, and consensus with conformance tests, adds operational certificate verification, and updates the feature checklist.

### New Features

- Added pure-Go VRF package (ECVRF-ED25519-SHA512-Elligator2) with `KeyGen`, `Prove`, `Verify`, `VerifyAndHash`, `ProofToHash`, and `MkInputVrf` with conformance test vectors.
- Added pure-Go KES package (MMM Sum-composition, depth 6) with `KeyGen`, `Sign`, `Update`, `PublicKey`, and `VerifySignedKES` with conformance tests.
- Added consensus package with Ouroboros Praos/Genesis leader election and thresholds, block builder, header validation, chain selection, Genesis rule, and Byron OBFT header validation with conformance tests.
- Added operational certificate verification with `VerifyOpCertSignature`, `ValidateKesPeriod`, `ValidateOpCert`, and `CreateOpCert`.

### Breaking Changes

- Replaced `ledger.VerifyVrf` with `vrf.Verify` and `ledger.MkInputVrf` with `vrf.MkInputVrf`.
- Use `kes.Verify` for KES checks and `ledger.VerifyOpCert` for operational certificates.

## v0.148.0 - script data hash validation and conformance tests

- **Date:** 2026-01-13
- **Version:** 0.148.0

### Summary

This release adds ScriptDataHash validation with malformed reference script detection, implements stake snapshot and pool state queries, enables ChainSync client reuse, and introduces Amaru conformance test vectors.

### New Features

- Added `ScriptDataHash` verification, malformed reference script detection, redeemer bounds checking for Conway, and Conway certificate Plutus version compatibility checks with original CBOR preservation for redeemers.
- Added stake snapshot query supporting multiple pools in a single request with detailed per-pool snapshot data.
- Added `PoolStateResult` with full registration parameters, current/future state, retiring records, and deposit info.
- Added ChainSync `Stop`/`Start` reuse on the same connection without reconnecting, with graceful shutdown via stop signal.

### Bug Fixes

- Fixed block body hash calculation to include invalid transaction indices.

### Additional Changes

- Added conformance tests using Amaru test vectors.
- Added `AGENTS.md` for robot development guidelines.
- Bumped `go-ethereum` from 1.16.7 to 1.16.8.

## v0.147.0 - validation overhaul and Plutus script contexts

- **Date:** 2026-01-11
- **Version:** 0.147.0

### Summary

This release delivers a major validation overhaul reaching 302/314 conformance tests, builds PlutusV1 and V2 script contexts, switches monetary amounts to `*big.Int`, adds governance state, and fixes numerous ledger edge cases.

### New Features

- Added PlutusV1 and V2 script context building with full `TxInfo` construction.
- Added governance state implementation for the ledger.
- Added CIP-0005 bech32 string representations for all common types.
- Added `ShelleyStakePoolParamsQuery` for protocol-level stake pool parameter queries.

### Breaking Changes

- Replaced fixed-width integer types with `*big.Int` for monetary amounts and multi-asset outputs to prevent overflow.

### Bug Fixes

- Overhauled delegation, withdrawal, native-script evaluation, `ScriptDataHash` checks, and governance/committee validations with new error types.
- Fixed PlutusData panic on unsupported type conversions.
- Fixed V1/V2 script context issues: correct context selection, null datum wrapping, data format, and witness datum handling.
- Fixed Conway proposal rules for empty treasury withdrawals and invalid networks.
- Fixed protocol parameter proposal rules and errors.
- Fixed multi-asset conservation validation.
- Fixed CIP-0033 reference scripts from inputs.
- Fixed inline datums to be refused in Plutus V1.
- Fixed governance script witness handling.
- Fixed treasury donation subtraction in value conservation.
- Fixed race condition around protocol start when acting as server.
- Fixed zero-byte read panic in protocol `readLoop`.
- Improved handshake refusal message handling with specific error types.
- Added `DoneChan` to txsubmission `CallbackContext`.
- Fixed registration to not require witnesses.

### Additional Changes

- Refactored rules, errors, and formatting across ledger packages.
- Added CIP compliance tests for CIP-0014, CIP-0019, CIP-0031, CIP-0033, CIP-0006.
- Removed legacy binaries from the repository.

## v0.146.0 - signature validation and CIP-0137

- **Date:** 2026-01-03
- **Version:** 0.146.0

### Summary

This release adds full transaction signature validation, implements the CIP-0137 distributed message queue protocol, fixes Byron witness validation, adds TX auxiliary data decoding across all eras, and delivers extensive CIP conformance testing.

### New Features

- Added full transaction signature validation for the ledger.
- Added CIP-0137 distributed message queue protocol support.
- Added TX auxiliary data decoding across all eras.
- Added handshake query reply message handling and version data query support.
- Added witness validation rules with tests.
- Added metadata validation during rule checks.
- Added `IsValid`, collateral redeemer, and cost model checks for the ledger.
- Updated Leios implementation per latest CIP-0164.

### Bug Fixes

- Fixed Byron witness validation with correct address root computation using pubkey, chain code, and raw attributes.
- Fixed network nil handling to support omission.
- Fixed Conway network validation.
- Fixed metadata size validation and auxiliary data unwrap before unmarshal.
- Fixed lenient Conway witness switch.
- Fixed script data hash computation without redeemers.
- Fixed metadata store and validation.
- Fixed inline datum hash returns.
- Fixed Babbage cost model rules to align with Conway.
- Fixed chainsync pipeline deadlock with default configuration.
- Fixed muxer race condition on shutdown causing panic.
- Fixed Conway collateral return simplification.
- Fixed block-fetch timeouts and limits to match spec.
- Fixed CBOR error handling to not ignore errors.
- Fixed protocol done channel selection.
- Used `slices.Clone` instead of `append` to nil slice for correct copy semantics.

### Additional Changes

- Added CIP conformance tests: CIP-0033, CIP-0006, CIP-0031, CIP-0019, CIP-0014.
- Centralized validation errors across ledger packages.
- Added configurable mock ledger state for testing.
- Added CBOR indefinite-length map encoding tests.
- Enabled `unparam` linter in golangci-lint.
- Bumped `plutigo` to v0.0.18, `golang.org/x/crypto` from 0.45.0 to 0.46.0.

## v0.145.0 - structured validation errors

- **Date:** 2025-12-13
- **Version:** 0.145.0

### Summary

This release introduces structured validation errors with typed error categories, adds pool distribution queries and CIP-129 voter string representations, and standardizes import aliases.

### New Features

- Added `ValidationError` and `ValidationErrorType` with categories (`body_hash`, `transaction`, `stake_pool`, `vrf`, `kes`, `protocol`, `configuration`) and contextual details (era, slot, block number, tx hash).
- Added `Voter.String()` returning CIP-129 bech32 identifiers for all voter types.
- Added `PoolDistrResult` for pool distribution queries by pool ID.

### Bug Fixes

- Included era in block verification error output for easier troubleshooting.

### Additional Changes

- Standardized `lcommon` (ledger/common) and `pcommon` (protocol/common) import aliases across the codebase.
- Surfaced errors in verify block tests.

## v0.144.0 - stake pool validation

- **Date:** 2025-12-09
- **Version:** 0.144.0

### Summary

This release adds stake pool validation to block verification, centralizes header field extraction across eras, and fixes metadata handling.

### New Features

- Added stake pool validation to `VerifyBlock`, checking that blocks are produced by registered pools with matching VRF keys. Supports Shelley through Conway with opt-out via `SkipStakePoolValidation`.
- Added centralized header field extraction helper for issuer vkey and VRF key across eras.

### Bug Fixes

- Fixed metadata handling to use raw bytes.

## v0.143.0 - transaction validation and KES verification

- **Date:** 2025-12-05
- **Version:** 0.143.0

### Summary

This release wires transaction validation into block verification, extends KES verification to all Shelley-era descendants, adds public network roots, introduces muxer unit tests, and fixes several ledger edge cases.

### New Features

- Wired transaction validation into `VerifyBlock` using era-specific UTXO rules (Shelley through Conway) with `SkipTransactionValidation`, `LedgerState`, and `ProtocolParameters` config fields.
- Added `VerifyTransaction` function for centralized UTXO validation.
- Added `ValidateBlockBodyHash` centralized in `ledger/common` with opt-out via `SkipBodyHashValidation`.
- Added public roots in named networks with `NetworkPublicRoot` and `NetworkAccessPoint` types, configured for mainnet and testnet.

### Bug Fixes

- Extended KES verification to all Shelley+ eras instead of Babbage-only.
- Fixed panic when calling `IssuerVkey` on Byron Epoch Boundary Blocks.
- Fixed UTxO value conservation to account for ADA minted/burned via treasury withdrawals.
- Fixed `VerifyConfig` to be per-caller instead of global, preventing races in concurrent verification.
- Fixed chainsync pipeline limit to 10 for controlled concurrency and memory.
- Simplified invalid transaction processing.

### Additional Changes

- Added comprehensive muxer unit tests covering concurrency, error handling, and protocol operations.
- Bumped `plutigo` to v0.0.16.
- Replaced `interface{}` with `any` across the codebase.

## v0.142.0 - polymorphic metadata and output pointer fixes

- **Date:** 2025-12-01
- **Version:** 0.142.0

### Summary

This release fixes output pointer aliasing, adds custom NativeScript CBOR marshaling, refactors metadata to support polymorphic types across eras, and prevents body mutation during fee calculation.

### Bug Fixes

- Fixed `Outputs()` and `ReferenceInputs()` to return unique pointers, preventing all items from referencing the same range variable.
- Fixed `MinFeeTx` to encode a local copy with `TxFee=0` instead of mutating the original transaction body.
- Added custom NativeScript CBOR marshaler using stored raw bytes when present.

### Additional Changes

- Refactored transaction metadata to support polymorphic auxiliary data types across all eras.
- Bumped `plutigo` to v0.0.15 (Plutus V3 CEK machine version [1, 2, 0]).
- Bumped `ouroboros-mock` to v0.4.0 (negative test cases and client handshake entries).

## v0.141.0 - TX metadata, rewards, and deterministic CBOR

- **Date:** 2025-11-29
- **Version:** 0.141.0

### Summary

This release adds proper transaction metadata encoding/decoding, deterministic CBOR for MultiAsset, initial rewards calculation, Leios endorser block bodies, bech32 hash representations, and fixes InvalidTransactions CBOR serialization.

### New Features

- Added `TransactionMetadatum` type replacing `cbor.LazyValue`, supporting int, bytes, text, lists, and maps per CDDL.
- Added CIP-0005 bech32 representations for hashes with `Blake2b224.Bech32(prefix)` and `NewScriptHashFromBech32`.
- Added deterministic CBOR encoding for `MultiAsset` using `SortCoreDeterministic`.
- Added `IndefLengthMap` CBOR encoder with deterministic key ordering.
- Added Leios endorser block body with CBOR array encoding and `BlockBodyHash()`.
- Added initial rewards calculation and distribution formulas with `RewardService` and reward-related `LedgerState` APIs.

### Bug Fixes

- Fixed `InvalidTransactions` CBOR encoding to use indefinite-length arrays with strict type/range checks across Babbage and Conway.
- Fixed metadata encoding/decoding with `LazyValue` to `TransactionMetadatum` conversion and null handling.
- Fixed protocol state timeout to delay activation until after initial state is set.

### Additional Changes

- Updated CI to test on Go 1.25.x.
- Updated benchmarks to use `b.Loop()` for lower loop overhead.
- Bumped `plutigo` from 0.0.13 to 0.0.14, `golang.org/x/crypto` from 0.43.0 to 0.45.0.

## v0.140.0 - Apex Fusion networks and certificate validation

- **Date:** 2025-11-18
- **Version:** 0.140.0

### Summary

This release adds Apex Fusion Prime network support, certificate deposit validation, CIP-0094 amount handling for Conway, Leios endorser block fixes, and performance improvements for transaction processing.

### New Features

- Added Apex Fusion Prime Mainnet and Prime Testnet networks with bootstrap peers and Cardano-prefixed network identifiers with legacy compatibility aliases.
- Added Leios era to the protocol version map.

### Bug Fixes

- Fixed Conway registration/deregistration certificates to use amount-based validation per CIP-0094.
- Fixed certificate deposit validation with explicit checks and clear error types.
- Fixed Leios endorser block shape and EB certificate shape.
- Fixed empty leiosfetch message guard.
- Fixed `PoolKeyHash` usage for certificate consistency across eras.
- Fixed utxorpc certificate type in return values.
- Fixed `CollateralContainsNonADA` error CBOR structure.

### Performance

- Pre-allocated transaction input/output slices with 3x improvement for Conway transaction type determination.
- Increased default protocol receive/pipeline sizes for better throughput.

### Additional Changes

- Added comprehensive serde benchmarks across all eras.
- Added all certificate type verification tests.
- Added GoDoc comments for keepalive and blockfetch packages.
