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
   │ Done                             │ BlockAnnouncement
   │                                  │ BlockOffer
   │                                  │ BlockTxsOffer
   │                                  │ VotesOffer
   ▼                                  ▼
┌──────┐                           ┌──────┐
│ Done │                           │ Idle │
└──────┘                           └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for notification request |
| **Busy** | 2 | Server | Preparing notification |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `NotificationRequestNext` | 0 | Client → Server | Request next notification |
| `BlockAnnouncement` | 1 | Server → Client | Announce new block |
| `BlockOffer` | 2 | Server → Client | Offer block for download |
| `BlockTxsOffer` | 3 | Server → Client | Offer block transactions |
| `VotesOffer` | 4 | Server → Client | Offer votes for download |
| `Done` | 5 | Client → Server | Terminate protocol |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `NotificationRequestNext` | Busy |
| `Done` | Done |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `BlockAnnouncement` | Idle |
| `BlockOffer` | Idle |
| `BlockTxsOffer` | Idle |
| `VotesOffer` | Idle |

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Default Timeout | 60 seconds | Time to receive notification |

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
