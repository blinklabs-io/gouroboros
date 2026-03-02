# Gouroboros Architecture

Gouroboros is a Go implementation of the Cardano blockchain protocol, providing ledger validation, network protocol support, consensus primitives, and cryptographic operations.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                     Ouroboros (Entry Point)                     │
│              connection.go, connection_options.go               │
└────────────┬────────────────────────────────────────────────────┘
             │
      ┌──────┴──────┐
      │             │
      ▼             ▼
  ┌────────┐   ┌──────────┐
  │ Muxer  │   │ Protocol │  (Protocol Base Layer)
  └────────┘   └──────────┘
      │             │
      └──────┬──────┴──────────────────┬─────────────────┐
             │                          │                 │
        ┌────▼─────┐ ┌──────────┐ ┌──────────────┐ ┌─────▼────────┐
        │ChainSync │ │BlockFetch│ │TxSubmission  │ │LocalStateQry │
        └──────────┘ └──────────┘ └──────────────┘ └──────────────┘
             │
             ▼
        ┌─────────────────────────────────────┐
        │        Ledger Layer                 │
        │  (Block and Transaction Validation) │
        └────────┬────────────────────────────┘
                 │
        ┌────────▼─────────────────────────┐
        │  Ledger/Common (Shared Types)    │
        │  ├─ state.go (Interfaces)        │
        │  ├─ tx.go (Transaction API)      │
        │  ├─ rules.go (Validation Rules)  │
        │  └─ verify_config.go (Config)    │
        └────────┬─────────────────────────┘
                 │
        ┌────────┴─────────────────────────────────┐
        │  Era-Specific Implementations            │
        │  Byron → Shelley → Allegra → Mary →      │
        │  Alonzo → Babbage → Conway → Leios       │
        └───────────────────────────────────────────┘
             │
    ┌────────┴──────────────┬──────────────┬────────────┐
    │                       │              │            │
    ▼                       ▼              ▼            ▼
┌────────┐             ┌────────┐    ┌────────────┐ ┌──────────┐
│ CBOR   │             │ Crypto │    │ Consensus  │ │ Protocol │
│Encode/ │             │ KES/VRF│    │  Praos     │ │ Messages │
│Decode  │             │        │    │            │ │          │
└────────┘             └────────┘    └────────────┘ └──────────┘
```

## Package Structure

| Package | Purpose |
|---------|---------|
| `/` (root) | Connection API and entry points |
| `ledger/` | Ledger types and validation by era |
| `ledger/common/` | Shared types, interfaces, utilities |
| `ledger/{era}/` | Era-specific implementations |
| `protocol/` | Network mini-protocol implementations |
| `protocol/common/` | Shared protocol utilities |
| `pipeline/` | Block processing pipeline (decode, validate, apply stages) |
| `consensus/` | Ouroboros Praos consensus primitives |
| `consensus/byron/` | Byron-era consensus validation |
| `consensus/genesis/` | Genesis consensus handling |
| `kes/` | Key-Evolving Signatures (MMM scheme) |
| `vrf/` | Verifiable Random Functions |
| `cbor/` | CBOR encoding/decoding utilities |
| `muxer/` | Protocol multiplexing |
| `connection/` | ConnectionId type definition |
| `internal/test/` | Test utilities and conformance tests |

## Ledger Layer

### Core Interfaces (ledger/common/state.go)

```go
type LedgerState interface {
    UtxoState           // Query UTxOs by transaction input
    CertState           // Stake credential registration
    SlotState           // Slot/time conversion
    PoolState           // Pool registration status
    RewardState         // Reward calculations
    GovState            // Governance (Conway+)
    NetworkId() uint
    CostModels() map[PlutusLanguage]CostModel
}
```

### Transaction Interface (ledger/common/tx.go)

Transaction embeds TransactionBody, inheriting all of its methods directly:

```go
type Transaction interface {
    TransactionBody
    Type() int
    Cbor() []byte
    Hash() Blake2b256
    LeiosHash() Blake2b256
    Metadata() TransactionMetadatum
    AuxiliaryData() AuxiliaryData
    IsValid() bool
    Consumed() []TransactionInput
    Produced() []Utxo
    Witnesses() TransactionWitnessSet
}

