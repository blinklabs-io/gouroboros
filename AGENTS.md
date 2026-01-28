# AGENTS.md - AI Coding Assistant Guide

This guide helps AI coding assistants work effectively with the gouroboros codebase.

## Project Overview

**gouroboros** is a Go library implementing Cardano blockchain protocols. It provides:
- Ledger types and validation across all Cardano eras (Byron through Conway)
- Ouroboros network protocol implementation (ChainSync, BlockFetch, TxSubmission, etc.)
- CBOR serialization/deserialization for blockchain data

## Quick Reference

| Action | Command |
|--------|---------|
| Run all tests | `go test ./...` |
| Run tests with race detection | `make test` |
| Run linter | `make lint` or `golangci-lint run ./...` |
| Format code | `make format` |
| Run conformance tests | `go test -v ./internal/test/conformance/...` |
| Build all packages | `go build ./...` |

## Project Structure

```text
ledger/              # Blockchain ledger types and validation
├── common/          # Shared types, interfaces, utilities
│   └── script/      # Script validation (Native, PlutusV1/V2/V3)
├── byron/           # Byron era
├── shelley/         # Shelley era (base validation logic)
├── allegra/         # Allegra era
├── mary/            # Mary era (multi-asset)
├── alonzo/          # Alonzo era (Plutus scripts)
├── babbage/         # Babbage era (reference scripts, inline datums)
├── conway/          # Conway era (governance)
└── leios/           # Leios era

protocol/            # Ouroboros network mini-protocols
├── chainsync/       # Chain synchronization
├── blockfetch/      # Block retrieval
├── txsubmission/    # Transaction submission
├── localstatequery/ # Local node queries
└── ...

cbor/                # Custom CBOR utilities
connection/          # Network connection management
muxer/               # Protocol multiplexing
internal/test/       # Test utilities and conformance tests
```

## Package Dependencies

```text
                    ┌─────────────┐
                    │ ledger/common │ ◄── Base types, interfaces
                    └──────┬──────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌─────────┐  ┌─────────┐  ┌─────────┐
        │  byron  │  │ shelley │  │  cbor   │
        └─────────┘  └────┬────┘  └─────────┘
                          │
              ┌───────────┼───────────┐
              ▼           ▼           ▼
        ┌─────────┐ ┌─────────┐ ┌─────────┐
        │ allegra │ │  mary   │ │   kes   │
        └────┬────┘ └────┬────┘ └─────────┘
             │           │
             └─────┬─────┘
                   ▼
             ┌─────────┐
             │  alonzo │ ◄── Plutus scripts
             └────┬────┘
                  │
             ┌────┴────┐
             ▼         ▼
       ┌─────────┐ ┌─────────┐
       │ babbage │ │   vrf   │
       └────┬────┘ └─────────┘
            │
            ▼
       ┌─────────┐
       │ conway  │ ◄── Governance
       └─────────┘

Era lineages:
  Byron (standalone, legacy format)
  Shelley → Allegra → Mary → Alonzo → Babbage → Conway → Leios

Crypto packages (kes, vrf, consensus) used by: ledger/verify_*.go
Protocol packages independent of ledger (use ledger types for messages)
```

## Architecture

### Era Inheritance Pattern

Each Cardano era extends the previous one. Validation rules follow this pattern:

```go
// Shelley defines base validation rules
var UtxoValidationRules = []common.UtxoValidationRuleFunc{
    UtxoValidateTimeToLive,
    UtxoValidateInputSetEmptyUtxo,
    UtxoValidateFeeTooSmallUtxo,
    // ...
}

// Later eras delegate to earlier eras and add new rules
func UtxoValidateDelegation(tx, slot, ls, pp) error {
    return shelley.UtxoValidateDelegation(tx, slot, ls, pp)
}
```

### Key Interfaces

**LedgerState** (`ledger/common/state.go`):
```go
type LedgerState interface {
    UtxoState       // UTxO lookup
    CertState       // Stake/pool registration
    SlotState       // Slot-time conversion
    PoolState       // Pool queries
    RewardState     // Reward calculations
    GovState        // Governance/DRep queries
    NetworkId() uint
    CostModels() map[uint][]int64
}
```

**Transaction** (`ledger/common/tx.go`):
```go
type Transaction interface {
    TransactionBody
    Hash() Blake2b256
    Cbor() []byte
    IsValid() bool
    Consumed() []TransactionInput
    Produced() []Utxo
    Witnesses() TransactionWitnessSet
}
```

### Validation Rule Pattern

Each validation function has this signature:
```go
func UtxoValidate{RuleName}(
    tx common.Transaction,
    slot uint64,
    ls common.LedgerState,
    pp common.ProtocolParameters,
) error
```

