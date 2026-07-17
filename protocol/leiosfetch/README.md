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
              │ BlockTxsRequest          │ Block / NoBlock
              │                          │
              ▼                          ▼
         ┌──────────┐               ┌──────┐
         │ BlockTxs │               │ Idle │
         └────┬─────┘               └──────┘
              │
              │ BlockTxs / NoBlockTxs
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
| `NoBlock` | 10 | Server → Client | Requested block not available |
| `NoBlockTxs` | 11 | Server → Client | Requested block transactions not available |

> **Note:** Type IDs `10` and `11` are placeholders pending confirmation against
> the Leios protocol spec (CIP-0164 or equivalent), consistent with the other
> IDs in this experimental protocol.

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
| `NoBlock` | Idle |

### From BlockTxs (Server Agency)
| Message | New State |
|---------|-----------|
| `BlockTxs` | Idle |
| `NoBlockTxs` | Idle |

### From Votes (Server Agency)
| Message | New State |
|---------|-----------|
| `Votes` | Idle |

### From BlockRange (Server Agency)
| Message | New State |
|---------|-----------|
| `NextBlockAndTxsInRange` | BlockRange |
| `LastBlockAndTxsInRange` | Idle |

`VotesRequest` identifies each requested vote by `(SlotNo, VoterId)`, where
`VoterId` is the voter's index in the epoch's stake-based committee.
`Votes` keeps the wire payload as raw CBOR and provides typed helpers for
validated `common.LeiosVote` values.

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

## Not-found responses

A server that cannot serve a requested endorser block (for example, an
already-synced relay whose in-memory cache has expired) responds with `NoBlock`
or `NoBlockTxs` instead of returning an error. This lets the server decline
gracefully rather than triggering a protocol violation that tears down the
whole node-to-node connection.

- A `BlockRequestFunc` / `BlockTxsRequestFunc` callback signals not-found by
  returning `ErrBlockNotFound` / `ErrBlockTxsNotFound` (directly or wrapped
  with `fmt.Errorf("...: %w", ...)`). Any other error is still treated as a
  protocol violation.
- The client's `BlockRequest` / `BlockTxsRequest` returns the matching sentinel
  error to the caller, who can distinguish "not available" from a real protocol
  error with `errors.Is`.

## Notes

- Part of the experimental Leios protocol suite
- Supports both single-item and streaming requests
- BlockRange allows efficient bulk retrieval
- Used in conjunction with LeiosNotify for announcements
