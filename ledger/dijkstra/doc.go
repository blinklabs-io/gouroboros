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
//	https://github.com/IntersectMBO/cardano-ledger/blob/c47305fcf47bd77437b837d0dfb9cb4181bfbc77/eras/dijkstra/impl/cddl/data/dijkstra.cddl
//
// When updating types to track upstream changes, bump the commit hash above so
// it always records the exact CDDL revision this package was validated against.
package dijkstra
