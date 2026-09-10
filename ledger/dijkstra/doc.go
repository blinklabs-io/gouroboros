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

// Package dijkstra implements the Dijkstra era, including the Leios
// extensions maintained on top of it by the Leios team.
//
// # CDDL Reference
//
// The wire formats in this package target the canonical Dijkstra CDDL
// maintained in IntersectMBO/cardano-ledger. The pinned version we aim to be
// compatible with is:
//
//	https://github.com/IntersectMBO/cardano-ledger/blob/1587f21a7d1306dc590c2749a5c66232ef66aad0/eras/dijkstra/impl/cddl/data/dijkstra.cddl
//
// This is the commit the respun ouroboros-leios prototype-2026w36 "musashi"
// testnet is built against. At this revision block = [header, block_body] with
// a three-field block_body ([transactions, leios_certificate, peras_certificate])
// and a four-field block_transaction carrying its validity flag.
//
// A verbatim copy of that revision's CDDL is vendored at
// testdata/dijkstra.cddl for reference and diffing against upstream.
//
// When updating types to track upstream changes, bump the commit hash above,
// refresh testdata/dijkstra.cddl from the same revision, and keep them in sync
// so this package always records the exact CDDL it was validated against.
package dijkstra
