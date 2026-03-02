# MessageSubmission Protocol

The MessageSubmission protocol propagates authenticated messages between nodes. It is the node-to-node component of CIP-0137 (Distributed Message Queue) using a pull-based model similar to TxSubmission.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `MessageSubmission` |
| Protocol ID | `17` |
| Mode | Node-to-Node |

## State Machine

```
┌──────┐     Init      ┌──────┐
│ Init │ ─────────────►│ Idle │◄────────────────────────┐
└──────┘               └──┬───┘                         │
                          │                             │
      RequestMessageIds   │                             │ ReplyMessageIds
      (blocking)          │                             │ ReplyMessages
                          ▼                             │
                   ┌───────────────────┐                │
                   │ MessageIdsBlocking│────────────────┤
                   └─────────┬─────────┘                │
                             │                          │
                             │ Done                     │
                             ▼                          │
                         ┌──────┐                       │
                         │ Done │                       │
                         └──────┘                       │
                                                        │
      RequestMessageIds   │                             │
      (non-blocking)      │                             │
                          ▼                             │
                   ┌─────────────────────┐              │
                   │MessageIdsNonBlocking│──────────────┤
                   └─────────────────────┘              │
                                                        │
      RequestMessages     │                             │
                          ▼                             │
                   ┌──────────┐                         │
                   │ Messages │─────────────────────────┘
                   └──────────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Init** | 1 | Client | Initial state before activation |
| **Idle** | 2 | Server | Waiting for message request |
| **MessageIdsBlocking** | 3 | Client | Client provides message IDs (blocking) |
| **MessageIdsNonBlocking** | 4 | Client | Client provides message IDs (non-blocking) |
| **Messages** | 5 | Client | Client provides full messages |
| **Done** | 6 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `Init` | 0 | Client → Server | Initialize protocol |
| `RequestMessageIds` | 1 | Server → Client | Request message IDs |
| `ReplyMessageIds` | 2 | Client → Server | Provide message IDs |
| `RequestMessages` | 3 | Server → Client | Request full messages |
| `ReplyMessages` | 4 | Client → Server | Provide message bodies |
| `Done` | 5 | Client ↔ Server | Terminate protocol |

## State Transitions

### From Init (Client Agency)
| Message | New State |
|---------|-----------|
| `Init` | Idle |

### From Idle (Server Agency)
| Message | New State | Condition |
|---------|-----------|-----------|
| `RequestMessageIds` | MessageIdsBlocking | Blocking = true |
| `RequestMessageIds` | MessageIdsNonBlocking | Blocking = false |
| `RequestMessages` | Messages | |
| `Done` | Done | |

### From MessageIdsBlocking (Client Agency)
| Message | New State |
|---------|-----------|
| `ReplyMessageIds` | Idle |
| `Done` | Done |

### From MessageIdsNonBlocking (Client Agency)
| Message | New State |
|---------|-----------|
| `ReplyMessageIds` | Idle |
| `Done` | Done |

### From Messages (Client Agency)
| Message | New State |
|---------|-----------|
| `ReplyMessages` | Idle |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Init | 30 seconds | Client must send Init |
| Idle | 300 seconds | Server must request messages |
| MessageIdsBlocking | 30 seconds | Client must reply with IDs |
| MessageIdsNonBlocking | 0 (immediate) | Non-blocking, no timeout |
| Messages | 30 seconds | Client must reply with messages |
| Done | 10 seconds | Cleanup timeout |

## Configuration Options

```go
messagesubmission.NewConfig(
    messagesubmission.WithRequestMessageIdsFunc(requestIdsCallback),
    messagesubmission.WithRequestMessagesFunc(requestMsgsCallback),
    messagesubmission.WithReplyMessageIdsFunc(replyIdsCallback),
    messagesubmission.WithReplyMessagesFunc(replyMsgsCallback),
    messagesubmission.WithMaxQueueSize(100),
    messagesubmission.WithInitTimeout(30 * time.Second),
    messagesubmission.WithIdleTimeout(300 * time.Second),
    messagesubmission.WithMessageIdsBlockingTimeout(30 * time.Second),
    messagesubmission.WithMessagesTimeout(30 * time.Second),
    messagesubmission.WithAuthenticator(authenticator),
    messagesubmission.WithTTLValidator(ttlValidator),
)
```

## Usage Example

```go
// Server requests message IDs (blocking mode)
err := server.RequestMessageIdsBlocking(0, 10)

// Server requests message IDs (non-blocking mode)
err := server.RequestMessageIdsNonBlocking(0, 10)

// Client provides IDs via RequestMessageIdsFunc callback
// Client provides messages via RequestMessagesFunc callback

// Server requests full messages
err := server.RequestMessages([][]byte{msgId1})
```

## Message ID Format

```go
type MessageIDAndSize struct {
    MessageID   []byte // Message identifier
    SizeInBytes uint32 // Message size in bytes
}
```

## Notes

- Part of CIP-0137 (Distributed Message Queue)
- Pull-based model (server requests, client provides)
- Similar design to TxSubmission protocol
- Blocking mode waits for new messages
- Non-blocking mode returns immediately
- Messages are authenticated with KES signatures
- TTL prevents message replay
