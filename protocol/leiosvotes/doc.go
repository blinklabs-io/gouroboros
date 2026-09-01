// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package leiosvotes implements the Leios vote diffusion mini-protocol.
//
// # AI Navigation Guide
//
// This package implements the CIP-0164 vote diffusion protocol for
// propagating votes on endorser blocks between node-to-node peers.
//
// # Key Files
//
//   - leiosvotes.go: Protocol definition, state machine, and config
//   - messages.go: Request, vote, and done message types with CBOR encoding
//   - client.go: Client implementation for requesting and processing votes
//   - server.go: Server implementation for serving requested vote batches
//
// # State Machine
//
// The protocol follows bounded push semantics:
//
//	Idle -> Busy -> Idle
//
// The client sends MsgVotesRequestNext(N) from Idle. The server then has
// agency and sends exactly N MsgVote messages. The first N-1 votes keep the
// protocol in Busy, and the final vote returns it to Idle. MsgDone terminates
// the protocol from Idle.
//
// # Vote Shape
//
// Votes contain the slot number, endorser block hash, voter ID, and vote
// signature. This matches the current CIP-0164 stake-based committee direction,
// where every committee member has the same uniform vote structure.
package leiosvotes
