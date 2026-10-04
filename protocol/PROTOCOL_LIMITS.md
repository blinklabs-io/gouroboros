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

## Connection read buffer allowance

`MaxReadBufferSize` bounds one mini-protocol in one role. A full-duplex
node-to-node connection runs eight mini-protocols in both roles, so sixteen
read loops each hold their own buffer.

The muxer carries a connection-wide allowance for those buffers. Each
protocol raises it to its own effective `MaxReadBufferSize` as it registers,
so the allowance is the largest registered cap (16 MB by default), not the
sum of all of them. A mini-protocol whose reassembly would take the
connection past the allowance fails with a
`connection read buffer budget exhausted` error rather than growing; the
allowance is returned as messages are consumed and when a read loop exits.

The largest message any node-to-node mini-protocol admits is bounded by its
own pending-message byte limit, and the largest of those is Block Fetch's
2,500,000 bytes, so one protocol's 16 MB cap covers every legitimate
reassembly on the connection with room to spare. A caller that needs more
headroom raises `MaxReadBufferSize` on the protocol that needs it, which
raises the connection allowance with it.

## Muxer ingress limits

The muxer reads every mini-protocol on a connection through one read loop,
so it never waits for a protocol's consumer. Each protocol role has its own
ingress queue between the socket and the protocol, and its own limit on the
segment payload held there. Ingress past the limit stops the connection with
`muxer.ErrIngressOverflow` rather than pausing the read loop, which would
stall every other protocol on the connection, keep-alive included. The one
exception is a protocol role with backpressure enabled
(`Muxer.SetIngressBackpressure`): its ingress past the limit pauses the read
loop until the protocol has made room. Only the Block Fetch client enables
it, and only while an unestimated request is outstanding.

A protocol's limit is its `ProtocolConfig.IngressLimit` when set. Otherwise
it is derived from its state map: the largest `PendingMessageByteLimit` of
any state (for the node-to-node protocols, the reference implementation's
per-protocol ingress queue limit), or the protocol's effective
`MaxReadBufferSize` when no state declares one, and never less than one batch
of ten maximum-size segments (655,350 bytes). The floor exists because the
queue sits in front of the protocol's own reassembly and pending-message
buffers, so it briefly holds segments a protocol that is keeping up has not
taken yet.

| Protocol role | Limit |
| --- | ---: |
| Block Fetch client | 23,068,694, raised while requests are outstanding |
| Block Fetch server | 2,500,000 |
| Tx Submission | 721,424 |
| Chain Sync (node-to-node), Peer Sharing | 655,350 |
| Keep Alive | 655,350 (floor; state limit 65,535) |
| Handshake | 655,350 (floor; state limit 5,760) |
| Local protocols | effective `MaxReadBufferSize` (16 MB) |

The worst-case memory a connection can queue is the sum of the limits of its
registered protocol roles; a queue only fills while its protocol is not
taking segments.

Block Fetch is the only node-to-node protocol whose peer sends an amount
chosen by the local side. Its client limit is the reference
`blockFetchProtocolLimits` value, and the client raises it to cover every
outstanding request with a size estimate, plus one maximum-size message for
batch framing, from before the request is sent until it is resolved. See
Block Fetch below for how a request is sized.

The muxer reports each queue's depth on every enqueue and dequeue, how long
each delivery waited for its protocol to take it, and how long the read loop
paused under backpressure, through the optional `muxer.Metrics` hooks set
with `Muxer.SetMetrics`.

## Muxer socket deadlines

The muxer sets a 120-second write deadline immediately before each segment
write, after acquiring its connection-wide send lock. A deadline-setting error
is returned without attempting the write. The deadline bounds the socket write,
not time spent waiting for the send lock or the protocol's outbound queue.

Read deadlines remain independently managed per segment. Connection wrappers
must preserve those read deadlines and support `SetWriteDeadline`. The muxer
replaces an existing write deadline; wrappers that enforce a shorter external
deadline must cap the requested value. Stopping the muxer closes the underlying
connection to interrupt pending socket operations.

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
| Total expected in-flight request bytes | — | 9,011,200 (100 × 88 KiB) |

