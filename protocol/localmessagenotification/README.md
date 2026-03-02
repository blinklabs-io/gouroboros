# LocalMessageNotification Protocol

The LocalMessageNotification protocol receives notifications about new messages from the local node. It is part of CIP-0137 (Distributed Message Queue) for secure off-chain messaging.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `LocalMessageNotification` |
| Protocol ID | `19` |
| Mode | Node-to-Client |

## State Machine

```
┌──────┐  RequestMessages   ┌──────────────────┐
│ Idle │  (non-blocking)    │ BusyNonBlocking  │
└──┬───┴───────────────────►└────────┬─────────┘
   │                                 │
   │ RequestMessages                 │ ReplyMessagesNonBlocking
   │ (blocking)                      │
   │                                 ▼
   │                            ┌──────┐
   │                            │ Idle │
   │                            └──────┘
   │
   ▼
┌───────────────┐
│ BusyBlocking  │
└───────┬───────┘
        │
        │ ReplyMessagesBlocking
        ▼
   ┌──────┐           Done       ┌──────┐
   │ Idle │ ────────────────────►│ Done │
   └──────┘                      └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for message request |
| **BusyNonBlocking** | 2 | Server | Processing non-blocking request |
| **BusyBlocking** | 3 | Server | Processing blocking request |
| **Done** | 4 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `RequestMessages` | 0 | Client → Server | Request messages (blocking/non-blocking) |
| `ReplyMessagesNonBlocking` | 1 | Server → Client | Non-blocking response |
| `ReplyMessagesBlocking` | 2 | Server → Client | Blocking response |
| `ClientDone` | 3 | Client → Server | Terminate protocol |

## State Transitions

### From Idle (Client Agency)
| Message | New State | Condition |
|---------|-----------|-----------|
| `RequestMessages` | BusyNonBlocking | Non-blocking request |
| `RequestMessages` | BusyBlocking | Blocking request |
| `ClientDone` | Done | |

### From BusyNonBlocking (Server Agency)
| Message | New State |
|---------|-----------|
| `ReplyMessagesNonBlocking` | Idle |

### From BusyBlocking (Server Agency)
| Message | New State |
|---------|-----------|
| `ReplyMessagesBlocking` | Idle |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Idle | 300 seconds | Client must send RequestMessages |
| BusyNonBlocking | 0 (immediate) | Non-blocking returns immediately |
| BusyBlocking | 0 (indefinite) | Blocking waits for messages |

## Request Modes

| Mode | Description |
|------|-------------|
| **Non-blocking** | Returns immediately with available messages |
| **Blocking** | Waits until messages are available |

## Configuration Options

```go
localmessagenotification.NewConfig(
    localmessagenotification.WithReplyMessagesFunc(replyCallback),
    localmessagenotification.WithMaxQueueSize(100),
    localmessagenotification.WithBlockingRequestTimeout(timeout),
    localmessagenotification.WithAuthenticator(authenticator),
    localmessagenotification.WithTTLValidator(ttlValidator),
)
```

## Usage Example

```go
// Configure client with reply callback
cfg := localmessagenotification.NewConfig(
    localmessagenotification.WithReplyMessagesFunc(func(ctx CallbackContext, messages []DmqMessage, hasMore bool) {
        for _, msg := range messages {
            // Handle message
        }
    }),
)

// Non-blocking: get available messages immediately
err := client.RequestMessagesNonBlocking()

// Blocking: wait for new messages
err := client.RequestMessagesBlocking()
```

## Message Authentication

Received messages have been validated:
- KES signature verification
- TTL validation
- Message format validation

## Notes

- Part of CIP-0137 (Distributed Message Queue)
- Non-blocking mode for polling
- Blocking mode for push-style notification
- Messages are pre-validated by the node
- Default queue size is 100 messages