type TransactionBody interface {
    Cbor() []byte
    Fee() *big.Int
    Id() Blake2b256
    Inputs() []TransactionInput
    Outputs() []TransactionOutput
    TTL() uint64
    ValidityIntervalStart() uint64
    ReferenceInputs() []TransactionInput
    Collateral() []TransactionInput
    CollateralReturn() TransactionOutput
    TotalCollateral() *big.Int
    Certificates() []Certificate
    Withdrawals() map[*Address]*big.Int
    AuxDataHash() *Blake2b256
    RequiredSigners() []Blake2b224
    AssetMint() *MultiAsset[MultiAssetTypeMint]
    ScriptDataHash() *Blake2b256
    VotingProcedures() VotingProcedures
    ProposalProcedures() []ProposalProcedure
    CurrentTreasuryValue() *big.Int
    Donation() *big.Int
    ProtocolParameterUpdates() (uint64, map[Blake2b224]ProtocolParameterUpdate)
    Utxorpc() (*utxorpc.Tx, error)
}
```

### Era Hierarchy

Each era builds on the previous, delegating unchanged validation rules:

```
Byron (genesis)
  └─ Shelley (base validation, staking)
      └─ Allegra (time locks)
          └─ Mary (multi-asset)
              └─ Alonzo (Plutus scripts)
                  └─ Babbage (reference scripts)
                      └─ Conway (governance)
                          └─ Leios (experimental)
```

### Validation Pattern

Each era defines validation rules that delegate to earlier eras for unchanged logic:

```go
// shelley/rules.go - defines base rules
var UtxoValidationRules = []common.UtxoValidationRuleFunc{
    UtxoValidateMetadata,
    UtxoValidateRequiredVKeyWitnesses,
    UtxoValidateSignatures,
    UtxoValidateTimeToLive,
    // ...
}

// allegra/rules.go - delegates unchanged rules
func UtxoValidateTimeToLive(tx, slot, ls, pp) error {
    return shelley.UtxoValidateTimeToLive(tx, slot, ls, pp)
}
```

### Era-Specific Features

| Era | Key Features |
|-----|--------------|
| Shelley | Staking, delegation, native scripts, VRF/KES |
| Allegra | Time-lock scripts (InvalidBefore/InvalidHereafter) |
| Mary | Multi-asset support (native tokens) |
| Alonzo | Plutus smart contracts, redeemers, datums |
| Babbage | Reference scripts, inline datums |
| Conway | On-chain governance, DReps, proposals |
| Leios | Experimental (input blocks, endorser blocks) |

## Protocol Layer (protocol/)

Implements Ouroboros mini-protocols for network communication.

### Mini-Protocols

| Protocol | Purpose |
|----------|---------|
| Handshake | Version negotiation |
| ChainSync | Blockchain synchronization |
| BlockFetch | Block retrieval |
| TxSubmission | Transaction submission to network |
| LocalTxSubmission | Local node transaction submission |
| LocalStateQuery | Query node state |
| LocalTxMonitor | Monitor local mempool |
| PeerSharing | Peer discovery |
| KeepAlive | Connection health |
| MessageSubmission | Off-chain message submission |
| LocalMessageSubmission | Local message submission |
| LocalMessageNotification | Local message notifications |
| LeiosFetch | Leios block fetching |
| LeiosNotify | Leios block notifications |

### Protocol Structure

Each protocol follows the same pattern:

```
protocol/{name}/
├── protocol.go   # Protocol ID, state machine
├── messages.go   # Message types with CBOR encoding
├── client.go     # Client implementation
└── server.go     # Server implementation
```

## Cryptography

### VRF (vrf/)

Spec: ECVRF-ED25519-SHA512-Elligator2

Used for leader election in Ouroboros Praos:

```go
func Prove(secretKey []byte, alpha []byte) ([]byte, []byte, error)
func Verify(vrfKey, proof, expectedOutput, msg []byte) (bool, error)
```

### KES (kes/)

Scheme: MMM sum composition with Ed25519

Forward-secure signatures (compromising current key doesn't affect past signatures):

```go
const CardanoKesDepth = 6  // 64 time periods
const CardanoKesSignatureSize = 448

func VerifySignedKES(vkey []byte, period uint64, msg []byte, sig []byte) bool
```

## Consensus (consensus/)

Implements Ouroboros Praos consensus primitives.

### Key Components

| File | Purpose |
|------|---------|
| consensus.go | Core interfaces (VRFSigner, KESSigner, ConsensusHeader, etc.) |
| block.go | Block construction (BlockBuilder, OperationalCert, Header) |
| leader.go | Leader eligibility checking |
| selection.go | Block producer selection |
| threshold.go | Leadership threshold calculation |
| validate.go | Block validation |
| byron/ | Byron-era consensus validation |
| genesis/ | Genesis consensus handling |

### Interfaces

```go
type VRFSigner interface {
    Prove(input []byte) (proof, output []byte, err error)
    PublicKey() []byte
}

type KESSigner interface {
    Sign(message []byte) (signature []byte, err error)
    PublicKey() []byte
    Period() uint64
}

type ConsensusHeader interface {
    Slot() uint64
    BlockNumber() uint64
    PrevHash() []byte
    IssuerVKey() []byte
    VRFVKey() []byte
    VRFProof() []byte
    VRFOutput() []byte
    KESSignature() []byte
    KESPeriod() uint64
    Era() uint8
}
```

## CBOR Support (cbor/)

Cardano-specific CBOR encoding with byte preservation for hashing:

```go
// Preserve original bytes for hash verification
type DecodeStoreCbor struct {
    cborData []byte
}

