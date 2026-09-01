# TxSubmission Protocol

The TxSubmission protocol propagates transactions between nodes. It uses a pull-based model where the server requests transaction IDs and bodies from the client.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `tx-submission` |
| Protocol ID | `4` |
| Mode | Node-to-Node |

## State Machine

```text
┌──────┐    Init      ┌──────┐
│ Init │ ────────────►│ Idle │◄───────────────────┐
└──────┘              └──┬───┘                    │
                         │                        │
         RequestTxIds    │                        │ ReplyTxIds
         (blocking)      │                        │ ReplyTxs
                         ▼                        │
                  ┌─────────────────┐             │
                  │ TxIdsBlocking   │─────────────┤
                  └────────┬────────┘             │
                           │                      │
                           │ Done                 │
                           ▼                      │
                       ┌──────┐                   │
                       │ Done │                   │
                       └──────┘                   │
                                                  │
         RequestTxIds    │                        │
         (non-blocking)  │                        │
                         ▼                        │
                  ┌─────────────────┐             │
                  │ TxIdsNonblocking│─────────────┤
                  └─────────────────┘             │
                                                  │
         RequestTxs      │                        │
                         ▼                        │
                  ┌──────────┐                    │
                  │   Txs    │────────────────────┘
                  └──────────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Init** | 1 | Client | Initial state before protocol activation |
| **Idle** | 2 | Server | Waiting for server to request transactions |
| **TxIdsBlocking** | 3 | Client | Client must provide tx IDs (blocking) |
| **TxIdsNonblocking** | 4 | Client | Client may provide tx IDs (non-blocking) |
| **Txs** | 5 | Client | Client must provide transaction bodies |
| **Done** | 6 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `Init` | 6 | Client → Server | Initialize protocol |
| `RequestTxIds` | 0 | Server → Client | Request transaction IDs |
| `ReplyTxIds` | 1 | Client → Server | Provide transaction IDs and sizes |
| `RequestTxs` | 2 | Server → Client | Request full transactions |
| `ReplyTxs` | 3 | Client → Server | Provide transaction bodies |
| `Done` | 4 | Client → Server | Terminate protocol |

## State Transitions

### From Init (Client Agency)
| Message | New State |
|---------|-----------|
| `Init` | Idle |

### From Idle (Server Agency)
| Message | New State | Condition |
|---------|-----------|-----------|
| `RequestTxIds` | TxIdsBlocking | Blocking = true |
| `RequestTxIds` | TxIdsNonblocking | Blocking = false |
| `RequestTxs` | Txs | |

### From TxIdsBlocking (Client Agency)
| Message | New State |
|---------|-----------|
| `ReplyTxIds` | Idle |
| `Done` | Done |

### From TxIdsNonblocking (Client Agency)
| Message | New State |
|---------|-----------|
| `ReplyTxIds` | Idle |

### From Txs (Client Agency)
| Message | New State |
|---------|-----------|
| `ReplyTxs` | Idle |

## Timeouts (per spec Table 3.11)

| State | Timeout | Description |
|-------|---------|-------------|
| Init | none | No timeout per spec |
| Idle | none | No timeout per spec |
| TxIdsBlocking | none | No timeout per spec (blocking waits indefinitely) |
| TxIdsNonblocking | 10 seconds | Client must reply with tx IDs |
| Txs | 10 seconds | Client must reply with transactions |

## Limits

| Limit | Value | Description |
|-------|-------|-------------|
| Max Request Count | 65535 | Max transactions per request (uint16) |
| Max Ack Count | 65535 | Max transaction acknowledgments (uint16) |
| Default Request Limit | 1000 | Default request limit |
| Default Ack Limit | 1000 | Default ack limit |

## Request Parameters

The `RequestTxIds` message includes:
- **Blocking**: Whether to block waiting for transactions
- **Ack**: Number of previously received transactions to acknowledge
- **Req**: Number of new transaction IDs requested

## Configuration Options

```go
txsubmission.NewConfig(
    txsubmission.WithRequestTxIdsFunc(requestTxIdsCallback),
    txsubmission.WithRequestTxsFunc(requestTxsCallback),
    txsubmission.WithInitFunc(initCallback),
    txsubmission.WithDoneFunc(doneCallback),
    txsubmission.WithIdleTimeout(0), // no timeout per spec
)
```

## Usage Example

```go
// Server requests transaction IDs (blocking mode, request 10 IDs)
txIds, err := server.RequestTxIds(true, 10)

// Client provides IDs and sizes via RequestTxIdsFunc callback
cfg := txsubmission.NewConfig(
    txsubmission.WithRequestTxIdsFunc(func(ctx CallbackContext, blocking bool, ack, req uint16) ([]TxIdAndSize, error) {
        return []TxIdAndSize{
            {TxId: txId1, Size: 256},
            {TxId: txId2, Size: 512},
        }, nil
    }),
)

// Server requests full transactions
txs, err := server.RequestTxs([]TxId{txId1, txId2})

// Client provides transaction bodies via RequestTxsFunc callback
```

## Transaction ID Format

```go
type TxId struct {
    EraId uint16   // Era identifier
    TxId  [32]byte // Transaction hash
}

type TxIdAndSize struct {
    TxId TxId
    Size uint32 // Transaction size in bytes
}
```

## Notes

- Uses pull-based model (server requests, client provides)
- Blocking mode waits for new transactions
- Non-blocking mode returns immediately with available transactions
- Transaction sizes are provided to help with mempool management
- The Done message can only be sent from TxIdsBlocking state
