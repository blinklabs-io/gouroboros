# MessageSubmission Protocol

The MessageSubmission protocol propagates authenticated messages between nodes. It is the node-to-node component of CIP-0137 (Distributed Message Queue) using a pull-based model similar to TxSubmission.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `MessageSubmission` |
| Protocol ID | `11` |
| Mode | Node-to-Node |

## Sides and versions

The **Client** has messages and answers requests for them; the **Server**
wants messages and requests them. The wire format and the side that starts
follow the reference implementation, dmq-node, where it differs from
CIP-0137:

| Version | Negotiated as | Wire format | Initiator |
|---------|---------------|-------------|-----------|
| V1 | DMQ `NodeToNodeV_1`, or a Cardano NtN version below 15 | tx-submission's | Client (sends `Init`) |
| V2 | DMQ `NodeToNodeV_2`, or a Cardano NtN version of 15 or more | dmq-node's `SigSubmissionV2` | Server (sends the first request) |

In V2 the Server is registered on the muxer as the initiator and the Client
as the responder. Start the Server on the side that opened the connection.

## States

| State | V1 agency | V2 agency |
|-------|-----------|-----------|
| **Init** | Client | (none: V2 starts in Idle) |
| **Idle** | Server | Server |
| **MessageIdsBlocking** | Client | Client |
| **MessageIdsNonBlocking** | Client | Client |
| **Messages** | Client | Client |
| **Done** | None | None |

## Messages

Every collection is encoded as an indefinite-length CBOR list; the reference
decoders accept no other form.

| Message | V1 tag | V2 tag | Sender | From state | To state |
|---------|--------|--------|--------|------------|----------|
| `Init` | 6 | - | Client | Init | Idle |
| `RequestMessageIds` | 0 | 1 | Server | Idle | MessageIdsBlocking or MessageIdsNonBlocking |
| `ReplyMessageIds` | 1 | 2 | Client | MessageIds* | Idle |
| `ReplyNoMessageIds` | - | 3 | Client | MessageIdsBlocking | Idle |
| `RequestMessages` | 2 | 4 | Server | Idle | Messages |
| `ReplyMessages` | 3 | 5 | Client | Messages | Idle |
| `Done` | 4 | 6 | V1: Client; V2: Server | V1: MessageIdsBlocking; V2: Idle | Done |

`ReplyNoMessageIds` (`Client.ReplyNoMessageIds`) answers a V2 blocking
request with no IDs and still applies the request's acknowledgement.
dmq-node's V2 requester allows 20 seconds for a blocking answer, so a Client
with nothing to announce should send it before then.

In V2 the Server's `Done` ends a run, not the Client's protocol. dmq-node
sends it when it demotes the peer and starts a new run on the same connection
when it promotes the peer again, and the reference restarts the responder for
that run. The Client calls `DoneFunc`, forgets the run's outstanding IDs and
returns to Idle to answer the next request. The Idle timeout does not apply
until that request arrives.

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Init | 30 seconds | Client must send Init |
| Idle | 300 seconds | Server must request messages |
| MessageIdsBlocking | V1: none; V2: 30 seconds | Client must reply with IDs; in V1 it may wait until it has one |
| MessageIdsNonBlocking | 10 seconds | Client must reply at once, possibly with no IDs |
| Messages | 30 seconds | Client must reply with messages |
| Done | 10 seconds | Cleanup timeout |

A timeout bounds the wait of the side without agency, so the Server, which
sends the requests, is timed by the replies and the Client by Idle. Both
install the configured timeouts. `WithMessageIdsBlockingTimeout` applies only
to a V2 blocking request. `WithInitTimeout` and `WithDoneTimeout` have no
effect: the initial state is never timed and nothing waits in Done.

## Configuration Options

```go
messagesubmission.NewConfig(
    messagesubmission.WithRequestMessageIdsFunc(requestIdsCallback),
    messagesubmission.WithRequestMessagesFunc(requestMsgsCallback),
    messagesubmission.WithReplyMessageIdsFunc(replyIdsCallback),
    messagesubmission.WithReplyMessagesFunc(replyMsgsCallback),
    messagesubmission.WithMaxQueueSize(100),
    messagesubmission.WithMaxUnacknowledgedMessageIDs(132),
    messagesubmission.WithInitTimeout(30 * time.Second),
    messagesubmission.WithIdleTimeout(300 * time.Second),
    messagesubmission.WithMessageIdsBlockingTimeout(30 * time.Second),
    messagesubmission.WithMessagesTimeout(30 * time.Second),
    messagesubmission.WithAuthenticator(authenticator),
    messagesubmission.WithTTLValidator(ttlValidator),
)
```

`MaxUnacknowledgedMessageIDs` defaults to 132, dmq-node's window. Both roles
reject a request that would make the IDs awaiting acknowledgement exceed this
limit. The window is not negotiated, and dmq-node's requester fills its own,
so a Client configured below 132 refuses requests a dmq-node peer may send.
Non-blocking ID requests must leave at least one ID outstanding after applying
their acknowledgement count.

Replies may omit unavailable entries. A message-body reply may use any order,
but every returned message ID must be unique and present in the active request.
Message-ID replies may not exceed the requested count, and blocking replies
must include at least one ID. Each returned ID must be 32 bytes and unique
across both the reply and the IDs that remain outstanding.

Collection counts are checked before typed decoding against the same request
state: a reply larger than its outstanding request, or a message request naming
more IDs than the client holds unacknowledged, is refused before allocation.

## Usage Example

```go
// Server requests message IDs (blocking mode)
err := server.RequestMessageIdsBlocking(0, 10)

// After the peer announces at least two IDs, acknowledge one while leaving
// another outstanding and request up to ten more without blocking.
err = server.RequestMessageIdsNonBlocking(1, 10)

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
