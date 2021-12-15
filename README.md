# go-ouroboros-network

A Go client implementation of the Cardano Ouroboros network protocol

This is loosely based on the [official Haskell implementation](https://github.com/input-output-hk/ouroboros-network)

## Implementation status

The Ouroboros protocol consists of a simple multiplexer protocol and various mini-protocols that run on top of it.
This makes it easy to implement only parts of the protocol without negatively affecting usability of this library.

The multiplexer and handshake mini-protocol are "fully" working. The focus will be on the node-to-client (local) protocols,
but the node-to-node protocols will also be implemented in time.

### Mini-protocols

| Name | Status |
| --- | --- |
| Handshake | Implemented |
| Chain-Sync | In Progress |
| Block-Fetch | Not Implemented |
| TxSubmission | Not Implemented |
| Local TxSubmission | Not Implemented |
| Local State Query | Not Implemented |
| Keep-Alive | Not Implemented |

