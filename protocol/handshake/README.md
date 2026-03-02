# Handshake Protocol

The Handshake protocol negotiates protocol versions and capabilities between two Ouroboros nodes at connection establishment.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `handshake` |
| Protocol ID | `0` |
| Mode | Node-to-Node / Node-to-Client |

## State Machine

```text
┌─────────┐  ProposeVersions   ┌─────────┐
│ Propose │ ────────────────► │ Confirm │
└─────────┘                    └────┬────┘
     │                              │
     │                    AcceptVersion │
     │                    Refuse        │
     │                    QueryReply    │
     │                              │
     │                              ▼
     │                         ┌──────┐
     └────────────────────────►│ Done │
                               └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Propose** | 1 | Client | Initial state; client proposes supported versions |
| **Confirm** | 2 | Server | Server confirms or refuses version |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `ProposeVersions` | 0 | Client → Server | Propose supported protocol versions |
| `AcceptVersion` | 1 | Server → Client | Accept a proposed version |
| `Refuse` | 2 | Server → Client | Refuse all proposed versions |
| `QueryReply` | 3 | Server → Client | Reply to version query |

## State Transitions

### From Propose (Client Agency)
| Message | New State |
|---------|-----------|
| `ProposeVersions` | Confirm |

### From Confirm (Server Agency)
| Message | New State |
|---------|-----------|
| `AcceptVersion` | Done |
| `Refuse` | Done |
| `QueryReply` | Done |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Propose | 5 seconds | Client must propose versions |
| Confirm | 5 seconds | Server must accept or refuse |

## Refusal Reasons

| Reason | Code | Description |
|--------|------|-------------|
| Version Mismatch | 0 | No compatible version found |
| Decode Error | 1 | Failed to decode version data |
| Refused | 2 | General refusal |

## Configuration Options

```go
handshake.NewConfig(
    handshake.WithProtocolVersionMap(versionMap),
    handshake.WithFinishedFunc(finishedCallback),
    handshake.WithQueryReplyFunc(queryReplyCallback),
    handshake.WithTimeout(5 * time.Second),
)
```

## Usage Example

```go
// Client initiates handshake with supported versions
// Start() sends the ProposeVersions message automatically
client.Start()

// Server responds with accepted version
// or Refuse message if no compatible version
// Handle response via FinishedFunc callback
```

## Notes

- The handshake must complete before any other protocol can be used
- Version data contains network-specific parameters (magic number, etc.)
- Both node-to-node and node-to-client connections use this protocol
