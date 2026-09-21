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

// Package perasvotes reserves the Ouroboros mini-protocol number for the
// Peras vote-diffusion mini-protocol (CIP-0140).
//
// CIP-0140: https://cips.cardano.org/cip/CIP-0140
//
// This package intentionally contains no protocol state machine, message
// types, or codec: implementing the client/server handlers and wire format
// is tracked separately. Its sole purpose is to hold the reserved
// node-to-node mini-protocol number so that no other mini-protocol in this
// module is later assigned the same number.
package perasvotes

const (
	ProtocolName = "peras-vote-diffusion"

	// ProtocolId is the reserved node-to-node mini-protocol number for
	// Peras vote diffusion. It matches the reference implementation's
	// perasVoteDiffusionMiniProtocolNum in IntersectMBO/ouroboros-network
	// (cardano-diffusion/lib/Cardano/Network/NodeToNode.hs), which is the
	// implementation vehicle for the Tweag cardano-peras design
	// (https://github.com/tweag/cardano-peras). The adjacent number 16 is
	// likewise reserved there for Peras certificate diffusion
	// (perasCertDiffusionMiniProtocolNum), tracked separately from this
	// issue.
	ProtocolId uint16 = 17
)
