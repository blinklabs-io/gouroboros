# ChainSync Protocol

The ChainSync protocol synchronizes blockchain state between nodes by streaming headers (NtN) or blocks (NtC) from a producer to a consumer.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `chain-sync` |
| Protocol ID (NtN) | `2` |
| Protocol ID (NtC) | `5` |
| Mode | Node-to-Node / Node-to-Client |

## State Machine

```text
                    RequestNext
              ┌──────────────────┐
              │                  ▼
┌──────┐      │           ┌──────────┐
│ Idle │──────┴──────────►│ CanAwait │◄────────┐
└──┬───┘                  └────┬─────┘         │
   │                           │               │
   │ FindIntersect    AwaitReply│               │ RollForward
   │                           │               │ RollBackward
   │                           ▼               │ (pipeline > 1)
   │                    ┌───────────┐          │
   │                    │ MustReply │──────────┘
   │                    └───────────┘
   │                           │
   │                           │ RollForward
   │                           │ RollBackward
   │                           │ (pipeline = 1)
   │                           │
   ▼                           ▼
┌───────────┐           ┌──────┐
│ Intersect │──────────►│ Idle │
└───────────┘           └──────┘
      │
      │ IntersectFound
      │ IntersectNotFound
      ▼
   ┌──────┐       Done      ┌──────┐
   │ Idle │ ───────────────►│ Done │
   └──────┘                 └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for client request |
| **CanAwait** | 2 | Server | Server may provide block or signal wait |
| **MustReply** | 3 | Server | Server must provide block (no await allowed) |
| **Intersect** | 4 | Server | Processing intersection request |
| **Done** | 5 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `RequestNext` | 0 | Client → Server | Request next block/header |
| `AwaitReply` | 1 | Server → Client | Signal to wait for new blocks |
| `RollForward` | 2 | Server → Client | Provide next block/header |
| `RollBackward` | 3 | Server → Client | Signal chain rollback |
| `FindIntersect` | 4 | Client → Server | Find common chain point |
| `IntersectFound` | 5 | Server → Client | Intersection found |
| `IntersectNotFound` | 6 | Server → Client | No intersection found |
| `Done` | 7 | Client → Server | Terminate protocol |

## State Transitions

### From Idle (Client Agency)
| Message | New State | Notes |
|---------|-----------|-------|
| `RequestNext` | CanAwait | Increments pipeline count |
| `FindIntersect` | Intersect | |
| `Done` | Done | |

### From CanAwait (Server Agency)
| Message | New State | Notes |
|---------|-----------|-------|
| `RequestNext` | CanAwait | Increments pipeline count |
| `AwaitReply` | MustReply | |
| `RollForward` | Idle | When pipeline count = 1 |
| `RollForward` | CanAwait | When pipeline count > 1 |
| `RollBackward` | Idle | When pipeline count = 1 |
| `RollBackward` | CanAwait | When pipeline count > 1 |

### From MustReply (Server Agency)
| Message | New State | Notes |
|---------|-----------|-------|
| `RollForward` | Idle | When pipeline count = 1 |
| `RollForward` | CanAwait | When pipeline count > 1 |
| `RollBackward` | Idle | When pipeline count = 1 |
| `RollBackward` | CanAwait | When pipeline count > 1 |

### From Intersect (Server Agency)
| Message | New State |
|---------|-----------|
| `IntersectFound` | Idle |
| `IntersectNotFound` | Idle |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Idle | 60 seconds | Client must send next request |
| CanAwait | 300 seconds | Server must provide block or await |
| Intersect | 5 seconds | Server must respond to intersect |
| MustReply | 300 seconds | Server must provide block |

## Limits

| Limit | Value | Description |
|-------|-------|-------------|
| Max Pipeline Limit | 100 | Maximum pipelined requests |
| Default Pipeline Limit | 75 | Default pipeline limit |
| Max Recv Queue Size | 100 | Maximum receive queue size |
| Default Recv Queue Size | 75 | Default queue size |
| Max Pending Message Bytes | 100 KB | Maximum pending message bytes |

## Pipelining

ChainSync supports request pipelining for improved throughput. The client can send multiple `RequestNext` messages before receiving responses. The pipeline count tracks outstanding requests.

## Configuration Options

```go
chainsync.NewConfig(
    chainsync.WithRollBackwardFunc(rollBackwardCallback),
    chainsync.WithRollForwardFunc(rollForwardCallback),
    chainsync.WithRollForwardRawFunc(rollForwardRawCallback),
    chainsync.WithFindIntersectFunc(findIntersectCallback),
    chainsync.WithRequestNextFunc(requestNextCallback),
    chainsync.WithIntersectTimeout(5 * time.Second),
    chainsync.WithBlockTimeout(300 * time.Second),
    chainsync.WithPipelineLimit(75),
    chainsync.WithRecvQueueSize(75),
)
```

## Usage Example

```go
// Find intersection point
client.FindIntersect(knownPoints)

// Stream blocks
for {
    client.RequestNext()
    // Handle RollForward/RollBackward in callbacks
}
```

## Notes

- Node-to-Node mode streams headers only (use BlockFetch for full blocks)
- Node-to-Client mode streams full blocks
- Pipelining significantly improves sync performance
- The protocol supports chain rollbacks during reorganizations
