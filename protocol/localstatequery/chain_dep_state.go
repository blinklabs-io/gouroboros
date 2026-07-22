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

package localstatequery

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// ChainDepStateProtocol identifies which consensus protocol produced a
// DebugChainDepState result. The two protocols serialise the chain-dependent
// state with different on-wire layouts, distinguished by a leading version
// number, so callers can tell which optional fields are populated.
type ChainDepStateProtocol int

const (
	// ChainDepStateProtocolTPraos is the Transitional Praos protocol used by
	// the Shelley, Allegra, Mary and Alonzo eras (serialisation version 1).
	ChainDepStateProtocolTPraos ChainDepStateProtocol = 1
	// ChainDepStateProtocolPraos is the Praos protocol used by the Babbage
	// era and later, including Conway (serialisation version 0).
	ChainDepStateProtocolPraos ChainDepStateProtocol = 0
)

// chainDepState serialisation version tags, as emitted by
// Ouroboros.Consensus.Protocol.{Praos,TPraos} `encodeVersion`.
const (
	chainDepStateVersionPraos  uint64 = 0
	chainDepStateVersionTPraos uint64 = 1
)

// DebugChainDepStateResult is the typed result of a DebugChainDepState query
// (Shelley sub-query 13). It decodes the consensus chain-dependent state and
// exposes the authoritative on-chain operational-certificate counters that a
// synced node enforces when validating blocks.
//
// The wire layout is the `encodeVersion`-wrapped serialisation from
// ouroboros-consensus:
//
//	Praos  (Babbage+): [0, [lastSlot, opCertCounters, evolvingNonce,
//	                        candidateNonce, epochNonce, previousEpochNonce,
//	                        labNonce, lastEpochBlockNonce]]
//	TPraos (Shelley..Alonzo): [1, [lastSlot,
//	                        [opCertCounters, evolvingNonce, candidateNonce]]]
//
// OpCertCounters and the two shared nonces are present for both protocols; the
// remaining nonce fields are Praos-only and are nil when Protocol is TPraos.
type DebugChainDepStateResult struct {
	// Protocol is the consensus protocol the state was serialised for.
	Protocol ChainDepStateProtocol
	// LastSlot is the slot of the last block applied to the acquired state, or
	// Origin if no block has been applied yet.
	LastSlot WithOriginSlot
	// OpCertCounters maps each block-issuer key hash (a stake pool's cold-key
	// hash) to the highest operational-certificate issue number the chain has
	// accepted for it. This is the authoritative counter the node enforces: a
	// new block whose opcert issue number is lower is rejected.
	OpCertCounters map[ledger.Blake2b224]uint64
	// EvolvingNonce and CandidateNonce are present for both protocols.
	EvolvingNonce  lcommon.Nonce
	CandidateNonce lcommon.Nonce
	// EpochNonce, PreviousEpochNonce, LabNonce and LastEpochBlockNonce are
	// Praos-only (Babbage era and later); they are nil for TPraos results.
	EpochNonce          *lcommon.Nonce
	PreviousEpochNonce  *lcommon.Nonce
	LabNonce            *lcommon.Nonce
	LastEpochBlockNonce *lcommon.Nonce
}

// OpCertCounter returns the on-chain operational-certificate counter for a
// single block issuer (a stake pool's cold-key hash), and whether the chain
// has recorded a counter for it. A pool that has never minted a block under an
// operational certificate has no counter and returns (0, false).
func (r *DebugChainDepStateResult) OpCertCounter(
	poolKeyHash ledger.Blake2b224,
) (uint64, bool) {
	v, ok := r.OpCertCounters[poolKeyHash]
	return v, ok
}

func (r *DebugChainDepStateResult) UnmarshalCBOR(data []byte) error {
	// Both protocols wrap the state in `encodeVersion N`, which serialises as
	// a 2-element array [version, innerState]. The version selects the layout
	// of innerState.
	var outer struct {
		cbor.StructAsArray
		Version uint64
		Inner   cbor.RawMessage
	}
	if _, err := cbor.Decode(data, &outer); err != nil {
		return err
	}
	switch outer.Version {
	case chainDepStateVersionPraos:
		// Praos (Babbage era and later): an 8-element array.
		var p struct {
			cbor.StructAsArray
			LastSlot            WithOriginSlot
			OpCertCounters      map[ledger.Blake2b224]uint64
			EvolvingNonce       lcommon.Nonce
			CandidateNonce      lcommon.Nonce
			EpochNonce          lcommon.Nonce
			PreviousEpochNonce  lcommon.Nonce
			LabNonce            lcommon.Nonce
			LastEpochBlockNonce lcommon.Nonce
		}
		if _, err := cbor.Decode(outer.Inner, &p); err != nil {
			return err
		}
		r.Protocol = ChainDepStateProtocolPraos
		r.LastSlot = p.LastSlot
		r.OpCertCounters = p.OpCertCounters
		r.EvolvingNonce = p.EvolvingNonce
		r.CandidateNonce = p.CandidateNonce
		// Copy into fresh locals so the pointers do not alias the decode
		// scratch struct.
		epochNonce := p.EpochNonce
		previousEpochNonce := p.PreviousEpochNonce
		labNonce := p.LabNonce
		lastEpochBlockNonce := p.LastEpochBlockNonce
		r.EpochNonce = &epochNonce
		r.PreviousEpochNonce = &previousEpochNonce
		r.LabNonce = &labNonce
		r.LastEpochBlockNonce = &lastEpochBlockNonce
	case chainDepStateVersionTPraos:
		// TPraos (Shelley through Alonzo): [lastSlot, PrtclState], where
		// PrtclState is [opCertCounters, evolvingNonce, candidateNonce].
		var t struct {
			cbor.StructAsArray
			LastSlot   WithOriginSlot
			PrtclState struct {
				cbor.StructAsArray
				OpCertCounters map[ledger.Blake2b224]uint64
				EvolvingNonce  lcommon.Nonce
				CandidateNonce lcommon.Nonce
			}
		}
		if _, err := cbor.Decode(outer.Inner, &t); err != nil {
			return err
		}
		r.Protocol = ChainDepStateProtocolTPraos
		r.LastSlot = t.LastSlot
		r.OpCertCounters = t.PrtclState.OpCertCounters
		r.EvolvingNonce = t.PrtclState.EvolvingNonce
		r.CandidateNonce = t.PrtclState.CandidateNonce
		// EpochNonce and the other Praos-only nonces do not exist in TPraos.
		// Clear them explicitly so decoding into a reused result cannot leave
		// stale Praos values from a previous decode.
		r.EpochNonce = nil
		r.PreviousEpochNonce = nil
		r.LabNonce = nil
		r.LastEpochBlockNonce = nil
	default:
		return fmt.Errorf(
			"unsupported ChainDepState serialisation version: %d",
			outer.Version,
		)
	}
	// Normalise a missing/absent counter map to an empty (non-nil) map so
	// callers can range over it and look up keys without a nil check.
	if r.OpCertCounters == nil {
		r.OpCertCounters = map[ledger.Blake2b224]uint64{}
	}
	return nil
}
