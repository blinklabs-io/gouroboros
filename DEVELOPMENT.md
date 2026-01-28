# Development Guide

This guide covers everything you need to contribute to gouroboros.

## Quick Start

```bash
# Clone and enter the repository
git clone https://github.com/blinklabs-io/gouroboros.git
cd gouroboros

# Install dependencies
go mod download

# Run tests
make test

# Run linter
make lint
```

**Requirements:**
- Go 1.24+
- [golangci-lint](https://golangci-lint.run/docs/welcome/install/local/) for linting
- [golines](https://github.com/segmentio/golines) for line length formatting (optional)

## Project Structure

```text
gouroboros/
├── ledger/              # Blockchain ledger types and validation
│   ├── common/          # Shared types and interfaces (start here)
│   ├── byron/           # Byron era
│   ├── shelley/         # Shelley era (base validation)
│   ├── allegra/         # Allegra era
│   ├── mary/            # Mary era (multi-asset)
│   ├── alonzo/          # Alonzo era (Plutus scripts)
│   ├── babbage/         # Babbage era (reference scripts)
│   ├── conway/          # Conway era (governance)
│   └── leios/           # Leios era (experimental)
├── protocol/            # Ouroboros network mini-protocols
│   ├── chainsync/       # Chain synchronization
│   ├── blockfetch/      # Block retrieval
│   ├── txsubmission/    # Transaction submission
│   ├── localstatequery/ # Local node queries
│   └── .../             # Other protocols
├── cbor/                # CBOR encoding utilities
├── kes/                 # Key-Evolving Signatures
├── vrf/                 # Verifiable Random Functions
├── consensus/           # Consensus primitives
├── connection/          # Network connection management
├── muxer/               # Protocol multiplexing
└── internal/test/       # Test utilities and conformance tests
```

## Development Workflow

### Before You Start

1. **Find the right package** - Use the structure above to locate where your change belongs
2. **Read existing code** - Check similar implementations in the same package
3. **Understand era inheritance** - Later eras delegate to earlier ones; don't duplicate logic

### Making Changes

```bash
# Create a branch
git checkout -b feature/your-feature

# Make your changes...

# Format code
make format

# Run tests
make test

# Run linter
make lint

# Commit (we use conventional commits)
git commit -m "feat(ledger): add new validation rule for X"
```

### Commit Message Format

We use [Conventional Commits](https://www.conventionalcommits.org/):

```text
type(scope): description

[optional body]
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

Scopes: `ledger`, `protocol`, `cbor`, `kes`, `vrf`, `consensus`

## Code Style

### Formatting

- **Line length:** 80 characters (enforced by `golines`)
- **Formatting:** `gofmt` and `goimports`
- **Run:** `make format` before committing

### Testing

Use `testify` for assertions:

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestSomething(t *testing.T) {
    result, err := DoSomething()
    require.NoError(t, err)           // Fatal: stops test on failure
    assert.Equal(t, expected, result) // Non-fatal: continues on failure
}
```

### Error Handling

- Return specific error types, not generic errors
- Error types live in `{package}/errors.go`
- Use `errors.Is()` and `errors.As()` for checking

### CBOR Patterns

When creating types that need CBOR encoding:

```go
type MyBlock struct {
    cbor.StructAsArray   // Encode as array, not map
    cbor.DecodeStoreCbor // Preserve original bytes for hashing
    Header  *BlockHeader
    Body    *BlockBody
}

// IMPORTANT: If using DecodeStoreCbor, implement UnmarshalCBOR
func (m *MyBlock) UnmarshalCBOR(data []byte) error {
    type tMyBlock MyBlock
    var tmp tMyBlock
    if _, err := cbor.Decode(data, &tmp); err != nil {
        return err
    }
    *m = MyBlock(tmp)
    m.SetCbor(data)  // Store original bytes!
    return nil
}
```

## Common Tasks

### Adding a Validation Rule

1. **Create the rule** in `ledger/{era}/rules.go`:

```go
func UtxoValidateMyRule(
    tx common.Transaction,
    slot uint64,
    ls common.LedgerState,
    pp common.ProtocolParameters,
) error {
    // Validation logic
    if invalid {
        return MyRuleError{Details: "..."}
    }
    return nil
}
```

2. **Add error type** in `ledger/{era}/errors.go`:

```go
type MyRuleError struct {
    Details string
}

func (e MyRuleError) Error() string {
    return fmt.Sprintf("my rule failed: %s", e.Details)
}
```

3. **Register the rule** in the `UtxoValidationRules` slice:

```go
var UtxoValidationRules = []common.UtxoValidationRuleFunc{
    // ... existing rules
    UtxoValidateMyRule,
}
```

4. **Write tests** using `MockLedgerState`:

```go
func TestUtxoValidateMyRule(t *testing.T) {
    ls := &test.MockLedgerState{
        NetworkIdVal: 0,
        UtxoByIdFunc: func(id common.TransactionInput) (common.Utxo, error) {
            return testUtxo, nil
        },
    }

    err := UtxoValidateMyRule(tx, slot, ls, pp)
    assert.NoError(t, err)
}
```

### Adding a Protocol Message

1. **Define message type** in `protocol/{name}/messages.go`:

```go
const MessageTypeMyMsg = 7

type MsgMyMsg struct {
    protocol.MessageBase
    Field1 string
    Field2 uint64
}

func NewMsgMyMsg(field1 string, field2 uint64) *MsgMyMsg {
    return &MsgMyMsg{
        MessageBase: protocol.MessageBase{
            MessageType: MessageTypeMyMsg,
        },
        Field1: field1,
        Field2: field2,
    }
}
```

2. **Register in message parsing** (usually a switch statement in the package)

3. **Add client/server methods** as needed

### Working with Eras

Each era extends the previous. When adding era-specific logic:

```go
// In ledger/conway/rules.go - delegate to parent era
func UtxoValidateSomething(tx, slot, ls, pp) error {
    // First, run the parent era's validation
    if err := babbage.UtxoValidateSomething(tx, slot, ls, pp); err != nil {
        return err
    }
    // Then add Conway-specific checks
    // ...
    return nil
}
```

## Testing

### Test Commands

```bash
# All tests with race detection
make test

# Specific package
go test -v ./ledger/shelley/...

# Conformance tests (314 test vectors)
go test -v ./internal/test/conformance/...

# Single test
go test -v -run TestSpecificName ./ledger/...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Conformance Tests

Located in `internal/test/conformance/`. These test against official Cardano ledger specification vectors.

```bash
# Run all conformance tests
go test -v ./internal/test/conformance/...

# Count passing/failing
go test -v ./internal/test/conformance/... 2>&1 | grep -E "--- (PASS|FAIL):" | sort | uniq -c
```

### Mock Ledger State

Use `internal/test/ledger/ledger.go` for mocking:

```go
import test "github.com/blinklabs-io/gouroboros/internal/test/ledger"

ls := &test.MockLedgerState{
    NetworkIdVal: 0,
    UtxoByIdFunc: func(id common.TransactionInput) (common.Utxo, error) {
        // Return test data or error
    },
    IsStakeRegisteredFunc: func(cred []byte) (bool, error) {
        return true, nil
    },
}
```

## Debugging

### Common Issues

**CBOR decode errors:**
- Check field order matches CBOR structure
- Verify CBOR tags are correct
- Use `cbor.Diagnose()` to inspect raw bytes

**Hash mismatches:**
- Ensure `SetCbor()` is called in `UnmarshalCBOR`
- Use `Cbor()` method, not re-encoded data
- Check for indefinite vs definite length encoding issues

**Validation failures:**
- Check which rule failed (error type indicates the rule)
- Verify mock state returns expected values
- Compare with official test vectors if available

### Useful Debug Commands

```bash
# Decode CBOR hex to diagnostic notation
echo "85..." | xxd -r -p | cbor-diag

# Run single test with verbose output
go test -v -run TestName ./package/...

# Check for goroutine leaks
go test -v -race ./...
```

## Architecture Notes

### Era Lineages

Cardano has two separate era lineages:

```text
Byron (standalone, legacy)

Shelley → Allegra → Mary → Alonzo → Babbage → Conway → Leios
```

**Byron** is the original Cardano implementation with its own block/transaction format.

**Shelley and later** are a fresh start with a new architecture. Each post-Shelley era:
- Extends types from the previous era
- Delegates validation to parent era, then adds new rules
- May add new transaction fields, certificate types, etc.

### Key Interfaces

**LedgerState** (`ledger/common/state.go`):
```go
type LedgerState interface {
    UtxoState       // UTxO lookup
    CertState       // Stake/pool registration
    SlotState       // Slot-time conversion
    PoolState       // Pool queries
    RewardState     // Reward calculations
    GovState        // Governance queries
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

### Protocol Structure

Each mini-protocol package follows this pattern:
- `{protocol}.go` - Protocol definition, state machine
- `client.go` - Client implementation
- `server.go` - Server implementation
- `messages.go` - Message types

## Getting Help

- **Discord:** [Blink Labs Discord](https://discord.gg/5fPRZnX4qW)
- **Issues:** [GitHub Issues](https://github.com/blinklabs-io/gouroboros/issues)
- **GoDoc:** [pkg.go.dev](https://pkg.go.dev/github.com/blinklabs-io/gouroboros)

## License

Apache 2.0 - Include the license header in new files:

```go
// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// ...
```