Rules return `nil` on success or a specific error type on failure.

## CBOR Encoding Patterns

### Embeddable Types

The `cbor` package provides embeddable types for common patterns:

**`cbor.StructAsArray`** - Embed to encode struct as CBOR array:
```go
type ShelleyBlock struct {
    cbor.StructAsArray           // Enables CBOR array encoding
    cbor.DecodeStoreCbor         // Preserves original bytes
    BlockHeader            *ShelleyBlockHeader
    TransactionBodies      []ShelleyTransactionBody
    // ...
}
```

**`cbor.DecodeStoreCbor`** - Embed to preserve original CBOR bytes for hashing:
```go
type MyType struct {
    cbor.DecodeStoreCbor
    // fields...
}

func (m *MyType) UnmarshalCBOR(cborData []byte) error {
    type tMyType MyType
    var tmp tMyType
    if _, err := cbor.Decode(cborData, &tmp); err != nil {
        return err
    }
    *m = MyType(tmp)
    m.SetCbor(cborData)  // Store original bytes
    return nil
}

// Later, m.Cbor() returns the original bytes for hashing
```

### Common CBOR Types

- `cbor.RawMessage` - Deferred/lazy decoding
- `cbor.ByteString` - Bytestrings usable as map keys
- `cbor.Tag` / `cbor.RawTag` - CBOR semantic tags

### Encoding Gotchas

1. **Hash computation** - Always use preserved original CBOR bytes via `Cbor()`, not re-encoded data
2. **Map key ordering** - Cardano uses specific ordering rules (e.g., "shortLex" for language views)
3. **Indefinite vs definite length** - Some encodings require specific length encoding
4. **Custom UnmarshalCBOR** - When embedding `DecodeStoreCbor`, implement `UnmarshalCBOR` and call `SetCbor()`

## Testing

### Running Tests

```bash
# All tests with race detection
make test

# Specific package
go test -v ./ledger/...

# Conformance tests (314 test vectors)
go test -v ./internal/test/conformance/...
```

### Writing Tests

Use `testify` for assertions:
```go
import "github.com/stretchr/testify/assert"

func TestSomething(t *testing.T) {
    result := DoSomething()
    assert.NoError(t, err)
    assert.Equal(t, expected, result)
}
```

Use `MockLedgerState` for validation tests (`internal/test/ledger/ledger.go`):
```go
ls := &MockLedgerState{
    NetworkIdVal: 0,
    UtxoByIdFunc: func(id TransactionInput) (Utxo, error) {
        return testUtxo, nil
    },
}
```

### Conformance Tests

Located in `internal/test/conformance/`. Test vectors are from Cardano's official ledger specification. See `internal/test/conformance/README.md` for detailed structure.

## Glossary

| Term | Definition | Key Files |
|------|------------|-----------|
| **UTxO** | Unspent Transaction Output - fundamental unit of value | `ledger/common/utxo.go` |
| **TTL** | Time-to-live - slot deadline for transaction validity | `tx.TTL()` method |
| **Datum** | Data attached to script-locked UTxOs | `ledger/common/datum.go` |
| **Redeemer** | Arguments passed to Plutus scripts during execution | `ledger/common/redeemer.go` |
| **Cost Model** | Plutus execution cost parameters per version | `ledger/*/pparams.go` |
| **Era** | Cardano protocol version (Byron, Shelley, etc.) | `ledger/{era}/` |
| **Slot** | Time unit (~1 second on mainnet) | `SlotState` interface |
| **Epoch** | Collection of slots (~5 days on mainnet) | `EpochState` interface |
| **Collateral** | Ada locked to cover script failure costs | Alonzo+ transactions |
| **Reference Input** | Input read but not consumed | `tx.ReferenceInputs()` |
| **Reference Script** | Script stored in UTxO, referenced by hash | Babbage+ feature |
| **DRep** | Delegated Representative for governance | Conway era |
| **VRF** | Verifiable Random Function for leader election | `vrf/` package |
| **KES** | Key-Evolving Signature for block signing | `kes/` package |
| **CBOR** | Concise Binary Object Representation | `cbor/` package |

## Decision Tree

Use this to find where to make changes:

```text
What are you doing?
│
├─► Adding/fixing a validation rule?
│   ├─► Which era? → ledger/{era}/rules.go
│   ├─► Need new error type? → ledger/{era}/errors.go
│   ├─► Shared across eras? → ledger/common/rules.go
│   └─► Test it → internal/test/conformance/ (if vector exists)
│
├─► Working with transactions?
│   ├─► Transaction interface → ledger/common/tx.go
│   ├─► Transaction body fields → ledger/{era}/shelley.go (or era file)
│   ├─► Witnesses → ledger/common/witness.go
│   └─► Fee calculation → ledger/{era}/rules.go (MinFeeTx)
│
├─► Working with scripts?
│   ├─► Native scripts → ledger/common/script/native.go
│   ├─► Plutus scripts → ledger/common/script/plutus.go
│   ├─► Script validation → ledger/common/script/validate.go
│   └─► Script data hash → ledger/common/script_data_hash.go
│
├─► Working with certificates?
│   ├─► Certificate types → ledger/common/certs.go
│   ├─► Certificate validation → ledger/{era}/rules.go (UtxoValidateDelegation)
│   └─► Governance certs (Conway) → ledger/conway/gov.go
│
├─► Working with protocol messages?
│   ├─► Find protocol → protocol/{name}/
│   ├─► Message types → protocol/{name}/messages.go
│   ├─► Client implementation → protocol/{name}/client.go
│   └─► Server implementation → protocol/{name}/server.go
│
├─► Fixing a conformance test failure?
│   ├─► Find rule name in error output
│   ├─► Grep: grep -r "UtxoValidate{RuleName}" ledger/
│   ├─► Check spec: internal/test/cardano-blueprint/src/ledger/
│   └─► Compare with test vector JSON in conformance/testdata/
│
├─► Working with CBOR?
│   ├─► Encoding utilities → cbor/
│   ├─► Type needs array encoding → embed cbor.StructAsArray
│   ├─► Need original bytes for hash → embed cbor.DecodeStoreCbor
│   └─► Deferred decoding → use cbor.RawMessage
│
└─► Working with block verification?
    ├─► Main entry → ledger/verify_block.go
    ├─► KES verification → ledger/verify_kes.go
    ├─► VRF verification → vrf/vrf.go
    └─► Validation config → ledger/common/verify_config.go
```

## Common Tasks

### Adding a Validation Rule

1. Create the rule function in the appropriate era package (`ledger/{era}/rules.go`)
2. Add to `UtxoValidationRules` slice at the correct position
3. Define a custom error type if needed (`ledger/{era}/errors.go`)
4. Write unit tests

Example:
```go
// In ledger/conway/rules.go
func UtxoValidateNewRule(
    tx common.Transaction,
    slot uint64,
    ls common.LedgerState,
    pp common.ProtocolParameters,
) error {
    // Validation logic
    if invalid {
        return NewRuleError{Details: "..."}
    }
    return nil
}

// Add to rules slice
var UtxoValidationRules = []common.UtxoValidationRuleFunc{
    // ... existing rules
    UtxoValidateNewRule,
}
```

### Adding Support for a New Certificate Type

1. Define the certificate struct in `ledger/common/certs.go`
2. Add CBOR tags and encoding
3. Update certificate parsing switch statement
4. Add validation in appropriate era's rules

### Extending an Era

1. Create new package `ledger/{era_name}/`
2. Define types extending previous era
3. Implement `rules.go` inheriting parent rules
4. Add compatibility exports at `ledger/{era_name}.go`

## Code Quality

### Required Checks

Before submitting:
```bash
make lint      # Must pass
make test      # Must pass
make format    # Apply formatting
```

### Style Guidelines

- 80 character line limit (enforced by `golines`)
- Use `gofmt` and `goimports`
- Apache 2.0 license header on new files
- Follow existing patterns in similar files

### Linter Configuration

See `.golangci.yml` for enabled/disabled linters. Key enabled linters:
- `gosec` - Security checks
- `errorlint` - Error handling
- `exhaustive` - Exhaustive switch statements

## Important Implementation Details

### ScriptDataHash Computation

```
ScriptDataHash = Blake2b256(redeemers || datums || language_views)
```

#### Language Views Encoding (per Cardano Ledger Spec)

**PlutusV1** (double-bagged for historical compatibility):
- Tag: `serialize(serialize(0))` = `0x4100` (bytestring containing 0x00)
- Params: `serialize(indefinite_list(cost_model))` wrapped in bytestring

**PlutusV2/V3**:
- Tag: `serialize(version)` = `0x01` or `0x02`
- Params: `definite_list(cost_model)` (no bytestring wrapper)

The language views map is encoded with keys sorted by "shortLex" order (length first, then lexicographic).

### Native Script Evaluation

```go
func (n *NativeScript) Evaluate(
    slot uint64,
    validityStart, validityEnd uint64,
    keyHashes map[Blake2b224]bool,
) bool
```

### Governance Ratification

Two-phase process at epoch boundaries:
1. Enact proposals ratified in previous epoch
2. Ratify new proposals using updated state

## Error Catalog

Common validation errors, their causes, and fixes:

### Shelley Era (Base Errors)

