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

**Enforcement:**
- Client-side pipeline tracking with disconnect on violation
- Configuration validation with panic on invalid values
- Server-side queue size limits enforced by protocol framework
- Per-state pending message byte limits enforced with connection teardown on violation

**Files modified:**
- `protocol/chainsync/chainsync.go` - Added constants, validation, and documentation
- `protocol/chainsync/client.go` - Added pipeline count tracking and enforcement

### BlockFetch Protocol

**Constants defined in `protocol/blockfetch/blockfetch.go`:**
- `MaxRecvQueueSize = 512` - Maximum size of the receive message queue
- `DefaultRecvQueueSize = 256` - Default receive queue size
- `MaxPendingMessageBytes = 5242880` - Maximum pending message bytes (5MB)

**Enforcement:**
- Configuration validation with panic on invalid values
- Queue size limits enforced by protocol framework
- Per-state pending message byte limits enforced with connection teardown on violation

**Files modified:**
- `protocol/blockfetch/blockfetch.go` - Added constants, validation, and documentation

### TxSubmission Protocol

**Constants defined in `protocol/txsubmission/txsubmission.go`:**
- `MaxRequestCount = 65535` - Maximum number of transactions per request (uint16 limit)
- `MaxAckCount = 65535` - Maximum number of transaction acknowledgments (uint16 limit)
- `DefaultRequestLimit = 1000` - Reasonable default for transaction requests
- `DefaultAckLimit = 1000` - Reasonable default for transaction acknowledgments
- Pending message byte limits: Not enforced (0 = no limit)

**Enforcement:**
- Server-side validation with disconnect on excessive request counts
- Client-side validation with disconnect on excessive received counts

**Files modified:**
- `protocol/txsubmission/txsubmission.go` - Added constants and documentation
- `protocol/txsubmission/server.go` - Added request count validation
- `protocol/txsubmission/client.go` - Added received count validation

## Protocol Violation Errors

**New error types defined in `protocol/error.go`:**
- `ErrProtocolViolationQueueExceeded` - Message queue limit exceeded
- `ErrProtocolViolationPipelineExceeded` - Pipeline limit exceeded  
- `ErrProtocolViolationRequestExceeded` - Request count limit exceeded
- `ErrProtocolViolationInvalidMessage` - Invalid message received

These errors cause connection termination as per the network specification.

## Other Mini-Protocols

The following protocols were evaluated and determined not to need additional queue limits:
- **KeepAlive** - Simple ping/pong protocol with minimal state
- **LocalStateQuery** - Request-response protocol with no pipelining
- **LocalTxSubmission** - Simple request-response for single transaction submission

## Validation and Testing

**Test file:** `protocol/limits_test.go`
- Validates that all limits are properly defined and positive
- Tests configuration validation and panic behavior  
- Verifies protocol violation errors are defined
- Ensures default values are reasonable and within limits

## Behavior Changes

**Before:**
- No enforced limits on pipeline depth or queue sizes
- Potential for memory exhaustion from excessive pipelining
- No disconnect on protocol violations

**After:**
- Strict limits enforced as per network specification
- Automatic connection termination on limit violations
- Comprehensive logging of violations before disconnect
- Configuration validation prevents invalid setups

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