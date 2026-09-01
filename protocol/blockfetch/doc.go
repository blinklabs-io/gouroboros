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

// Package blockfetch implements the Ouroboros block-fetch mini-protocol.
//
// # AI Navigation Guide
//
// This package is a good reference for understanding the protocol package structure.
// All protocol packages follow similar patterns.
//
// # Key Files
//
//   - blockfetch.go: Protocol definition with ProtocolName, ProtocolId, StateMap
//   - client.go: Client implementation for requesting blocks
//   - server.go: Server implementation for serving blocks
//   - messages.go: Message types with CBOR encoding
//
// # Protocol Structure Pattern
//
// All mini-protocol packages follow this structure:
//
//  1. {protocol}.go defines:
//     - ProtocolName, ProtocolId constants
//     - StateMap for state machine transitions
//     - Protocol struct embedding protocol.Protocol
//
//  2. client.go provides:
//     - Client struct with connection handling
//     - NewClient() constructor
//     - Request methods (e.g., RequestBlock, RequestRange)
//
//  3. server.go provides:
//     - Server struct
//     - Handler registration
//     - Response methods
//
//  4. messages.go defines:
//     - Message type constants
//     - Message structs with CBOR tags
//     - Constructor functions (NewMsg*)
//
// # State Machine
//
// BlockFetch states: Idle -> Busy -> Streaming -> Idle
//
// Client requests a range of blocks, server streams them back.
package blockfetch