`WithRecvQueueSize` rejects values outside the receive-queue range. The
in-flight byte bound applies to client request pipelining and blocks the
request caller when the bound is full; it does not terminate the connection.

A range request with `RangeRequest.ExpectedBytes` set counts toward the
client's ingress limit with the estimate plus 10%, and against the in-flight
bound with the estimate; the estimate should cover the whole serialized
blocks, headers included. A range larger than the in-flight bound is sent
once no other request is outstanding.

A request without an estimate (every `GetBlock` and `GetBlockRange`
request, and `RequestRange` with no estimate) has no bound that is both
safe and never refuses an honest peer, because a range's reply is bounded
per block, not per range. It counts as 88 KiB against the in-flight bound
and adds nothing to the ingress limit, and while it is outstanding the
client enables backpressure: past the limit, the muxer stops reading the
connection until the block consumer makes room. Memory stays within the
limit and the peer is slowed rather than dropped, but the pause holds up
every protocol on the connection, so a consumer slower than the keep-alive
timeout can still lose the connection. A peer that sends more than was
asked for is refused once the client's range checks reach the excess,
or with `muxer.ErrIngressOverflow` if the muxer still holds excess when that
request fails and its backpressure ends.

## Transaction Submission

| State | Timeout | Pending bytes |
| --- | ---: | ---: |
| Init | none | 721,424 |
| Idle | none | 721,424 |
| TxIdsBlocking | none | 721,424 |
| TxIdsNonBlocking | 10 seconds | 721,424 |
| Txs | 10 seconds | 721,424 |
| Done | none | 721,424 |

`MaxPendingMessageBytes` is 721,424 bytes: `MaxUnackedTxIds` (10) maximum-size
transactions of `MaxTxSizeBytes` (65,540) plus the `TxIdReplyEntryBytes` (44)
tx-id reply entry that announced each, with a 10% safety margin. It matches
the tx-submission mux ingress limit the reference implementation enforces, so
a conforming peer never exceeds it.

Both requests are bounded by the outstanding window, not by the `uint16` wire
ranges. `MaxRequestCount` and `MaxAckCount` (65,535) describe the ranges of the
`MsgRequestTxIds` count fields and no longer bound either request path;
`MaxUnackedTxIds` (10) does, and it is also what sizes the byte limit.

A request for transaction IDs must leave the peer inside that window. The
client's `MsgRequestTxIds` handler rejects an acknowledgement larger than what
it has outstanding, and rejects a request where `unacknowledged - ack + req`
exceeds `MaxUnackedTxIds`; `Server.RequestTxIds` applies the same condition
before putting a request on the wire. Both return
`ErrProtocolViolationRequestExceeded`. This is the reference implementation's
condition in `Ouroboros.Network.TxSubmission.Outbound`, which throws
`ProtocolErrorAckedTooManyTxids` and `ProtocolErrorRequestedTooManyTxids`
respectively.

A request for transaction bodies is bounded the same way: `Server.RequestTxs`
and the client's request handler both reject more than `MaxUnackedTxIds`
transaction IDs. A peer may only request transactions it has left
unacknowledged, and a reply to a larger request cannot fit
`MaxPendingMessageBytes`, which is derived from that same window.

Without these bounds a peer requesting 65,535 transaction IDs draws a reply of
roughly 2.6 MB, which `Protocol.enqueueMessage` refuses against
`MaxPendingMessageBytes` and then fails the protocol over, dropping the
connection.

`DefaultRequestLimit` and `DefaultAckLimit` are exported guidance constants
(1,000). They are not configuration fields, are not applied automatically, and
are larger than `MaxUnackedTxIds`: a caller using either as a request count is
refused.

## Handshake

