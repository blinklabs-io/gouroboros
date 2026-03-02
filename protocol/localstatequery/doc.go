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

// Package localstatequery implements the Ouroboros local state query mini-protocol.
//
// # AI Navigation Guide
//
// Key files in this package:
//   - localstatequery.go: Protocol definition, state machine, constants
//   - queries.go: Query type definitions and CBOR encoding (start here for query work)
//   - client.go: Client-side query execution methods
//   - server.go: Server-side query handling
//   - messages.go: Protocol message types
//
// # State Machine
//
// The protocol follows this state flow:
//
//	Idle -> Acquiring -> Acquired -> Querying -> Acquired -> ...
//	                 \-> Idle (on release)
//
// # Known Incomplete Areas
//
// Several query types have partial implementations (see TODOs with issue references):
//   - #858: Protocol parameters need additional fields
//   - #860: Some query result types incomplete
//   - #863-867: Various query implementations pending
//
// # Common Patterns
//
// Queries are era-polymorphic. Check CurrentEra() before sending era-specific queries.
// Results decode into era-specific types based on the query type.
//
// # Example Usage
//
//	client, _ := localstatequery.NewClient(...)
//	client.Acquire(point)  // Acquire state at a specific chain point
//	result, _ := client.GetCurrentEra()
//	client.Release()
package localstatequery