| Error | Rule | Cause | Fix |
|-------|------|-------|-----|
| `ExpiredUtxoError` | `UtxoValidateTimeToLive` | TTL < current slot | Increase TTL value |
| `InputSetEmptyUtxoError` | `UtxoValidateInputSetEmptyUtxo` | No inputs in transaction | Add at least one input |
| `FeeTooSmallUtxoError` | `UtxoValidateFeeTooSmallUtxo` | Fee < minimum required | Use `MinFeeTx()` to calculate |
| `BadInputsUtxoError` | `UtxoValidateBadInputsUtxo` | Input not in UTxO set | Check input exists/not spent |
| `WrongNetworkError` | `UtxoValidateWrongNetwork` | Output address wrong network | Match network ID |
| `ValueNotConservedUtxoError` | `UtxoValidateValueNotConservedUtxo` | Inputs ≠ Outputs + Fee | Balance the transaction |
| `OutputTooSmallUtxoError` | `UtxoValidateOutputTooSmallUtxo` | Output below min UTxO | Increase output Ada |
| `MaxTxSizeUtxoError` | `UtxoValidateMaxTxSizeUtxo` | Transaction too large | Reduce inputs/outputs |

### Allegra Era

| Error | Rule | Cause | Fix |
|-------|------|-------|-----|
| `OutsideValidityIntervalUtxoError` | `UtxoValidateValidityInterval` | Slot outside validity range | Adjust validity interval |
| `NativeScriptFailedError` | Script evaluation | Native script conditions not met | Check required signatures/slots |

### Alonzo Era (Plutus)

| Error | Rule | Cause | Fix |
|-------|------|-------|-----|
| `ExUnitsTooBigUtxoError` | `UtxoValidateExUnitsTooBig` | Execution units exceed max | Reduce script complexity |
| `InsufficientCollateralError` | `UtxoValidateCollateral` | Collateral < required | Add more collateral |
| `CollateralContainsNonAdaError` | `UtxoValidateCollateral` | Collateral has tokens | Use Ada-only UTxO |
| `NoCollateralInputsError` | `UtxoValidateCollateral` | No collateral specified | Add collateral inputs |

### Common/Shared Errors

| Error | Rule | Cause | Fix |
|-------|------|-------|-----|
| `MissingCostModelError` | Script validation | No cost model for Plutus version | Ensure cost model in protocol params |
| `ScriptDataHashMismatchError` | `UtxoValidateScriptDataHash` | Hash doesn't match witnesses | Recalculate script data hash |
| `MissingVKeyWitnessesError` | `UtxoValidateRequiredVKeyWitnesses` | Required signature missing | Add VKey witness |
| `MalformedReferenceScriptsError` | `UtxoValidateMalformedReferenceScripts` | Invalid UPLC bytecode | Fix script encoding |

### Conway Era (Governance)

| Error | Rule | Cause | Fix |
|-------|------|-------|-----|
| `DelegateVoteToUnregisteredDRepError` | `UtxoValidateDelegation` | DRep not registered | Register DRep first |
| `StakeCredentialAlreadyRegisteredError` | `UtxoValidateDelegation` | Re-registering stake key | Deregister first or skip |
| `PlutusScriptFailedError` | Script execution | Script returned False | Debug script logic |

**Error files by era:**
- `ledger/shelley/errors.go` - Base validation errors
- `ledger/allegra/errors.go` - Validity interval, native scripts
- `ledger/alonzo/errors.go` - Plutus execution, collateral
- `ledger/babbage/errors.go` - Reference scripts, inline datums
- `ledger/conway/errors.go` - Governance, DReps
- `ledger/common/errors.go` - Shared across eras

## Common Pitfalls

1. **Reference inputs** - Resolved but never consumed from UTxO set
2. **Collateral** - Only consumed when `IsValid=false`
3. **Datum lookup** - Check witness set, inline datums, AND reference inputs
4. **Cost models** - Must exist for each Plutus version used in transaction
5. **Era delegation** - Later eras should call parent era functions, not duplicate logic
6. **CBOR preservation** - Use original bytes for hashing, not re-encoded data
7. **DecodeStoreCbor** - Must implement custom `UnmarshalCBOR` and call `SetCbor()`

## Related Documentation

- `README.md` - Feature checklist and manual testing guide
- `internal/test/conformance/README.md` - Conformance test structure
- `protocol/PROTOCOL_LIMITS.md` - Protocol buffer limits

## Dependencies

Key dependencies (see `go.mod`):
- `github.com/blinklabs-io/plutigo` - Plutus script handling
- `github.com/fxamacker/cbor/v2` - CBOR serialization
- `golang.org/x/crypto` - Cryptographic operations
- `github.com/stretchr/testify` - Test assertions
