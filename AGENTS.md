# Gouroboros Agent Guide

Go library for Cardano: ledger validation across all eras (Byron→Leios), Ouroboros network protocols, CBOR. Primary agent reference; Claude-specific layer in `CLAUDE.md`.

## Commands

| Action | Command |
|--------|---------|
| Tests + race | `make test` |
| Lint | `make lint` |
| Format | `make format` |
| Conformance | `go test -v ./internal/test/conformance/...` |
| Build | `go build ./...` |

All must pass before submitting. 80-col limit (`golines`), Apache-2.0 header on new files.

## Layout

```
ledger/            ledger types + validation
  common/          shared interfaces, types, rules
    script/        Plutus script context + purpose wrappers (V1/V2/V3)
  byron/           standalone legacy format
  shelley/ allegra/ mary/ alonzo/ babbage/ conway/ leios/
protocol/          Ouroboros mini-protocols (chainsync, blockfetch,
                   txsubmission, localstatequery, ...)
cbor/              CBOR helpers (EncMode/DecMode cached globally)
connection/        network connection mgmt
muxer/             protocol multiplexing
kes/ vrf/ consensus/   crypto primitives (used by ledger/verify_*.go)
pipeline/          block processing (decode → validate → apply)
internal/test/     test utilities + conformance harness
```

Era lineage: Shelley → Allegra → Mary → Alonzo → Babbage → Conway → Leios. Byron is standalone.

## Validation rule signature

```go
func UtxoValidate{Name}(
    tx common.Transaction,
    slot uint64,
    ls common.LedgerState,
    pp common.ProtocolParameters,
) error
```

Returns `nil` or a typed error. Later eras often delegate to earlier ones, but not universally — Conway `UtxoValidateWithdrawals` has its own body. Always grep the function before claiming delegation.

## Key interfaces

- `ledger/common/state.go` — `LedgerState` embeds `UtxoState`, `CertState`, `SlotState`, `PoolState`, `RewardState`, `GovState`; exposes `NetworkId()`, `CostModels()`.
- `ledger/common/tx.go` — `Transaction`: `Hash()`, `Cbor()`, `IsValid()`, `Consumed()`, `Produced()`, `Witnesses()`.

## CBOR

Embeddable helpers:
- `cbor.StructAsArray` — encode struct as array.
- `cbor.DecodeStoreCbor` — preserve original bytes. Requires a custom `UnmarshalCBOR` that calls `SetCbor(cborData)`; retrieve via `.Cbor()`.
- `cbor.RawMessage` — deferred decode.
- `cbor.ByteString` — map-key-safe bytes.
- `cbor.Tag`, `cbor.RawTag` — semantic tags.

Rules:
- Hash from preserved `.Cbor()` bytes, never re-encoded.
- Map key order uses Cardano rules (e.g. shortLex for language views).
- Indefinite vs definite length matters for some encodings.
- `EncMode`/`DecMode` are globally cached via `sync.Once`; don't construct per call.

## ScriptDataHash

```
Blake2b256(redeemers_cbor || datums_cbor || language_views_cbor)
```

Language-views map, keys sorted shortLex (length-first, then lex):
- PlutusV1 (double-bagged): tag `serialize(serialize(0))` = `0x4100`; params `serialize(indefinite_list(cost_model))` wrapped in bytestring.
- PlutusV2/V3: tag `serialize(version)` = `0x01`/`0x02`; params `definite_list(cost_model)` unwrapped.

## Native script evaluation

`NativeScript.Evaluate(slot, validityStart, validityEnd, keyHashes) bool` at `ledger/common/script.go`. Types: `Pubkey`, `All`, `Any`, `NofK`, `InvalidBefore`, `InvalidHereafter`.

## Governance ratification

Two-phase at epoch boundaries:
1. Enact proposals ratified in the previous epoch (updates roots).
2. Ratify new proposals using updated roots.

Ordering ensures sibling proposals resolve correctly.

## Mock fixtures

