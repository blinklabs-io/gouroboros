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
// DebugChainDepState result. The protocols serialise the chain-dependent
// state with different on-wire layouts, so callers can tell which optional
// fields are populated.
type ChainDepStateProtocol int

const (
	// ChainDepStateProtocolPraos is the Praos protocol used by the Babbage
	// era and later, including Conway (serialisation version 0).
	ChainDepStateProtocolPraos ChainDepStateProtocol = iota
	// ChainDepStateProtocolTPraos is the Transitional Praos protocol used by
	// the Shelley, Allegra, Mary and Alonzo eras (serialisation version 1).
	ChainDepStateProtocolTPraos
	// ChainDepStateProtocolPraos2 is the Praos2 protocol of the Leios
	// overlay, which adds the endorser block announcement to the Praos
	// state. It shares its serialisation version with TPraos, so the
	// values here identify the protocol rather than the version.
	ChainDepStateProtocolPraos2
)

// chainDepState serialisation version tags, as emitted by
// Ouroboros.Consensus.Protocol.{Praos,TPraos,PolyPraos} `encodeVersion`.
//
// The protocols' version numbers are separate namespaces: the hard-fork
// combinator's era index precedes them, so the codec is already chosen when
// the version is read. Praos2 therefore writes the same version as TPraos,
// and the two layouts are told apart by the number of fields in the state.
const (
	chainDepStateVersionPraos  uint64 = 0
	chainDepStateVersionTPraos uint64 = 1
)

// The number of fields in the serialised state of each protocol.
const (
	chainDepStateFieldsPraos  = 8
	chainDepStateFieldsTPraos = 2
	chainDepStateFieldsPraos2 = 9
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
//	Praos2 (Leios):     [1, [lastSlot, opCertCounters, evolvingNonce,
//	                        candidateNonce, epochNonce, previousEpochNonce,
//	                        labNonce, lastEpochBlockNonce, announcement]]
//	TPraos (Shelley..Alonzo): [1, [lastSlot,
//	                        [opCertCounters, evolvingNonce, candidateNonce]]]
//
// Praos2 and TPraos write the same version number, so the layout is told
// apart by the number of fields in the state.
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
	// Announcement is the endorser block announcement the last applied
	// header carried, if any. It is only present for Praos2 results, and is
	// nil for the other protocols.
	Announcement *ChainDepStateAnnouncement
}

// ChainDepStateAnnouncement is the endorser block announcement a Leios header
// carried, together with the issuer of that header. With the header's slot,
// the issuer identifies the election the announcement belongs to.
//
// On the wire it is a 3-element array:
//
//	[ issuer, ebHash, ebSize ]
type ChainDepStateAnnouncement struct {
	cbor.StructAsArray
	// Issuer is the key hash of the block issuer that carried the
	// announcement.
	Issuer ledger.Blake2b224
	// EbHash is the hash of the announced endorser block references.
	EbHash ledger.Blake2b256
	// EbSize is the size of the announced endorser block references, a
	// Word32 in the ledger's EbReferencesAnnouncement.
	EbSize uint32
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
		// Praos has no announcement.
		r.Announcement = nil
	case chainDepStateVersionTPraos:
		// TPraos (Shelley through Alonzo) and Praos2 (the Leios overlay)
		// write the same version, so the layout is told apart by the
		// number of fields in the state.
		listLen, err := cbor.ListLength(outer.Inner)
		if err != nil {
			return err
		}
		switch listLen {
		case chainDepStateFieldsTPraos:
			// TPraos: [lastSlot, PrtclState], where PrtclState is
			// [opCertCounters, evolvingNonce, candidateNonce].
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
			// EpochNonce and the other Praos-only nonces do not exist in
			// TPraos. Clear them explicitly so decoding into a reused
			// result cannot leave stale Praos values from a previous
			// decode.
			r.EpochNonce = nil
			r.PreviousEpochNonce = nil
			r.LabNonce = nil
			r.LastEpochBlockNonce = nil
			r.Announcement = nil
		case chainDepStateFieldsPraos2:
			// Praos2: the Praos fields, plus the announcement of the
			// endorser block the last applied header carried.
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
				Announcement        []ChainDepStateAnnouncement
			}
			if _, err := cbor.Decode(outer.Inner, &p); err != nil {
				return err
			}
			r.Protocol = ChainDepStateProtocolPraos2
			r.LastSlot = p.LastSlot
			r.OpCertCounters = p.OpCertCounters
			r.EvolvingNonce = p.EvolvingNonce
			r.CandidateNonce = p.CandidateNonce
			epochNonce := p.EpochNonce
			previousEpochNonce := p.PreviousEpochNonce
			labNonce := p.LabNonce
			lastEpochBlockNonce := p.LastEpochBlockNonce
			r.EpochNonce = &epochNonce
			r.PreviousEpochNonce = &previousEpochNonce
			r.LabNonce = &labNonce
			r.LastEpochBlockNonce = &lastEpochBlockNonce
			// The announcement is a StrictMaybe, which the ledger writes
			// as a list of zero or one element and decodes by rejecting
			// any other length.
			if len(p.Announcement) > 1 {
				return fmt.Errorf(
					"praos2 announcement: expected 0 or 1 entries, got %d",
					len(p.Announcement),
				)
			}
			if len(p.Announcement) == 1 {
				announcement := p.Announcement[0]
				r.Announcement = &announcement
			} else {
				r.Announcement = nil
			}
		default:
			return fmt.Errorf(
				"unsupported ChainDepState layout: %d fields",
				listLen,
			)
		}
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