Both state maps bound the pending message bytes of the Propose and Confirm
states at 5,760 bytes (`handshake.MaxPendingMessageBytes`), the reference
implementation's `byteLimitsHandshake` (4 x 1440). A larger message is refused
before it is decoded, so a peer that has not completed the handshake cannot
make the node buffer and decode a message up to the read-buffer cap.

For N2N, `Propose` and `Confirm` each have a 10-second timeout. The framework
does not arm the initial state's timer by default; the N2N handshake server
opts into its configured `Propose` timeout. N2C has no state timeouts and does
not opt in. After the initial transition, `Confirm` uses the normal state
timeout. Client and server instances copy the N2N map and can override the
applicable timeout with `WithTimeout`; the N2C map remains timeout-free.

## Keep Alive

Both active states bound pending message bytes at 65,535
(`keepalive.MaxPendingMessageBytes`), the reference implementation's
`byteLimitsKeepAlive`.

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

The local protocols have no additional queue, pipeline, or pending-message
byte limits in their state maps.

## Peer Sharing

The map (`protocol/peersharing/peersharing.go`) is:

| State | Timeout | Pending bytes |
| --- | ---: | ---: |
| Idle | none | 5,760 |
| Busy | 60 seconds | 5,760 |
| Done | none | none |

`MaxPendingMessageBytes` is 5,760 bytes, the reference implementation's
`peerSharingProtocolLimits` ingress queue and `byteLimitsPeerSharing` per-state
limit (4 x 1440, one TCP initial congestion window). The Busy timeout is
configurable via `WithTimeout`.

The server clamps a `SharePeers` reply to the smaller of the requested
`Amount` and `MaxSharedPeers` (230), the largest number of addresses that
still encodes within `MaxPendingMessageBytes`. The client rejects a reply
carrying more addresses than it requested.

## Leios mini-protocols

| Protocol | State | Default timeout | Pending bytes |
| --- | --- | ---: | ---: |
| Leios Fetch client | Votes | 5 seconds | none |
| Leios Fetch client | BlockRange | 5 seconds | 64 MiB |
| Leios Notify client | Busy | 60 seconds | none |
| Leios Votes client | Busy | 60 seconds | none |
| Leios Votes server with a configured request callback | Busy | 60 seconds | none |

Leios Fetch `Block` and `BlockTxs` requests are bounded by the caller's
context, not by a protocol state timeout. Leios Notify has no pending-message
byte limit. Leios Fetch `BlockRangeRequest` retains at most 1,000 response
messages and 64 MiB of encoded response data per request by default; both
limits can be configured. The byte limit also bounds pending range messages in
the protocol receive queue. Exceeding either retained-response limit is a
protocol error and closes the connection. Leios Notify allows up to 100
pipelined requests and defaults to 10. Leios Votes allows up to 100 pipelined
requests, defaults to 1, and limits one request to 1,000 votes (the default is
also 1,000). Leios Notify limits each VotesOffer to 1,000 entries and 256 KiB
before parsing the vote list. Nonpositive Leios Fetch range limits select the
defaults; other invalid configured values are rejected by their constructors.

## Peras Vote Diffusion

| State | Timeout | Pending bytes |
| --- | ---: | ---: |
| Init, Idle, ObjectIDsBlocking | none | `maxObjectsUnacknowledged × 1,100 + 256` |
| ObjectIDsNonBlocking, Objects | 5 seconds by default | `maxObjectsUnacknowledged × 1,100 + 256` |
| Done | none | none |

The outstanding object window defaults to 50 and can be configured up to
1,000. The timeout is configurable and bounds non-blocking ID and object
requests; blocking ID requests wait for an available vote.

## Enforcement scope

State-map timeouts and pending-message limits are enforced by the protocol
framework. Message-specific limits and configuration validation are enforced
by the owning protocol implementation. This document intentionally does not
claim limits for protocols or states whose current map contains no such entry.

## CBOR allocation budgets

