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
//	https://github.com/IntersectMBO/cardano-ledger/blob/a24a2d69b6251d41bad96e53dbd40aabfe1bb25c/eras/dijkstra/impl/cddl/data/dijkstra.cddl
//
// This is the commit the respun ouroboros-leios prototype-2026w27 "musashi"
// testnet is built against (dated 2026-06-29, two days before the network's
// 2026-07-01 systemStart). At this revision block = [header, block_body] with a
// four-field block_body ([invalid_transactions, transactions, leios_certificate,
// peras_certificate]) and leios_certificate = [signers, aggregated_signature :
// bytes .size 48]. An earlier revision used a flat seven-element segwit block;
// this package tracks the two-element form the deployed network actually serves.
//
// A verbatim copy of that revision's CDDL is vendored at
// testdata/dijkstra.cddl for reference and diffing against upstream.
//
// When updating types to track upstream changes, bump the commit hash above,
// refresh testdata/dijkstra.cddl from the same revision, and keep them in sync
// so this package always records the exact CDDL it was validated against.
package dijkstra