All mock fixtures come from `github.com/blinklabs-io/ouroboros-mock`. Single source of truth across dingo, adder, shai, apollo, and downstream apps. If a fixture is missing, add it upstream and bump the dep — never inline a local mock.

| Need | Package |
|------|---------|
| LedgerState, UTxO, pools, pparams, governance, rewards | `ouroboros-mock/ledger` |
| Mock transactions / builders (`*MockTransaction`) | `ouroboros-mock/ledger` |
| Network conversations, connection/handshake mocks | `ouroboros-mock` (root) |
| Block/tx harnesses, protocol param JSON | `ouroboros-mock/fixtures` |

`NewTransactionBuilder()` returns `*MockTransaction`. The `TransactionBuilder` interface only exposes `WithId`, `WithInputs`, `WithOutputs`, `WithFee`, `WithTTL`, `WithMetadata`, `WithValid`, `Build`. Use the concrete type for `WithWithdrawals`, `WithCollateral`, `WithReferenceInputs`, `WithCertificates`, `WithRequiredSigners`, `WithScriptDataHash`, `WithMint`, `WithValidityIntervalStart`.

## Testing

Use `testify`: `require` for fatal, `assert` for continue-on-fail.

```go
import (
    "github.com/stretchr/testify/require"
    mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

ls := mockledger.NewLedgerStateBuilder().
    WithNetworkId(0).
    WithUtxos([]lcommon.Utxo{testUtxo}).
    Build()
tx, err := mockledger.NewTransactionBuilder().
    WithInputs(in).WithOutputs(out).WithFee(200000).Build()
require.NoError(t, err)
```

## Where to make changes

| Task | Files |
|------|-------|
| Add/fix validation rule | `ledger/{era}/rules.go`; register in `UtxoValidationRules`; error in `ledger/{era}/errors.go` (or `common/errors.go` if shared); add conformance vector if applicable |
| Transaction body fields | `ledger/{era}/shelley.go` or era-specific file |
| Witnesses | `ledger/common/witness.go` |
| Fee calculation | `MinFeeTx` in `ledger/{era}/rules.go` |
| Native scripts | `ledger/common/script.go` (`NativeScript` and variants) |
| Plutus scripts | `ledger/common/script.go` (`PlutusV1Script`, `PlutusV2Script`, `PlutusV3Script`); context/purpose wrappers in `ledger/common/script/` |
| Script validation | `ValidateScriptWitnesses` in `ledger/common/rules.go`; `UtxoValidateScriptWitnesses` in `ledger/{alonzo,babbage,conway}/rules.go` |
| Script data hash | `TransactionBodyBase.ScriptDataHash()` in `ledger/common/tx.go`; `UtxoValidateScriptDataHash` in `ledger/{alonzo,babbage,conway}/rules.go` |
| Certificate types | `ledger/common/certs.go` (+ CBOR tags, parsing switch, era rule) |
| Governance (Conway) | `ledger/conway/gov.go` |
| Protocol messages | `protocol/{name}/messages.go` |
| Protocol client/server | `protocol/{name}/{client,server}.go` |
| Block verification | `ledger/verify_block.go`, `verify_block_body.go`, `verify_kes.go`, `verify_opcert.go`; `vrf/vrf.go`, `kes/kes.go` |
| New era | `ledger/{era}/` + compat exports at `ledger/{era}.go` |
| Conformance failure | grep rule name in `ledger/`; cross-ref `internal/test/cardano-blueprint/src/ledger/`; compare vector JSON in the `github.com/blinklabs-io/ouroboros-mock` module under `conformance/testdata/` (embedded, extracted at test time) |

## Error catalog

### Shelley

