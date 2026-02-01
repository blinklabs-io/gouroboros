# BlockFetch Protocol

The BlockFetch protocol retrieves blocks by hash from a peer node. It is used in node-to-node communication to fetch full block bodies after discovering headers via ChainSync.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `block-fetch` |
| Protocol ID | `3` |
| Mode | Node-to-Node |

## State Machine

```
┌──────┐  RequestRange   ┌──────┐
│ Idle │ ───────────────►│ Busy │
└──┬───┘                 └──┬───┘
   │                        │
   │ ClientDone             │ StartBatch
   │                        │ NoBlocks
   │                        │
   ▼                        ▼
┌──────┐              ┌───────────┐
│ Done │◄─────────────│ Streaming │◄───┐
└──────┘              └─────┬─────┘    │
                            │          │
                            │ Block    │
                            └──────────┘
                            │
                            │ BatchDone
                            ▼
                       ┌──────┐
                       │ Idle │
                       └──────┘
```

## States

| State | ID | Agency | Description |
|-------|-----|--------|-------------|
| **Idle** | 1 | Client | Waiting for block range request |
| **Busy** | 2 | Server | Processing range request |
| **Streaming** | 3 | Server | Streaming blocks to client |
| **Done** | 4 | None | Terminal state |

## Messages

| Message | Type ID | Direction | Description |
|---------|---------|-----------|-------------|
| `RequestRange` | 0 | Client → Server | Request blocks in range |
| `ClientDone` | 1 | Client → Server | Terminate protocol |
| `StartBatch` | 2 | Server → Client | Begin streaming blocks |
| `NoBlocks` | 3 | Server → Client | No blocks available for range |
| `Block` | 4 | Server → Client | Single block in batch |
| `BatchDone` | 5 | Server → Client | End of block batch |

## State Transitions

### From Idle (Client Agency)
| Message | New State |
|---------|-----------|
| `RequestRange` | Busy |
| `ClientDone` | Done |

### From Busy (Server Agency)
| Message | New State |
|---------|-----------|
| `StartBatch` | Streaming |
| `NoBlocks` | Idle |

### From Streaming (Server Agency)
| Message | New State |
|---------|-----------|
| `Block` | Streaming |
| `BatchDone` | Idle |

## Timeouts

| State | Timeout | Description |
|-------|---------|-------------|
| Busy | 60 seconds | Server must start batch or respond no blocks |
| Streaming | 60 seconds | Server must send next block in batch |

## Limits

| Limit | Value | Description |
|-------|-------|-------------|
| Max Recv Queue Size | 512 | Maximum receive queue messages |
| Default Recv Queue Size | 384 | Default queue size |
| Streaming Max Pending Bytes | 2.5 MB | Max pending bytes in Streaming state |
| Idle/Busy Max Pending Bytes | 64 KB | Max pending bytes in Idle/Busy states |

## Configuration Options

```go
blockfetch.NewConfig(
    blockfetch.WithBlockFunc(blockCallback),
    blockfetch.WithBlockRawFunc(blockRawCallback),
    blockfetch.WithBatchDoneFunc(batchDoneCallback),
    blockfetch.WithRequestRangeFunc(requestRangeCallback),
    blockfetch.WithBatchStartTimeout(5 * time.Second),
    blockfetch.WithBlockTimeout(60 * time.Second),
    blockfetch.WithRecvQueueSize(384),
)
```

## Usage Example

```go
// Request a range of blocks
startPoint := Point{Slot: 1000, Hash: startHash}
endPoint := Point{Slot: 2000, Hash: endHash}

client.RequestRange(startPoint, endPoint)

// Blocks arrive via BlockFunc callback
// BatchDoneFunc called when range complete
```

## Block Format

Blocks are wrapped in CBOR with a type identifier:

```go
type WrappedBlock struct {
    Type     uint   // Block type identifier (era)
    RawBlock []byte // Raw CBOR block data
}
```

## Notes

- Used in conjunction with ChainSync (headers) + BlockFetch (bodies)
- Blocks are streamed in order from start to end point
- Large receive queue supports high-throughput block streaming
- The Streaming state has a higher pending byte limit for efficiency
