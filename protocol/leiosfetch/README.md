# LeiosFetch Protocol

The LeiosFetch protocol retrieves Leios-specific data including blocks, block transactions, votes, and block ranges. It is part of the experimental Leios high-throughput protocol suite.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `leios-fetch` |
| Protocol ID | `19` |
| Mode | Node-to-Node |

## State Machine

```
                      BlockRequest
              ┌───────────────────────────┐
              │                           ▼
         ┌────┴───┐                  ┌───────┐
         │  Idle  │                  │ Block │
         └────┬───┘                  └───┬───┘
              │                          │
              │ BlockTxsRequest          │ Block
              │                          │
              ▼                          ▼
         ┌──────────┐               ┌──────┐
         │ BlockTxs │               │ Idle │
         └────┬─────┘               └──────┘
              │
              │ BlockTxs
              ▼
         ┌──────┐
         │ Idle │
         └──────┘

              │ VotesRequest
              ▼
         ┌───────┐
         │ Votes │
         └───┬───┘
             │
             │ Votes
             ▼
         ┌──────┐
         │ Idle │
         └──────┘

              │ BlockRangeRequest
              ▼
         ┌────────────┐◄─────────────┐
         │ BlockRange │              │
         └─────┬──────┘              │
               │                     │
               │ NextBlockAndTxsInRange
               │                     │
               └─────────────────────┘
               │
               │ LastBlockAndTxsInRange
               ▼
         ┌──────┐         Done      ┌──────┐
         │ Idle │ ─────────────────►│ Done │
         └──────┘                   └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for request |
| **Block** | 2 | Server | Processing block request |
| **BlockTxs** | 3 | Server | Processing block transactions request |
| **Votes** | 4 | Server | Processing votes request |
| **BlockRange** | 5 | Server | Streaming blocks in range |
| **Done** | 6 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `BlockRequest` | 0 | Client → Server | Request single block |
| `Block` | 1 | Server → Client | Block response |
| `BlockTxsRequest` | 2 | Client → Server | Request block transactions |
| `BlockTxs` | 3 | Server → Client | Block transactions response |
| `VotesRequest` | 4 | Client → Server | Request votes |
| `Votes` | 5 | Server → Client | Votes response |
| `BlockRangeRequest` | 6 | Client → Server | Request range of blocks |
| `LastBlockAndTxsInRange` | 7 | Server → Client | Last block in range |
| `NextBlockAndTxsInRange` | 8 | Server → Client | Next block in range |
| `Done` | 9 | Client → Server | Terminate protocol |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `BlockRequest` | Block |
| `BlockTxsRequest` | BlockTxs |
| `VotesRequest` | Votes |
| `BlockRangeRequest` | BlockRange |
| `Done` | Done |

### From Block (Server Agency)
| Message | New State |
|---------|-----------|
| `Block` | Idle |

### From BlockTxs (Server Agency)
| Message | New State |
|---------|-----------|
| `BlockTxs` | Idle |

### From Votes (Server Agency)
| Message | New State |
|---------|-----------|
| `Votes` | Idle |

### From BlockRange (Server Agency)
| Message | New State |
|---------|-----------|
| `NextBlockAndTxsInRange` | BlockRange |
| `LastBlockAndTxsInRange` | Idle |

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Default Timeout | 5 seconds | General request timeout |

## Configuration Options

```go
leiosfetch.NewConfig(
    leiosfetch.WithBlockRequestFunc(blockRequestCallback),
    leiosfetch.WithBlockTxsRequestFunc(blockTxsRequestCallback),
    leiosfetch.WithVotesRequestFunc(votesRequestCallback),
    leiosfetch.WithBlockRangeRequestFunc(blockRangeRequestCallback),
    leiosfetch.WithTimeout(5 * time.Second),
)
```

## Usage Example

```go
// Request a single block
block, err := client.BlockRequest(slot, blockId)

// Request block transactions
txs, err := client.BlockTxsRequest(slot, blockId, txFilter)

// Request votes
votes, err := client.VotesRequest(voteIds)

// Request a range of blocks (blocks until all received)
blocks, err := client.BlockRangeRequest(startPoint, endPoint)
for _, block := range blocks {
    // Process each block
}
```

## Notes

- Part of the experimental Leios protocol suite
- Supports both single-item and streaming requests
- BlockRange allows efficient bulk retrieval
- Used in conjunction with LeiosNotify for announcements