| Error | Rule | Cause |
|-------|------|-------|
| `ExpiredUtxoError` | `UtxoValidateTimeToLive` | TTL < current slot |
| `InputSetEmptyUtxoError` | `UtxoValidateInputSetEmptyUtxo` | no inputs |
| `FeeTooSmallUtxoError` | `UtxoValidateFeeTooSmallUtxo` | fee below `MinFeeTx()` |
| `BadInputsUtxoError` | `UtxoValidateBadInputsUtxo` | input not in UTxO set |
| `WrongNetworkError` | `UtxoValidateWrongNetwork` | output address wrong network |
| `ValueNotConservedUtxoError` | `UtxoValidateValueNotConservedUtxo` | inputs ≠ outputs + fee |
| `OutputTooSmallUtxoError` | `UtxoValidateOutputTooSmallUtxo` | output below min UTxO |
| `MaxTxSizeUtxoError` | `UtxoValidateMaxTxSizeUtxo` | transaction too large |

### Allegra

| Error | Rule | Cause |
|-------|------|-------|
| `OutsideValidityIntervalUtxoError` | `UtxoValidateValidityInterval` | slot outside validity range |
| `NativeScriptFailedError` | script evaluation | native script failed |

### Alonzo

| Error | Rule | Cause |
|-------|------|-------|
| `ExUnitsTooBigUtxoError` | `UtxoValidateExUnitsTooBig` | execution units exceed max |
| `InsufficientCollateralError` | `UtxoValidateCollateral` | collateral below required |
| `CollateralContainsNonAdaError` | `UtxoValidateCollateral` | collateral has tokens |
| `NoCollateralInputsError` | `UtxoValidateCollateral` | collateral not specified |

### Conway

| Error | Rule | Cause |
|-------|------|-------|
| `DelegateVoteToUnregisteredDRepError` | `UtxoValidateDelegation` | DRep not registered |
| `StakeCredentialAlreadyRegisteredError` | `UtxoValidateDelegation` | re-registering stake key |
| `PlutusScriptFailedError` | script execution | script returned False |

### Shared

| Error | Rule | Cause |
|-------|------|-------|
| `MissingCostModelError` | script validation | no cost model for Plutus version |
| `ScriptDataHashMismatchError` | `UtxoValidateScriptDataHash` | hash mismatch against witnesses |
| `MissingVKeyWitnessesError` | `UtxoValidateRequiredVKeyWitnesses` | required signature missing |
| `MalformedReferenceScriptsError` | `UtxoValidateMalformedReferenceScripts` | invalid UPLC bytecode |

Error files: `ledger/{shelley,allegra,alonzo,babbage,conway,common}/errors.go`.

## Pitfalls

1. Reference inputs: resolved but never consumed from UTxO set.
2. Collateral: only consumed when `IsValid=false`.
3. Datum lookup: check witness set, inline datums, AND reference inputs.
4. Cost models: required per Plutus version used.
5. Era delegation is not universal — read the function body. Conway `UtxoValidateWithdrawals` has a custom impl.
6. Hash from preserved CBOR bytes, not re-encoded data.
7. `DecodeStoreCbor` requires a custom `UnmarshalCBOR` calling `SetCbor()`.
8. Withdrawal amount validation is intentionally disabled (see `NOTE:` at `conway/rules.go` `UtxoValidateWithdrawals`). Spec requires `amount == balance`, not `amount > 0`; zero-amount withdrawals from zero-balance accounts are spec-valid. Multi-tx balance tracking is deferred.
9. Read code before claiming "just delegates" or "missing check". `NOTE:` comments mark deliberate decisions.

## Reviewer guardrails

1. Do not hallucinate APIs. `UtxoValidateNoDuplicateInputs` and `DuplicateInputError` do not exist. The `TransactionBuilder` interface has no `WithWithdrawals` (the concrete `*MockTransaction` does).
2. Do not assume delegation. Most Conway rules delegate to Shelley; not all.
3. Do not propose Cardano spec checks without verifying the spec. E.g. withdrawal amount must equal the exact reward balance, not just be positive.
4. Respect `NOTE:` comments — they mark intentional omissions and spec deviations.

## Related

- `README.md` — feature checklist, manual testing
- `internal/test/conformance/README.md` — conformance harness
- `protocol/PROTOCOL_LIMITS.md` — protocol buffer limits
