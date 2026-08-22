# BlockFetch Protocol

The BlockFetch protocol retrieves blocks by hash from a peer node. It is used in node-to-node communication to fetch full block bodies after discovering headers via ChainSync.

## Protocol Identifiers

| Property | Value |
|----------|-------|
| Protocol Name | `block-fetch` |
| Protocol ID | `3` |
| Mode | Node-to-Node |

## State Machine

```text
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
| Busy Max Pending Bytes | 2.5 MB | Matches Streaming; a block can arrive before the state machine leaves Busy |
| Idle Max Pending Bytes | 64 KB | Only control messages are sent in Idle |
| Idle Max Pending Bytes (pipelining client) | 2.5 MB | A block for the next request can arrive while the state machine is momentarily back in Idle |
| Default Max In-Flight Bytes | 9.0 MB | Expected size of outstanding pipelined requests: 100 x 88 KiB = 9,011,200 bytes, as in `cardano-node`'s `blockFetchProtocolLimits` |
| Default Request Expected Bytes | 88 KiB | Size assumed for a request whose caller gave no estimate (one maximum-size block body) |

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
    // Client request pipelining
    blockfetch.WithRequestPipelining(true),
    blockfetch.WithRangeDoneFunc(rangeDoneCallback),
    blockfetch.WithMaxInFlightBytes(blockfetch.DefaultMaxInFlightBytes),
)
```

## Usage Example

```go
// Request a range of blocks. One request is outstanding at a time; a second
// call blocks until the first batch completes.
startPoint := Point{Slot: 1000, Hash: startHash}
endPoint := Point{Slot: 2000, Hash: endHash}

if err := client.GetBlockRange(startPoint, endPoint); err != nil {
    return err
}

// Blocks arrive via BlockFunc/BlockRawFunc callback
// BatchDoneFunc called when range complete
```

## Request Pipelining

`MsgRequestRange` is the pipelined client message of this mini-protocol, while
`MsgBlock` is the server response that streams blocks back. `cardano-node` runs
up to 100 outstanding requests per peer. A client that waits for a batch to
finish before sending the next request idles the peer for a round trip at every
batch boundary.

A client configured with `RequestPipelining` keeps a FIFO queue of outstanding
`MsgRequestRange`. Responses are ordered, so queue position identifies the
request a `StartBatch`, `Block`, `NoBlocks`, or `BatchDone` belongs to; no wire
change is involved. `CallbackContext.RequestId` carries that identity to the
block callbacks, and `RangeDoneFunc` is called exactly once per request with
the terminal outcome.

```go
id, err := client.RequestRange(ctx, blockfetch.RangeRequest{
    Start:         startPoint,
    End:           endPoint,
    ExpectedBytes: expectedRangeBytes, // from the chain-sync header sizes
})
```

`RequestRange` returns as soon as the request is queued and sent, and blocks
only while admitting it would exceed `MaxInFlightBytes`. The bound is in bytes
rather than requests because consumer range sizes vary by orders of magnitude;
a caller that supplies no estimate is charged one maximum-size block body per
request, which reproduces `cardano-node`'s limit of 100 outstanding requests.

`RequestPipelining` is protocol request pipelining. It is unrelated to the
`Pipeline` option, which is a processing pipeline for blocks that have already
been received.

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
- Blocks are streamed in order from start to end point, and batches are
  answered in request order
- Large receive queue supports high-throughput block streaming
- The Streaming state has a higher pending byte limit for efficiency
