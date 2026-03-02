# LocalTxMonitor Protocol

The LocalTxMonitor protocol monitors the local node's mempool. It allows clients to inspect pending transactions and mempool statistics.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `local-tx-monitor` |
| Protocol ID | `9` |
| Mode | Node-to-Client |

## State Machine

```
┌──────┐      Acquire      ┌───────────┐
│ Idle │ ─────────────────►│ Acquiring │
└──┬───┘                   └─────┬─────┘
   │                             │
   │ Done                        │ Acquired
   │                             │
   ▼                             ▼
┌──────┐                   ┌──────────┐◄───────────┐
│ Done │◄──────────────────│ Acquired │            │
└──────┘     Release       └────┬─────┘            │
                                │                  │
                   ┌────────────┼────────────┐     │
                   │            │            │     │
         HasTx     │   NextTx   │  GetSizes  │     │
                   │            │            │     │
                   ▼            ▼            ▼     │
              ┌────────────────────────────────┐   │
              │             Busy               │   │
              └────────────────┬───────────────┘   │
                               │                   │
                    ReplyHasTx │ ReplyNextTx       │
                    ReplyGetSizes                  │
                               │                   │
                               └───────────────────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | No mempool snapshot acquired |
| **Acquiring** | 2 | Server | Acquiring mempool snapshot |
| **Acquired** | 3 | Client | Snapshot acquired, ready for queries |
| **Busy** | 4 | Server | Processing query |
| **Done** | 5 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `Done` | 0 | Client → Server | Terminate protocol |
| `Acquire` | 1 | Client → Server | Acquire mempool snapshot |
| `Acquired` | 2 | Server → Client | Snapshot acquired with slot number |
| `Release` | 3 | Client → Server | Release snapshot |
| `NextTx` | 5 | Client → Server | Get next transaction |
| `ReplyNextTx` | 6 | Server → Client | Next transaction or empty |
| `HasTx` | 7 | Client → Server | Check if tx is in mempool |
| `ReplyHasTx` | 8 | Server → Client | HasTx result (bool) |
| `GetSizes` | 9 | Client → Server | Get mempool sizes |
| `ReplyGetSizes` | 10 | Server → Client | Mempool size information |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `Acquire` | Acquiring |
| `Done` | Done |

### From Acquiring (Server Agency)
| Message | New State |
|---------|-----------|
| `Acquired` | Acquired |

### From Acquired (Client Agency)
| Message | New State |
|---------|-----------|
| `Acquire` | Acquiring |
| `Release` | Idle |
| `HasTx` | Busy |
| `NextTx` | Busy |
| `GetSizes` | Busy |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `ReplyHasTx` | Acquired |
| `ReplyNextTx` | Acquired |
| `ReplyGetSizes` | Acquired |

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Acquire Timeout | 5 seconds | Time to acquire mempool snapshot |
| Query Timeout | 30 seconds | Time to execute query |

## Configuration Options

```go
localtxmonitor.NewConfig(
    localtxmonitor.WithGetMempoolFunc(getMempoolCallback),
    localtxmonitor.WithAcquireTimeout(5 * time.Second),
    localtxmonitor.WithQueryTimeout(30 * time.Second),
)
```

## Usage Example

```go
// Acquire mempool snapshot
err := client.Acquire()

// Check if specific transaction is in mempool
hasTx, err := client.HasTx(txId)

// Iterate through mempool transactions
for {
    tx, err := client.NextTx()
    if tx == nil {
        break // No more transactions
    }
    // Process transaction
}

// Get mempool statistics
capacity, size, numTxs, err := client.GetSizes()

// Release snapshot
client.Release()
```

## Mempool Size Information

The `GetSizes` query returns:
- **Capacity**: Maximum mempool capacity in bytes
- **Size**: Current mempool size in bytes
- **NumTxs**: Number of transactions in mempool

## Notes

- Mempool snapshot is consistent during queries
- Acquire again to get updated mempool state
- NextTx iterates through all transactions in snapshot
- HasTx checks for specific transaction by ID
- Release snapshot when done to free resources