// Encode struct as CBOR array (not map)
type StructAsArray struct{}

// Deferred decoding (alias for fxamacker/cbor/v2.RawMessage)
type RawMessage = _cbor.RawMessage
```

### Critical Pattern: CBOR Byte Preservation

Blocks and transactions must preserve original CBOR bytes for hash verification:

```go
type Block struct {
    cbor.DecodeStoreCbor
    cbor.StructAsArray
    Header *BlockHeader
    Body   interface{}
}

func (b *Block) Hash() Blake2b256 {
    return Blake2b256Hash(b.Cbor())  // Uses preserved bytes
}
```

## API Usage Examples

### Block Validation

```go
block, err := ledger.NewBlockFromCbor(blockType, blockCbor)
err = ledger.VerifyBlock(block, slot, ledgerState, protocolParams)
```

### Transaction Validation

```go
err := common.VerifyTransaction(
    tx,
    slot,
    ledgerState,
    protocolParams,
    shelley.UtxoValidationRules,
)
```

### Network Connection

```go
conn, err := NewConnection(ctx, "localhost:3001")

// Chain sync
cs := conn.ChainSync()
tip, err := cs.GetCurrentTip()

// Block fetch
bf := conn.BlockFetch()
block, err := bf.GetBlocks(hashes)

// Transaction submission
ts := conn.TxSubmission()
err = ts.SubmitTx(txCbor)
```

## Data Flow: Block Processing

```
1. Network receives block bytes
2. DetermineBlockType(header) -> BlockType
3. NewBlockFromCbor(type, bytes) -> Block
4. Extract components:
   |- BlockHeader (VRF, KES, slot)
   |- Transaction bodies
   |- Witness sets
   |- Metadata
5. VerifyBlock() validates:
   |- Body hash matches header
   |- Consensus rules (VRF, KES)
   |- Ledger rules per era
6. For each transaction:
   |- Verify signatures
   |- Check fees, TTL
   |- Validate UTxO consumption
   |- Execute Plutus scripts
   |- Process certificates
7. Update ledger state
```

## Block Pipeline (pipeline/)

The `pipeline` package provides a staged block processing pipeline with worker pools:

```
┌─────────────┐     ┌───────────────┐     ┌─────────────┐
│ DecodeStage │ --> │ ValidateStage │ --> │ ApplyStage  │
│  (workers)  │     │   (workers)   │     │  (workers)  │
└─────────────┘     └───────────────┘     └─────────────┘
```

- **DecodeStage**: CBOR decoding of raw block bytes (parallelizable)
- **ValidateStage**: Block validation (VRF, KES, ledger rules)
- **ApplyStage**: State updates (must preserve ordering)

Usage:
```go
pipeline := pipeline.NewBlockPipeline(
    pipeline.WithDecodeWorkers(4),
    pipeline.WithValidateWorkers(2),
)
pipeline.Start(ctx)
pipeline.Submit(blockItem)
for result := range pipeline.Results() { ... }
```

## Testing

### Organization

```
internal/test/
|- conformance/         # Amaru test vectors (314 rules)
|- cardano-blueprint/   # Cardano specification reference

ledger/**/*_test.go     # Unit tests
protocol/**/*_test.go
consensus/**/*_test.go
```

Mock ledger state for testing is provided by an external package:

```go
import ledgertest "github.com/blinklabs-io/ouroboros-mock/ledger"

ls := ledgertest.NewLedgerStateBuilder().
    WithNetworkId(0).
    WithUtxoByIdFunc(func(id common.TransactionInput) (common.Utxo, error) {
        return testUtxo, nil
    }).
    Build()
```

## Key Dependencies

| Package | Purpose |
|---------|---------|
| fxamacker/cbor/v2 | CBOR encoding |
| blinklabs-io/plutigo | Plutus scripts |
| blinklabs-io/ouroboros-mock | Mock ledger state for testing |
| utxorpc/go-codegen | UTxO RPC protocol buffer types |
| golang.org/x/crypto | Cryptographic primitives |
| filippo.io/edwards25519 | Ed25519 implementation |
| jinzhu/copier | Deep copy utilities |
| btcsuite/btcd/btcutil | Bitcoin-style base58 encoding |
| google.golang.org/protobuf | Protocol buffer runtime |
| stretchr/testify | Test assertions |
| go.uber.org/goleak | Goroutine leak detection in tests |

## Design Principles

1. Era Delegation: Later eras delegate unchanged rules to earlier eras, minimizing duplication.

2. Interface-Based Validation: Rules operate on interfaces, enabling easy mocking and cross-era compatibility.

3. CBOR Byte Preservation: Original bytes preserved for cryptographic hash verification.

4. Protocol State Machines: Each mini-protocol implements strict state transitions.

5. Streaming Support: BlockTransactionOffsets enables efficient large block handling without full deserialization.
