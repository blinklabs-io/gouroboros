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
| `NextBlockAndTxsInRange` | 7 | Server → Client | Next block in range |
| `LastBlockAndTxsInRange` | 8 | Server → Client | Last block in range |
| `Done` | 9 | Client → Server | Terminate protocol |

Message IDs match the [`leios-prototype` CDDL at revision
8b946c4](https://github.com/cardano-scaling/cardano-blueprint/blob/8b946c431e3209b2aa70bf5362f64f42e56fb849/src/network/node-to-node/leios-fetch/messages.cddl):
it defines tags 0–9, with 7 for the next block in a range and 8 for the last.
The prototype marks range messages 6–8 as not yet implemented and describes
its CDDL tags as provisional. It defines no not-found messages at IDs 10 or 11.
CIP-0164 says a server should disconnect when requested data is unavailable.

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

`VotesRequest` identifies each requested vote by `(SlotNo, VoterId)`, where
`VoterId` is the voter's index in the epoch's stake-based committee.
`Votes` keeps the wire payload as raw CBOR and provides typed helpers for
validated `common.LeiosVote` values.

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Default Timeout | 5 seconds | Votes and BlockRange state timeout |

`BlockRangeRequest` retains at most 1,000 response messages and 64 MiB of
encoded response data per request by default. The terminal response counts
toward both limits. Configure them with `WithMaxBlockRangeResponses` and
`WithMaxBlockRangeResponseBytes`. Exceeding either limit fails the request and
the connection; split large ranges into smaller requests.

## Configuration Options

```go
leiosfetch.NewConfig(
    leiosfetch.WithBlockRequestFunc(blockRequestCallback),
    leiosfetch.WithBlockTxsRequestFunc(blockTxsRequestCallback),
    leiosfetch.WithVotesRequestFunc(votesRequestCallback),
    leiosfetch.WithBlockRangeRequestFunc(blockRangeRequestCallback),
    leiosfetch.WithTimeout(5 * time.Second),
    leiosfetch.WithMaxBlockRangeResponses(1000),
    leiosfetch.WithMaxBlockRangeResponseBytes(64 * 1024 * 1024),
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

## Unavailable requested data

A server that cannot serve a requested block or its transactions returns a
protocol error and ends the connection. LeiosFetch defines no not-found reply.

- A `BlockRequestFunc` / `BlockTxsRequestFunc` callback signals not-found by
  returning `ErrBlockNotFound` / `ErrBlockTxsNotFound` (directly or wrapped
  with `fmt.Errorf("...: %w", ...)`). The server propagates the error as a
  protocol error.

## Optional server responders

Leios fetch is optional, but an unconfigured responder must never retain server
agency. A server that accepts a request and never answers wedges the
requester's client for the life of the connection: `protocol.sendLoop` waits
for agency that only the missing response returns, so the requester can neither
issue another Leios fetch request nor detect the condition.

- An unconfigured `VotesRequestFunc` returns `Votes` with an empty CBOR array.
- An unconfigured `BlockRequestFunc` or `BlockTxsRequestFunc` returns a
  protocol error; there is no valid absence message to send.
- An unconfigured `BlockRangeRequestFunc` returns a protocol error, because
  `LastBlockAndTxsInRange` carries a mandatory block and there is no absence
  reply to send. A connection-level error is diagnosable and lets the
  requester's peer governance replace the peer; a silent hang is neither.

Errors from configured callbacks and transport failures are still propagated.

## Abandoned requests

`BlockRequest`, `BlockTxsRequest`, `VotesRequest`, and `BlockRangeRequest` are
bounded by the caller's context. These requests share one connection-wide slot
because their responses carry no request identifier. A request whose context
expires is abandoned: its delivery channel is cleared so a late response is
dropped rather than mis-delivered, and the slot stays busy so the next request
cannot be correlated with the outstanding response. For `BlockRangeRequest`,
this applies to the whole streaming exchange; the terminal response drains the
abandoned slot.

A later request waits a bounded grace period for that response to drain. If it
arrives, the connection continues normally. If it does not, the exchange is
desynchronised beyond recovery and the client fails the connection with
`ErrRequestSlotAbandoned` so peer governance can drop and replace the peer.
This is deliberately narrower than a protocol-level state timeout, which would
also fire for a healthy relay that merely responded slowly. `StateBlock` and
`StateBlockTxs` still carry no `StateMap` timeout; `StateVotes` and
`StateBlockRange` retain their protocol-level timeout for exchanges that never
return agency.

## Notes

- Part of the experimental Leios protocol suite
- Supports both single-item and streaming requests
- BlockRange allows efficient bulk retrieval
- Used in conjunction with LeiosNotify for announcements
