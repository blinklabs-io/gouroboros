# PeerSharing Protocol

The PeerSharing protocol enables peer discovery by letting nodes exchange known
peer addresses. It is one of the node-to-node mini-protocols multiplexed over
an Ouroboros connection.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `peer-sharing` |
| Protocol ID | `10` |
| Mode | Node-to-Node |
| Min handshake version | `11` |

## State Machine

```text
┌──────┐   ShareRequest   ┌──────┐
│ Idle │ ────────────────►│ Busy │
└──┬───┘                  └──┬───┘
   │                         │
   │ Done                    │ SharePeers
   │                         │
   ▼                         ▼
┌──────┐                  ┌──────┐
│ Done │                  │ Idle │
└──────┘                  └──────┘
```

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for peer request |
| **Busy** | 2 | Server | Processing peer request |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `ShareRequest` | 0 | Client → Server | Request up to `Amount` peer addresses |
| `SharePeers` | 1 | Server → Client | Return zero or more peer addresses |
| `Done` | 2 | Client → Server | Terminate the protocol instance |

`ShareRequest.Amount` is a single byte (`uint8`), so the client may request at
most 255 addresses per round trip. The server is free to return fewer, including
none.

## Timeouts

| Timeout | Value | Description |
|---------|-------|-------------|
| Busy state (server) | 60s (`BusyTimeout`) | Maximum time the client waits for `SharePeers` after sending `ShareRequest`. Per Ouroboros Network Spec Table 3.15. |

The default is exposed as `peersharing.BusyTimeout`. The server's timeout can
be overridden per connection via `peersharing.WithTimeout`.

## Size limits

| Limit | Value | Applies to |
|-------|-------|------------|
| `MaxPendingMessageBytes` | 5,760 | Pending message bytes in the Idle and Busy states |
| `MaxSharedPeers` | 230 | Addresses the server will put in one `SharePeers` |

`MaxPendingMessageBytes` is the reference implementation's figure, used there
both as the mini-protocol ingress queue (`peerSharingProtocolLimits`) and as
the per-state codec size limit (`byteLimitsPeerSharing`): four 1440-byte TCP
segments, one initial congestion window, so a request and its reply complete
within a single round trip. The protocol framework refuses a received message
above it and fails the protocol.

`MaxSharedPeers` follows from that. A `PeerAddress` encodes to at most 25
bytes in its IPv6 form (6-element array header, peer type, four `uint32` words
at 5 bytes each, 3-byte port) and 10 bytes in its IPv4 form. A `SharePeers`
message adds a 2-byte frame and, above 23 entries, a 2-byte array header, so
230 maximum-size addresses encode to 5,754 bytes and 231 to 5,779. Because
`Amount` is a `uint8`, a peer may ask for 255, whose maximum-size reply would
encode to 6,379 bytes and exceed the limit.

The client additionally rejects a `SharePeers` carrying more addresses than it
asked for with `ErrTooManyPeersShared`. The protocol permits a shorter reply;
a longer one is a protocol violation, and the reference implementation raises
`PeerSharingProtocolViolation` in the same case.

## Handshake negotiation

PeerSharing is gated by the handshake's `PeerSharing` mode field. Each side
advertises one of the modes below. The protocol channel is multiplexed onto the
connection for any version 11+ regardless of the advertised modes. The client
refuses to send `ShareRequest` when the remote opted out. If the local side
opted out but still receives an invalid request, the server returns an empty
`SharePeers` array (`[1, []]`) so the request does not close the shared
connection.

### Mode values per version

| Handshake version | Mode 0 | Mode 1 | Mode 2 |
|---|---|---|---|
| 11, 12 | NoPeerSharing | PeerSharingPrivate | PeerSharingPublic |
| 13+ | NoPeerSharing | PeerSharingPublic | (unused) |

Both `Private` and `Public` are treated as "active" — the only mode that
disables peer sharing is `NoPeerSharing`.

### Runtime gating

| Local advertised | Remote advertised | Client may send `ShareRequest`? | Server honours incoming `ShareRequest`? |
|---|---|---|---|
| any | NoPeerSharing | no — `ErrRemotePeerSharingDisabled` | yes (if local advertised active) |
| NoPeerSharing | active | no — would be a self-protocol violation; client should not be calling `GetPeers` | empty `SharePeers` response |
| active | active | yes | yes |

Operators flip the local advertisement with `ouroboros.WithPeerSharing(true)`
on the connection. The remote's mode is taken from the negotiated handshake
version data and applied automatically; no user wiring is required.

The gating is implemented as positive `LocalDisabled` / `RemoteDisabled` flags
on `peersharing.Config`. Their zero value (false) preserves the permissive
legacy behaviour, so direct callers of `peersharing.New` who do not run a
handshake are not affected — only the connection layer, which knows the
handshake outcome, sets these flags to `true`.

## Operator-Facing Configuration

