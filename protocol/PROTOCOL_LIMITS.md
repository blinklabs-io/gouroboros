# Ouroboros mini-protocol limits

This document records the limits and state-transition timeouts implemented by
the protocol state maps in this repository. Values are source-level defaults;
an option passed to a client or server can replace the timeout where noted.
An entry with a zero timeout or pending-message limit has no framework limit.

The protocol framework applies a state's `PendingMessageByteLimit` while that
state is active. A state timeout closes the protocol when its agency does not
make the expected transition before the timeout expires. These are transport
and state-machine safeguards, not application-level transaction or block
validation.

## Chain Sync

The N2N map (`protocol/chainsync/chainsync.go`) has the following limits:

| State | Timeout | Pending bytes |
| --- | ---: | ---: |
| Idle | 3673 seconds | 462,000 |
| CanAwait | 10 seconds | 462,000 |
| Intersect | 10 seconds | 462,000 |
| MustReply | random in `[135, 269)` seconds | 462,000 |
| Done | none | 462,000 |

The N2C map has no state timeouts or pending-message byte limits. `MustReply`
uses a fresh random timeout for each state entry; `MustReplyTimeout` is the
fixed maximum retained for compatibility and configuration defaults.

Configuration limits are:

| Setting | Maximum | Default |
| --- | ---: | ---: |
| Pipeline limit | 100 requests | 75 |
| Receive queue size | 100 messages | 75 |

`MaxPendingMessageBytes` is 462,000 bytes. `WithPipelineLimit` and
`WithRecvQueueSize` reject negative values and values above their maximum.

## Block Fetch

The base map (`protocol/blockfetch/blockfetch.go`) is:

| State | Timeout | Pending bytes |
| --- | ---: | ---: |
| Idle | none | 65,535 |
| Busy | 60 seconds | 2,500,000 |
| Streaming | 60 seconds | 2,500,000 |
| Done | none | 0 |

Client and server instances copy the map and apply their configured
`BatchStartTimeout` and `BlockTimeout` to Busy and Streaming. A pipelining
client raises its Idle pending-message limit to 2,500,000 bytes so a block can
arrive during the Idle transition.

| Setting | Maximum | Default |
| --- | ---: | ---: |
| Receive queue size | 512 messages | 384 |
| Expected bytes per unestimated range request | — | 90,112 (88 KiB) |
| Total expected in-flight request bytes | — | 9,011,200 (100 × 88 KiB) |

`WithRecvQueueSize` rejects values outside the receive-queue range. The
in-flight byte bound applies to client request pipelining and blocks the
request caller when the bound is full; it does not terminate the connection.

## Transaction Submission

The TxSubmission state map has no pending-message byte limits:

| State | Timeout |
| --- | ---: |
| Init | none |
| Idle | none |
| TxIdsBlocking | none |
| TxIdsNonBlocking | 10 seconds |
| Txs | 10 seconds |
| Done | none |

The protocol accepts at most 65,535 transaction IDs in a request and at most
65,535 acknowledgements (`uint16` wire fields). Both client and server reject
counts outside those bounds with `ErrProtocolViolationRequestExceeded`.
`DefaultRequestLimit` and `DefaultAckLimit` are exported guidance constants
(1,000); they are not configuration fields and are not applied automatically.

## Handshake

For N2N, `Propose` and `Confirm` each have a 10-second timeout. N2C has no
state timeouts. Client and server instances copy the N2N map and can override
the applicable timeout with `WithTimeout`; the N2C map remains timeout-free.

## Keep Alive

| State | Timeout |
| --- | ---: |
| Client | 97 seconds |
| Server | 60 seconds |
| Done | none |

The keep-alive configuration separately defaults to a 60-second period and a
10-second response timeout for the keep-alive loop. Those values do not change
the exported state-map constants above.

## Local mini-protocols

These protocols have no static state-map timeout; the client copies the map and
applies the configured operation timeout where indicated:

| Protocol | State/operation | Default |
| --- | --- | ---: |
| Local State Query | Acquiring | 5 seconds |
| Local State Query | Querying | 180 seconds |
| Local Tx Monitor | Acquiring | 5 seconds |
| Local Tx Monitor | Busy queries | 30 seconds |
| Local Tx Submission | Busy submit | 30 seconds |
| Peer Sharing | Busy response | 60 seconds |

The local protocols and Peer Sharing have no additional queue, pipeline, or
pending-message byte limits in their state maps.

## Leios mini-protocols

| Protocol | State | Default timeout |
| --- | --- | ---: |
| Leios Fetch client | Votes, BlockRange | 5 seconds |
| Leios Notify client | Busy | 60 seconds |
| Leios Votes client | Busy | 60 seconds |
| Leios Votes server with a configured request callback | Busy | 60 seconds |

Leios Fetch `Block` and `BlockTxs` requests are bounded by the caller's
context, not by a protocol state timeout. Leios Notify and Leios Fetch have no
queue or pending-message byte limits. Leios Notify allows up to 100 pipelined
requests and defaults to 10. Leios Votes allows up to 100 pipelined requests,
defaults to 1, and limits one request to 1,000 votes (the default is also
1,000). Invalid configured values are rejected by their constructors.

## Enforcement scope

State-map timeouts and pending-message limits are enforced by the protocol
framework. Message-specific limits and configuration validation are enforced
by the owning protocol implementation. This document intentionally does not
claim limits for protocols or states whose current map contains no such entry.

The repository's `build-examples` workflow runs `make build` on pull requests;
that target builds every module under `examples/` against the public API.
