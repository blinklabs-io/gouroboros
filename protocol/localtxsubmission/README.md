# LocalTxSubmission Protocol

The LocalTxSubmission protocol submits transactions to the local node's mempool. It is the primary method for wallets and applications to submit transactions.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `local-tx-submission` |
| Protocol ID | `6` |
| Mode | Node-to-Client |

## State Machine

```
┌──────┐    SubmitTx     ┌──────┐
│ Idle │ ───────────────►│ Busy │
└──────┘                 └──┬───┘
   ▲                        │
   │                        │ AcceptTx
   │                        │ RejectTx
   │                        │
   └────────────────────────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for transaction submission |
| **Busy** | 2 | Server | Processing submitted transaction |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `SubmitTx` | 0 | Client → Server | Submit a transaction |
| `AcceptTx` | 1 | Server → Client | Transaction accepted |
| `RejectTx` | 2 | Server → Client | Transaction rejected |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `SubmitTx` | Busy |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `AcceptTx` | Idle |
| `RejectTx` | Idle |

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Submit Timeout | 30 seconds | Time to process transaction |

## Configuration Options

```go
localtxsubmission.NewConfig(
    localtxsubmission.WithSubmitTxFunc(submitTxCallback),
    localtxsubmission.WithTimeout(30 * time.Second),
)
```

## Usage Example

```go
// Submit a transaction
err := client.SubmitTx(txEraId, txBytes)

// AcceptTx or RejectTx received via callback
```

## Transaction Format

Transactions are submitted with era identification:

```go
type MsgSubmitTxTransaction struct {
    cbor.StructAsArray
    EraId uint16   // Era identifier
    Raw   cbor.Tag // CBOR-wrapped transaction (tag 24)
}
```

## Rejection Reasons

When a transaction is rejected, the `RejectTx` message includes detailed rejection reasons from the ledger validation, such as:
- Insufficient funds
- Invalid signatures
- Script validation failure
- Expired TTL
- Missing UTxO inputs
- Fee too low

## Notes

- Accepted transactions enter the node's mempool
- Acceptance does not guarantee inclusion in a block
- Rejection provides detailed validation errors
- The protocol is stateless; each submission is independent
- Multiple transactions can be submitted sequentially
