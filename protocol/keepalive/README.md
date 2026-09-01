# KeepAlive Protocol

The KeepAlive protocol maintains connection liveness between nodes by exchanging periodic ping/pong messages. It detects dead connections and triggers reconnection.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `keep-alive` |
| Protocol ID | `8` |
| Mode | Node-to-Node |

## State Machine

```text
┌────────┐   KeepAlive    ┌────────┐
│ Client │ ──────────────►│ Server │
└────┬───┘                └───┬────┘
     │                        │
     │ Done                   │ KeepAliveResponse
     │                        │
     ▼                        ▼
┌──────┐                 ┌────────┐
│ Done │                 │ Client │
└──────┘                 └────────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Client** | 1 | Client | Client may send ping |
| **Server** | 2 | Server | Server must respond with pong |
| **Done** | 3 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `KeepAlive` | 0 | Client → Server | Ping with cookie |
| `KeepAliveResponse` | 1 | Server → Client | Pong echoing cookie |
| `Done` | 2 | Client → Server | Terminate protocol |

## State Transitions

### From Client (Client Agency)
| Message | New State |
|---------|-----------|
| `KeepAlive` | Server |
| `Done` | Done |

### From Server (Server Agency)
| Message | New State |
|---------|-----------|
| `KeepAliveResponse` | Client |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Client | 60 seconds | Server waiting for next ping from client |
| Server | 10 seconds | Client waiting for pong from server |

## Default Configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| Period | 60 seconds | Interval between keep-alive probes |
| Timeout | 10 seconds | Response timeout |

## Configuration Options

```go
keepalive.NewConfig(
    keepalive.WithKeepAliveFunc(keepAliveCallback),
    keepalive.WithKeepAliveResponseFunc(responseCallback),
    keepalive.WithDoneFunc(doneCallback),
    keepalive.WithTimeout(10 * time.Second),
    keepalive.WithPeriod(60 * time.Second),
    keepalive.WithCookie(0x1234),
)
```

## Usage Example

```go
// Client sends periodic keep-alive
client.Start()

// Automatic ping every period
// If no response within timeout, connection is dead

// Server responds to pings automatically
server.Start()
```

## Cookie

The `KeepAlive` message includes a 16-bit cookie that must be echoed in the `KeepAliveResponse`. This verifies that responses correspond to specific pings.

```go
type MsgKeepAlive struct {
    Cookie uint16
}

type MsgKeepAliveResponse struct {
    Cookie uint16 // Must match request
}
```

## Notes

- Both client and server roles are typically active
- Timeout triggers connection closure
- Cookie ensures response matches request
- Protocol runs continuously during connection lifetime
- Essential for detecting network partitions
