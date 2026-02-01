# Ouroboros Mini-Protocols

This directory contains implementations of the Ouroboros mini-protocols used for communication between Cardano nodes and clients.

## Protocol Overview

| Protocol | ID | Mode | Purpose |
|----------|-----|------|---------|
| [Handshake](handshake/) | 0 | NtN/NtC | Version negotiation |
| [ChainSync](chainsync/) | 2/5 | NtN/NtC | Blockchain synchronization |
| [BlockFetch](blockfetch/) | 3 | NtN | Block retrieval |
| [TxSubmission](txsubmission/) | 4 | NtN | Transaction propagation |
| [LocalTxSubmission](localtxsubmission/) | 6 | NtC | Local transaction submission |
| [LocalStateQuery](localstatequery/) | 7 | NtC | Ledger state queries |
| [KeepAlive](keepalive/) | 8 | NtN | Connection liveness |
| [LocalTxMonitor](localtxmonitor/) | 9 | NtC | Mempool monitoring |
| [PeerSharing](peersharing/) | 10 | NtN | Peer discovery |
| [MessageSubmission](messagesubmission/) | 17 | NtN | DMQ message propagation |
| [LeiosNotify](leiosnotify/) | 18 | NtN | Leios notifications |
| [LocalMessageSubmission](localmessagesubmission/) | 18 | NtC | Local DMQ submission |
| [LeiosFetch](leiosfetch/) | 19 | NtN | Leios data retrieval |
| [LocalMessageNotification](localmessagenotification/) | 19 | NtC | Local DMQ notifications |

**Mode Key:**
- **NtN**: Node-to-Node (between full nodes)
- **NtC**: Node-to-Client (between node and wallet/application)

## Protocol Architecture

### State Machines

Each protocol is defined by a state machine with:
- **States**: Named protocol states with numeric IDs
- **Agency**: Which party (Client/Server) can send messages in each state
- **Transitions**: Valid message types that trigger state changes
- **Timeouts**: Optional deadlines for state transitions

### Agency Model

The Ouroboros protocol uses an agency model where only one party can send messages at a time:
- **Client Agency**: Client sends, server waits
- **Server Agency**: Server sends, client waits
- **None**: Terminal state (Done)

### Common Patterns

**Acquire/Release Pattern** (LocalStateQuery, LocalTxMonitor):
```
Idle → Acquiring → Acquired → (query) → Acquired → Release → Idle
```

**Request/Reply Pattern** (BlockFetch, PeerSharing):
```
Idle → Request → Busy → Reply → Idle
```

**Streaming Pattern** (ChainSync, BlockFetch):
```
Idle → Request → Streaming → (data)* → Done → Idle
```

**Init Pattern** (TxSubmission, MessageSubmission):
```
Init → Idle → (request/reply cycle)
```

## Protocol Groups

### Core Synchronization
- **Handshake**: Establishes protocol version
- **ChainSync**: Synchronizes blockchain headers/blocks
- **BlockFetch**: Retrieves full block bodies

### Transaction Handling
- **TxSubmission**: Node-to-node transaction propagation
- **LocalTxSubmission**: Submit transactions to local node
- **LocalTxMonitor**: Monitor local mempool

### State & Discovery
- **LocalStateQuery**: Query ledger state
- **PeerSharing**: Discover new peers
- **KeepAlive**: Maintain connection liveness

### CIP-0137 (DMQ)
- **MessageSubmission**: Node-to-node message propagation
- **LocalMessageSubmission**: Submit messages to local node
- **LocalMessageNotification**: Receive message notifications

### Leios (Experimental)
- **LeiosNotify**: Block/vote announcements
- **LeiosFetch**: Block/vote retrieval

## Usage

### Creating a Protocol Instance

```go
import (
    "log/slog"

    "github.com/blinklabs-io/gouroboros/protocol"
    "github.com/blinklabs-io/gouroboros/protocol/chainsync"
)

// Create protocol options
protoOptions := protocol.ProtocolOptions{
    ConnectionId: connId,                              // connection.ConnectionId
    Muxer:        muxer,                               // *muxer.Muxer
    Logger:       slog.Default(),                      // *slog.Logger
    ErrorChan:    errorChan,                           // chan error
    Mode:         protocol.ProtocolModeNodeToClient,
    Role:         protocol.ProtocolRoleClient,
    Version:      versionNumber,                       // uint16
}

// Create protocol config with callbacks
cfg := chainsync.NewConfig(
    chainsync.WithRollForwardFunc(handleRollForward),
    chainsync.WithRollBackwardFunc(handleRollBackward),
)

// Create protocol instance
cs := chainsync.New(protoOptions, &cfg)
```

### Starting the Protocol

```go
// Start the protocol
cs.Client.Start()

// Use protocol methods
cs.Client.RequestNext()
```

## Common Configuration

All protocols support these common options:
- **Timeouts**: Per-state or operation timeouts
- **Callbacks**: Functions called on message receipt
- **Queue Sizes**: Receive queue capacity

## Error Handling

Protocols report errors through the error channel:
```go
errChan := make(chan error, 10)
protoOptions := protocol.ProtocolOptions{
    ErrorChan: errChan,
    // ...
}

go func() {
    for err := range errChan {
        slog.Default().Error("Protocol error", "error", err)
    }
}()
```

## Limits

Each protocol defines specific limits documented in its README:
- Message sizes
- Queue depths
- Pipeline counts
- Pending byte limits

## References

- [Ouroboros Network Specification](https://github.com/IntersectMBO/ouroboros-network)
- [CIP-0137: Distributed Message Queue](https://github.com/cardano-foundation/CIPs/tree/master/CIP-0137)
- [Leios Protocol Specification](https://github.com/input-output-hk/ouroboros-leios)
