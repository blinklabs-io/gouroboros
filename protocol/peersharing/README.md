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

## Handshake negotiation

PeerSharing is gated by the handshake's `PeerSharing` mode field. Each side
advertises one of the modes below. The protocol channel is multiplexed onto the
connection for any version 11+ regardless of the advertised modes, but
gouroboros refuses to send or honour `ShareRequest` messages when either side
opted out.

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
| NoPeerSharing | active | no — would be a self-protocol violation; client should not be calling `GetPeers` | no — `ErrLocalPeerSharingDisabled` |
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

`ShareRequestFunc` is responsible for any per-request budget enforcement.
gouroboros does not cap the `Amount` value or the size of the returned slice
beyond the protocol's own `uint8` limit on `Amount`; operators that want to
publish fewer peers should clamp inside the callback.

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
`WithPeerSharing(false)`), incoming `ShareRequest` messages are refused with
`ErrLocalPeerSharingDisabled`, which surfaces as a protocol error and closes
the connection. A spec-compliant peer would not send `ShareRequest` in that
case.

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
- Responses may contain fewer peers than requested.
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
