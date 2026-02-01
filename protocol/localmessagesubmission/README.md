# LocalMessageSubmission Protocol

The LocalMessageSubmission protocol submits authenticated messages to the local node. It is part of CIP-0137 (Distributed Message Queue) for secure off-chain messaging.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `LocalMessageSubmission` |
| Protocol ID | `18` |
| Mode | Node-to-Client |

## State Machine

```
┌──────┐   SubmitMessage   ┌──────┐
│ Idle │ ─────────────────►│ Busy │
└──┬───┘                   └──┬───┘
   │                          │
   │ Done                     │ AcceptMessage
   │                          │ RejectMessage
   ▼                          ▼
┌──────┐                   ┌──────┐
│ Done │                   │ Idle │
└──────┘                   └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for message submission |
| **Busy** | 2 | Server | Processing submitted message |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `SubmitMessage` | 0 | Client → Server | Submit authenticated message |
| `AcceptMessage` | 1 | Server → Client | Message accepted |
| `RejectMessage` | 2 | Server → Client | Message rejected |
| `Done` | 3 | Client → Server | Terminate protocol |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `SubmitMessage` | Busy |
| `Done` | Done |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `AcceptMessage` | Idle |
| `RejectMessage` | Idle |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Idle | 300 seconds | Client must send SubmitMessage |
| Busy | 30 seconds | Server must accept or reject |

## Rejection Reasons

Messages may be rejected for various reasons including:
- Invalid signature
- Expired TTL
- Invalid message format
- Queue full

## Configuration Options

```go
localmessagesubmission.NewConfig(
    localmessagesubmission.WithSubmitMessageFunc(submitCallback),
    localmessagesubmission.WithAcceptMessageFunc(acceptCallback),
    localmessagesubmission.WithRejectMessageFunc(rejectCallback),
    localmessagesubmission.WithAuthenticator(authenticator),
    localmessagesubmission.WithTTLValidator(ttlValidator),
    localmessagesubmission.WithTimeout(30 * time.Second),
)
```

## Usage Example

```go
// Submit an authenticated message
message := &DmqMessage{
    // Message content with signature
}

err := client.SubmitMessage(message)
// AcceptMessage or RejectMessage via callbacks
```

## Message Authentication

Messages require cryptographic authentication using KES (Key-Evolving Signatures):
- Messages must be signed by a valid stake pool key
- TTL (Time-To-Live) prevents replay attacks
- Server validates signature before acceptance

## Notes

- Part of CIP-0137 (Distributed Message Queue)
- Messages are authenticated using KES signatures
- TTL validation prevents message replay
- Default authenticator uses standard KES verification
- Used for secure off-chain communication between nodes
