# Ouroboros Mini-Protocol Limits Implementation

## Overview

This document describes the implementation of queue/pipeline/message limits for the Ouroboros mini-protocols as specified in the Ouroboros Network Specification. These limits prevent resource exhaustion and ensure protocol compliance by terminating connections when limits are violated.

## Reference

All limits are based on the [Ouroboros Network Specification](https://ouroboros-network.cardano.intersectmbo.org/pdfs/network-spec/network-spec.pdf).

## Implemented Limits

### ChainSync Protocol

**Constants defined in `protocol/chainsync/chainsync.go`:**
- `MaxPipelineLimit = 100` - Maximum number of pipelined ChainSync requests
- `MaxRecvQueueSize = 100` - Maximum size of the receive message queue
- `DefaultPipelineLimit = 50` - Conservative default for pipeline limit
- `DefaultRecvQueueSize = 50` - Conservative default for receive queue size
- `MaxPendingMessageBytes = 102400` - Maximum pending message bytes (100KB)

**State timeout constants:**
- `IdleTimeout = 60s` - Timeout for client to send next request
- `CanAwaitTimeout = 300s` - Timeout for server to provide next block or await
- `IntersectTimeout = 5s` - Timeout for server to respond to intersect request
- `MustReplyTimeout = 300s` - Timeout for server to provide next block

**Enforcement:**
- Client-side pipeline tracking with disconnect on violation
- Configuration validation with panic on invalid values
- Server-side queue size limits enforced by protocol framework
- Per-state pending message byte limits enforced with connection teardown on violation
- State transition timeouts enforced with connection teardown on timeout

**Files modified:**
- `protocol/chainsync/chainsync.go` - Added constants, validation, and documentation
- `protocol/chainsync/client.go` - Added pipeline count tracking and enforcement

### BlockFetch Protocol

**Constants defined in `protocol/blockfetch/blockfetch.go`:**
- `MaxRecvQueueSize = 512` - Maximum size of the receive message queue
- `DefaultRecvQueueSize = 256` - Default receive queue size
- `MaxPendingMessageBytes = 5242880` - Maximum pending message bytes (5MB)

**State timeout constants:**
- `IdleTimeout = 60s` - Timeout for client to send block range request
- `BusyTimeout = 5s` - Timeout for server to start batch or respond no blocks
- `StreamingTimeout = 60s` - Timeout for server to send next block in batch

**Enforcement:**
- Configuration validation with panic on invalid values
- Queue size limits enforced by protocol framework
- Per-state pending message byte limits enforced with connection teardown on violation
- State transition timeouts enforced with connection teardown on timeout

**Files modified:**
- `protocol/blockfetch/blockfetch.go` - Added constants, validation, and documentation

### TxSubmission Protocol

**Constants defined in `protocol/txsubmission/txsubmission.go`:**
- `MaxRequestCount = 65535` - Maximum number of transactions per request (uint16 limit)
- `MaxAckCount = 65535` - Maximum number of transaction acknowledgments (uint16 limit)
- `DefaultRequestLimit = 1000` - Reasonable default for transaction requests
- `DefaultAckLimit = 1000` - Reasonable default for transaction acknowledgments
- Pending message byte limits: Not enforced (0 = no limit)

**State timeout constants:**
- `InitTimeout = 30s` - Timeout for client to send init message
- `IdleTimeout = 300s` - Timeout for server to send tx request when idle
- `TxIdsBlockingTimeout = 60s` - Timeout for client to reply with tx IDs (blocking)
- `TxIdsNonblockingTimeout = 30s` - Timeout for client to reply with tx IDs (non-blocking)
- `TxsTimeout = 120s` - Timeout for client to reply with full transactions

**Enforcement:**
- Server-side validation with disconnect on excessive request counts
- Client-side validation with disconnect on excessive received counts
- State transition timeouts enforced with connection teardown on timeout

**Files modified:**
- `protocol/txsubmission/txsubmission.go` - Added constants and documentation
- `protocol/txsubmission/server.go` - Added request count validation
- `protocol/txsubmission/client.go` - Added received count validation

### Handshake Protocol

**State timeout constants:**
- `ProposeTimeout = 5s` - Timeout for client to propose protocol version
- `ConfirmTimeout = 5s` - Timeout for server to confirm or refuse version

**Files modified:**
- `protocol/handshake/handshake.go` - Added timeout constants and StateMap integration

### Keepalive Protocol

**State timeout constants:**
- `ClientTimeout = 60s` - Timeout for client to send keepalive request
- `ServerTimeout = 10s` - Timeout for server to respond to keepalive

**Files modified:**
- `protocol/keepalive/keepalive.go` - Added timeout constants and StateMap integration

## Protocol Violation Errors

**New error types defined in `protocol/error.go`:**
- `ErrProtocolViolationQueueExceeded` - Message queue limit exceeded
- `ErrProtocolViolationPipelineExceeded` - Pipeline limit exceeded  
- `ErrProtocolViolationRequestExceeded` - Request count limit exceeded
- `ErrProtocolViolationInvalidMessage` - Invalid message received

These errors cause connection termination as per the network specification.

## Other Mini-Protocols

All remaining protocols have appropriate timeout implementations:

- **LocalStateQuery** - Has AcquireTimeout (5s) and QueryTimeout (180s) for database queries
- **LocalTxMonitor** - Has AcquireTimeout (5s) and QueryTimeout (30s) for mempool monitoring  
- **LocalTxSubmission** - Has Timeout (30s) for local transaction submission
- **PeerSharing** - Has Timeout (5s) for peer discovery requests
- **LeiosFetch** - Has Timeout (5s) for Leios block/transaction/vote fetching
- **LeiosNotify** - Has Timeout (60s) for Leios block/vote notifications
- **Handshake** - Has ProposeTimeout (5s) and ConfirmTimeout (5s) for protocol negotiation
- **Keepalive** - Has ClientTimeout (60s) and ServerTimeout (10s) for connection health

## Validation and Testing

**Test file:** `protocol/limits_test.go`
- Validates that all limits are properly defined and positive
- Tests configuration validation and panic behavior  
- Verifies protocol violation errors are defined
- Ensures default values are reasonable and within limits
- Comprehensive timeout validation for all 11 mini-protocols
- Verifies StateMap entries use correct timeout constants

## Protocol State Timeouts

### Implementation

Each protocol state can define a timeout value that is enforced by the protocol framework. When a state transition takes too long, the connection is automatically terminated to prevent hanging connections and ensure protocol compliance.

### Timeout Values

The timeout values are based on the Ouroboros Network Specification and real-world network conditions:

- **Short timeouts (5-30s)**: For rapid protocol handshakes and responses
- **Medium timeouts (60-120s)**: For normal message exchanges and client requests  
- **Long timeouts (300s)**: For waiting on new blocks or mempool queries

### Timeout Behavior

- Timeouts are set when entering a state with `StateMapEntry.Timeout > 0`
- If no state transition occurs within the timeout period, the protocol terminates
- Connection teardown includes proper error logging for debugging
- Terminal states (`AgencyNone`) do not have timeouts

## Behavior Changes

**Before:**
- No enforced limits on pipeline depth or queue sizes
- Potential for memory exhaustion from excessive pipelining
- No disconnect on protocol violations
- No state transition timeouts

**After:**
- Strict limits enforced as per network specification
- Automatic connection termination on limit violations
- Comprehensive logging of violations before disconnect
- Configuration validation prevents invalid setups
- State transition timeouts prevent hanging connections

## Usage Examples

### ChainSync with Custom Limits

```go
cfg := chainsync.NewConfig(
    chainsync.WithPipelineLimit(75),        // Max 100
    chainsync.WithRecvQueueSize(80),        // Max 100
)
```

### BlockFetch with Custom Queue Size

```go
cfg := blockfetch.NewConfig(
    blockfetch.WithRecvQueueSize(400),      // Max 512
)
```

### TxSubmission (limits enforced automatically)

The TxSubmission protocol enforces limits automatically in the client and server message handlers.

## Network Specification Compliance

This implementation ensures compliance with the Ouroboros Network Specification by:
1. Defining appropriate limits for each mini-protocol
2. Enforcing limits at both client and server sides
3. Terminating connections on protocol violations
4. Preventing resource exhaustion attacks
5. Maintaining protocol state machine integrity