Encoded-message admission and decoded allocation use different bounds. The
protocol framework's effective read-buffer allowance is 16 MiB by default and
can be raised by the caller. State-map limits listed above apply before message
decoding. A response containing an entire ledger-state map uses the normal
mode's 10,000,000-element collection allowance. The strict mode's 131,072-element
allowance applies when a caller explicitly chooses it. Both modes enforce their
own nesting and collection limits on custom `Value` destinations before tree
construction. Prefix decoding retains only the consumed item's bytes and leaves
subsequent items available to the caller.

| Message family | Encoded allowance | Decoded allocation contract |
| --- | --- | --- |
| Handshake proposal/acceptance | 5,760 bytes | Version maps pass the selected CBOR mode's pre-allocation validation. |
| Keep Alive | 65,535 bytes | Cookies and message discriminants have fixed scalar shapes. |
| Local State Query query/result | Effective read-buffer allowance | Query input sets have their own 10,000-item request bound. Result maps retain the normal decoder's larger allowance, including whole-UTxO queries. Raw result storage grows with the actual encoded payload. |
| Local Tx Monitor next-transaction reply | Effective read-buffer allowance | Transaction bytes and the optional reply envelope are validated before typed decoding. |
| Local Tx Submission submit/rejection | Effective read-buffer allowance | Transaction bytes and raw rejection data are retained from validated input. |
| Leios Fetch block, transaction and vote replies | Effective read-buffer allowance per message; range retention defaults to 64 MiB and 1,000 messages | Raw-item collections validate encoded items before allocation. Range retention is enforced by the client separately. |
| Leios Notify vote offers | 256 KiB and 1,000 votes | Definite counts and indefinite entries are checked before the vote list is allocated. Other notifications use the configured pending-byte allowance. |
| Leios Votes vote reply | Effective read-buffer allowance | Typed fields use the normal CBOR mode; request counts are independently limited to 1,000. |
| Peras vote IDs/objects | `maxObjectsUnacknowledged × 1,100 + 256` bytes | Lists are checked against the supported outstanding-window maximum. Fixed vote envelopes are checked before opaque bytes are retained. |
| Leios endorser-block references | At least 35 encoded bytes per valid reference | A hash32 uses a two-byte header and 32 payload bytes; size uses at least one byte. Counts exceeding available encoded entries are rejected before allocation. Collections grow as entries validate. |

Typed CBOR decoders validate definite and indefinite collection claims before
materializing them. Seven-byte truncated local-query, local-transaction, Leios
and Peras reply vectors exercise this admission separately from healthy large
responses. A whole-UTxO response with 131,073 valid outputs occupies about
9 MiB and remains accepted. Collection policies retain historical duplicate-map
behavior and optional set tags in their owning ledger eras.

Diagnostic tree construction has a separate inspection budget. Its default
retained-byte and work allowances are 128 MiB, eight times `Diagnose`'s 16 MiB
input allowance. This accommodates owned input, growing decoder scratch and
copied string payloads. Node admission reserves eight node-sized storage units
plus 512 bytes of scalar-decoder scratch. Node and cumulative collection-item
defaults are derived from that reservation. Array entries, map pairs and
indefinite-string chunks share one collection counter. Input visits, node
visits and payload-copy work share one work counter. Tag-24 block wrappers and
their embedded trees spend the same operation budget.

`DiagnosticOptions.ParseLimits` and `ParseDiagnosticWithLimits` configure these
inspection budgets. Zero fields choose the documented defaults; negative fields
are rejected. `MaxDepth`, `MaxArrayItems` and `MaxByteLength` control rendering.
Every raw span is a capacity-bounded, read-only view into one owned input buffer;
copy it before modification. String values and indefinite-string concatenation
are charged separately before copying.

The shared block corpus spans Byron through Conway. Its largest diagnostic
case has 17,943 encoded bytes, 1,543 nodes and depth 18. Construction budgets
cover the corpus and bound hostile scalar arrays, maps, nested tags and string
chunks without imposing inspection budgets on ledger validation.

The repository's `build-examples` workflow runs `make build` on pull requests;
that target builds every program under `examples/` against the public API
using the root module's dependencies.