Connection-level options (in `github.com/blinklabs-io/gouroboros`):

| Option | Effect |
|---|---|
| `WithPeerSharing(bool)` | Sets the `PeerSharing` mode advertised to the remote during the handshake. `false` (the default) advertises `NoPeerSharing`; `true` advertises the public/v13+ public mode. |
| `WithPeerSharingConfig(peersharing.Config)` | Supplies the protocol's runtime config — primarily the server-side `ShareRequestFunc` callback. The connection layer overlays `LocalDisabled` and `RemoteDisabled` on this config based on the handshake outcome; operator-supplied values for those fields are ignored. |

Protocol-level option helpers (in
`github.com/blinklabs-io/gouroboros/protocol/peersharing`):

| Option | Effect |
|---|---|
| `WithShareRequestFunc(fn)` | Required server-side callback. `fn(ctx, amount)` is invoked when a peer asks for up to `amount` peers; the returned slice (possibly nil) is sent back as `SharePeers`. |
| `WithTimeout(d)` | Overrides the server's `BusyTimeout` (default 60s). |
| `WithLocalDisabled(bool)` | Internal: set by the connection layer when the local node advertised NoPeerSharing. Exposed for tests. |
| `WithRemoteDisabled(bool)` | Internal: set by the connection layer when the remote peer advertised NoPeerSharing. Exposed for tests. |

`ShareRequestFunc` selects which peers to publish. The server clamps whatever
the callback returns to the smaller of the requested `Amount` and
`MaxSharedPeers`, so a longer slice publishes only its leading entries;
operators that want to publish fewer peers, or to choose which ones, should
clamp inside the callback. See [Size limits](#size-limits).

## Usage

### Requesting peers (client)

```go
oConn, err := ouroboros.New(
    ouroboros.WithConnection(c),
    ouroboros.WithNodeToNode(true),
    ouroboros.WithPeerSharing(true), // we are willing to share
    // ... other options ...
)
if err != nil {
    return err
}

ps := oConn.PeerSharing()
if ps == nil {
    // Negotiated handshake version was < 11; peer sharing not available.
    return nil
}
peers, err := ps.Client.GetPeers(10)
switch {
case errors.Is(err, peersharing.ErrRemotePeerSharingDisabled):
    // Peer advertised NoPeerSharing during handshake; nothing to do.
    return nil
case err != nil:
    return err
}
for _, p := range peers {
    // Connect to p.IP:p.Port
}
```

### Serving peers (server)

```go
shareCallback := func(_ peersharing.CallbackContext, amount int) ([]peersharing.PeerAddress, error) {
    n := amount
    if n > maxPeersPerResponse {
        n = maxPeersPerResponse
    }
    return pickKnownPeers(n), nil
}

cfg := peersharing.NewConfig(
    peersharing.WithShareRequestFunc(shareCallback),
)

oConn, err := ouroboros.New(
    ouroboros.WithConnection(c),
    ouroboros.WithNodeToNode(true),
    ouroboros.WithPeerSharing(true),
    ouroboros.WithPeerSharingConfig(cfg),
)
```

When the local node advertises `NoPeerSharing` (the default,
`WithPeerSharing(false)`), an unexpected incoming `ShareRequest` receives an
empty `SharePeers` array (`[1, []]`). A spec-compliant peer would not send the
request, but declining it at the mini-protocol boundary keeps an invalid
optional request from closing the shared connection. The same empty response
is used when no `ShareRequestFunc` is configured.

## Peer Address Format

```go
type PeerAddress struct {
    IP   net.IP
    Port uint16
}
```

The wire format depends on the negotiated handshake version:

| Handshake version | IPv4 shape | IPv6 shape |
|---|---|---|
| 11, 12 | `[0, addrLE32, port]` (3 elements) | `[1, a1..a4, flowInfo, scopeId, port]` (8 elements) |
| 13+ | `[0, addrLE32, port]` (3 elements) | `[1, a1..a4, port]` (6 elements) |

`IP` is represented in little-endian 32-bit words on the wire, matching the
Haskell reference implementation. The decoder accepts both v11/v12 and v13+
IPv6 shapes.

## Notes

- Nodes typically share only peers they have successfully connected to.
- Responses may contain fewer peers than requested, but never more.
- The protocol complements DNS-based discovery; it is not a replacement.
- Privacy: the `Private` mode (v11/v12 only) is treated as "active" by
  gouroboros. Selection-policy semantics for that mode are not enforced here;
  any policy beyond on/off lives in the operator's `ShareRequestFunc`.

## Interop verification

Code-level coverage (CBOR shapes, state machine, handshake gating) is
exercised by the unit tests in this package and in `protocol/`. Live
interoperability against the Haskell `cardano-node` implementation on a
public testnet (e.g. preview) is not covered by automated tests in this
repository — running such an interop test requires a Haskell-node peer and a
preview-net connection, and is performed out-of-band by maintainers.
