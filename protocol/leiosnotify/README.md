# LeiosNotify Protocol

The LeiosNotify protocol provides notifications about new Leios blocks, transactions, and votes. It is the announcement component of the experimental Leios high-throughput protocol suite.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `leios-notify` |
| Protocol ID | `18` |
| Mode | Node-to-Node |

## State Machine

```
┌──────┐  NotificationRequestNext  ┌──────┐
│ Idle │ ─────────────────────────►│ Busy │
└──┬───┘                           └──┬───┘
   │                                  │
   │ Quit                             │ BlockAnnouncement
   │                                  │ BlockOffer
   │                                  │ BlockTxsOffer
   │                                  │ VotesOffer
   ▼                                  │ Canceled
┌──────┐  Done   ┌──────┐             ▼
│ Quit │ ───────►│ Done │          ┌──────┐
└──────┘         └──────┘          │ Idle │
                                   └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for notification request |
| **Busy** | 2 | Server | Preparing notification |
| **Done** | 3 | None | Terminal state |
| **Quit** | 4 | Server | Client quit; server acknowledges |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `Done` | 0 | Server → Client | Acknowledge `Quit` and end the protocol |
| `Quit` | 1 | Client → Server | End the protocol |
| `Canceled` | 2 | Server → Client | Reply to a request outstanding at `Quit` |
| `NotificationRequestNext` | 3 | Client → Server | Request next notification |
| `BlockAnnouncement` | 4 | Server → Client | Announce new block |
| `BlockOffer` | 5 | Server → Client | Offer block for download |
| `BlockTxsOffer` | 6 | Server → Client | Offer block transactions |
| `VotesOffer` | 7 | Server → Client | Offer votes for download |

These tags match the LeiosNotify messages of the node of Leios release
candidate 2026w41, which numbers the messages of both Leios mini-protocols
from zero.

A client sends further requests, and `Quit`, without waiting for the reply to
an outstanding request. The protocol holds them back until the server has
replied, so the server answers every request before it acknowledges `Quit`.
`Quit` lets a client demote a peer from hot to warm without dropping the
connection for lack of Leios traffic.

A server answers each request still outstanding when the client quits with
`Canceled`, or with its notification if that is already ready, and then sends
`Done`. It reads the `Quit` while a request waits for a notification: the
server queues up to 1,002 received messages, enough for the 1,000 requests the
reference client pipelines. `CallbackContext.QuitChan` closes when the client
quits so a `RequestNextFunc` can stop waiting. A notification the source hands
back after the quit is not sent; `ResponseSentFunc` receives
`ErrRequestCanceled` for it.

A block announcement carries the ranking-block header in the hard-fork
combinator's node-to-node encoding, `[4, [era, #6.24(bytes .cbor header)]]`,
the same wrapping ChainSync uses for the headers of the Shelley-based eras.
`MsgBlockAnnouncement` holds the era index and the unwrapped header. The
reference node reads the era as a single byte and drops the connection on a
bare header, so era 0 (Byron) and indices above 255 are rejected.

A block offer is rejected unless its point hash is 32 bytes and its size is
between 1 and 600,000.

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `NotificationRequestNext` | Busy |
| `Quit` | Quit |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `BlockAnnouncement` | Idle |
| `BlockOffer` | Idle |
| `BlockTxsOffer` | Idle |
| `VotesOffer` | Idle |
| `Canceled` | Idle |

### From Quit (Server Agency)
| Message | New State |
|---------|-----------|
| `Done` | Done |

`VotesOffer` advertises available votes by `(SlotNo, VoterId)`, where
`VoterId` is the voter's index in the epoch's stake-based committee.
Each offer is limited to `MaxVotesOfferCount` (1,000) entries and
`MaxVotesOfferBytes` (65,535 bytes). The byte limit is checked before CBOR parsing;
the decoder then checks the array header before scanning vote values and
applies the count limit to definite- and indefinite-length arrays.
`BlockAnnouncement` is limited to `MaxBlockAnnouncementBytes` (one muxer
segment), checked before CBOR parsing.

## Byte budget

`Config.MaxPendingBytes` bounds the encoded bytes of received messages
queued for their handler, and is the largest message accepted. Every state
uses it as its pending-message byte limit, and the receive queue holds
`MaxPendingBytes / MaxBlockAnnouncementBytes` messages.

It is not the whole amount a client holds. While the budget is held the
protocol takes no more from the muxer, but the muxer keeps reading into its
ingress queue for the protocol, which is limited to `MaxPendingBytes` or ten
maximum-size segments, whichever is larger. Ingress past that fails the
connection with `muxer.ErrIngressOverflow`. Whole segments are also held
outside both limits: the muxer's delivery channel and forwarder, the read
loop's batch, the segment that overflowed, and the notification inside
`NotificationFunc`, 24 in all. At the default budget of 655,350 bytes and
maximum-size segments that bound is 2,883,732 bytes.

Zero sizes it to the larger of `PipelineLimit * MaxBlockAnnouncementBytes`
and `MaxVotesOfferBytes`. An explicit value must be at least each of those
two values; they are checked separately, not summed.

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Default Timeout | 60 seconds | Time for the server to send `Done` after `Quit` |

Idle and Busy have no timeout: a server with nothing to notify holds agency
until it has something to send.

## Configuration Options

```go
leiosnotify.NewConfig(
    leiosnotify.WithRequestNextFunc(requestNextCallback),
    leiosnotify.WithTimeout(60 * time.Second),
)
```

## Usage Example

```go
// Request notifications in a loop
for {
    notification, err := client.RequestNext()
    if err != nil {
        // Handle error
        break
    }

    switch n := notification.(type) {
    case *BlockAnnouncement:
        // New block announced
    case *BlockOffer:
        // Block available for download via LeiosFetch
    case *VotesOffer:
        // Votes available for download
    }
}
```

## Notification Types

| Type | Description |
|------|-------------|
| BlockAnnouncement | New block header announcement |
| BlockOffer | Full block available for retrieval |
| BlockTxsOffer | Block transactions available |
| VotesOffer | Vote bundle available |

## Notes

- Client must explicitly request each notification
- Works with LeiosFetch to retrieve announced data
- Supports efficient push-based data dissemination
- Part of the experimental Leios high-throughput design
