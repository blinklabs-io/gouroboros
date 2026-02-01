# PeerSharing Protocol

The PeerSharing protocol enables peer discovery by allowing nodes to share known peer addresses with each other. It supports network growth and resilience.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `peer-sharing` |
| Protocol ID | `10` |
| Mode | Node-to-Node |

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

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for peer request |
| **Busy** | 2 | Server | Processing peer request |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `ShareRequest` | 0 | Client → Server | Request peer addresses |
| `SharePeers` | 1 | Server → Client | Provide peer addresses |
| `Done` | 2 | Client → Server | Terminate protocol |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `ShareRequest` | Busy |
| `Done` | Done |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `SharePeers` | Idle |

## Timeouts

| Timeout | Default | Description |
|---------|---------|-------------|
| Request Timeout | 5 seconds | Time to receive peer list |

## Configuration Options

```go
peersharing.NewConfig(
    peersharing.WithShareRequestFunc(shareRequestCallback),
    peersharing.WithTimeout(5 * time.Second),
)
```

## Usage Example

```go
// Request peers from connected node
numPeersWanted := uint8(10)
peers, err := client.GetPeers(numPeersWanted)
if err != nil {
    // Handle error
}

// peers contains list of PeerAddress
for _, peer := range peers {
    // Connect to new peer
}
```

## Peer Address Format

```go
type PeerAddress struct {
    IP   net.IP
    Port uint16
}
```

## Request Parameters

The `ShareRequest` message includes the number of peers desired. The server responds with up to that many peer addresses from its known peer set.

## Notes

- Nodes share only peers they have successfully connected to
- Responses may contain fewer peers than requested
- Used for initial peer discovery and maintaining peer diversity
- Privacy considerations: nodes may limit sharing
- Complements DNS-based peer discovery
