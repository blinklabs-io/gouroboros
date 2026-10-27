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
	"bytes"
	"fmt"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

const (
	// cborIndefiniteMapHeader opens an indefinite-length CBOR map and
	// cborBreak closes it (RFC 8949 section 3.2.1).
	cborIndefiniteMapHeader = 0xbf
	cborBreak               = 0xff
	// ledgerMapLenThreshold is cardano-ledger's lengthThreshold: encodeMap
	// emits a definite-length header at or below this many pairs and an
	// indefinite-length map above it.
	ledgerMapLenThreshold = 23
)

// DRepStakeDistrEntry is one DRep's entry in a GetDRepStakeDistr reply: a DRep
// and the total stake, in lovelace, delegated to it.
type DRepStakeDistrEntry struct {
	Drep  lcommon.Drep
	Stake uint64
}

// DRepStakeDistrResult is the GetDRepStakeDistr reply, the ledger's
// Map DRep Coin.
//
// Wire shape:
//
//	drep_stake_distr_result = [ { * drep => coin } ]
//	drep                    = [ 0, addr_keyhash ] / [ 1, scripthash ]
//	                        / [ 2 ] / [ 3 ]
//	coin                    = uint
//
// The outer single-element array is the wrapper the Shelley era codec puts
// around every era-specific query result, the same one GetDRepState carries
// (#2169). The DRep keys use the encoding of [lcommon.Drep], where types 2 and
// 3 are the predefined Abstain and NoConfidence options and carry no
// credential.
//
// The map is held as a slice rather than a Go map because [lcommon.Drep]
// stores its credential in a byte slice and so cannot be a map key. Entries
// keep the order in which they arrived. cardano-ledger encodes Map DRep Coin
// with encodeMap, which walks the map in Haskell's derived Ord order over
// DRep's constructors, so a node emits the credential-backed DReps first
// (types 0 then 1) and the predefined options last (types 2 then 3). That is
// not the RFC 8949 section 4.2.1 order, which would sort the one-element
// arrays first.
//
// encodeMap also switches to an indefinite-length map above
// [ledgerMapLenThreshold] pairs, so a mainnet-sized distribution arrives as
// 0xbf ... 0xff rather than a definite-length header.
type DRepStakeDistrResult []DRepStakeDistrEntry

func (r *DRepStakeDistrResult) UnmarshalCBOR(data []byte) error {
	var entries DRepStakeDistrResult
	err := decodeDRepKeyedResult(
		data,
		"DRep stake distribution",
		func(dec *cbor.StreamDecoder, drep lcommon.Drep) error {
			entry := DRepStakeDistrEntry{Drep: drep}
			if _, _, err := dec.Decode(&entry.Stake); err != nil {
				return fmt.Errorf(
					"DRep stake distribution: decoding stake: %w",
					err,
				)
			}
			entries = append(entries, entry)
			return nil
		},
	)
	if err != nil {
		return err
	}
	if entries == nil {
		entries = DRepStakeDistrResult{}
	}
	*r = entries
	return nil
}

func (r DRepStakeDistrResult) MarshalCBOR() ([]byte, error) {
	entries := make([]drepKeyedValue, 0, len(r))
	for _, entry := range r {
		value, err := cbor.Encode(entry.Stake)
		if err != nil {
			return nil, fmt.Errorf(
				"DRep stake distribution: encoding stake: %w",
				err,
			)
		}
		entries = append(entries, drepKeyedValue{
			drep:  entry.Drep,
			value: value,
		})
	}
	return encodeDRepKeyedResult(entries, "DRep stake distribution")
}

// decodeDRepKeyedResult walks a ledger Map keyed by DRep inside the
// single-element era result wrapper, handing each key to decodeValue with the
// decoder positioned on its value. Keys arrive in wire order.
func decodeDRepKeyedResult(
	data []byte,
	what string,
	decodeValue func(dec *cbor.StreamDecoder, drep lcommon.Drep) error,
) error {
	var wrapper []cbor.RawMessage
	if _, err := cbor.Decode(data, &wrapper); err != nil {
		return err
	}
	if len(wrapper) != 1 {
		return fmt.Errorf(
			"%s: expected a single-element result array, got %d elements",
			what,
			len(wrapper),
		)
	}
	distr := []byte(wrapper[0])
	dec, err := cbor.NewStreamDecoder(distr)
	if err != nil {
		return err
	}
	var entryCount int
	indefinite := len(distr) > 0 && distr[0] == cborIndefiniteMapHeader
	if indefinite {
		if err := dec.Advance(1); err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	} else {
		entryCount, _, _, err = dec.DecodeMapHeader()
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
	}
	// The seen maps start empty and grow as keys are decoded. Sizing them
	// from entryCount would allocate for every declared pair before a single
	// key is checked, so a small payload could force a large allocation.
	//
	// The decode modes in the cbor package reject a repeated map key
	// (DupMapKeyEnforcedAPF), so every other map-shaped result in this
	// package gets that for free. Walking this map by hand does not, and a
	// caller summing a slice that repeats a DRep counts it twice.
	//
	// Two checks are needed, because a repeat can take two forms. A CBOR
	// map's keys are unique by their encoding, so seenEncoded rejects a
	// literally repeated key. That is not enough on its own: a credential
	// written as indefinite-length chunks decodes to the same lcommon.Drep
	// as the definite-length form from different bytes. seenDrep rejects
	// that pair too.
	type drepKey struct {
		drepType   int
		credential string
	}
	seenEncoded := make(map[string]struct{})
	seenDrep := make(map[drepKey]struct{})
	for i := 0; indefinite || i < entryCount; i++ {
		if indefinite {
			// A bounds guard before the index below, not a validation
			// path: distr is one complete well-formed CBOR item, so an
			// indefinite map inside it always carries its break byte.
			if dec.EOF() {
				return fmt.Errorf(
					"%s: unterminated indefinite-length map",
					what,
				)
			}
			if distr[dec.Position()] == cborBreak {
				if err := dec.Advance(1); err != nil {
					return fmt.Errorf("%s: %w", what, err)
				}
				break
			}
		}
		var drep lcommon.Drep
		_, key, err := dec.DecodeRaw(&drep)
		if err != nil {
			return fmt.Errorf("%s: decoding DRep: %w", what, err)
		}
		if _, dup := seenEncoded[string(key)]; dup {
			return fmt.Errorf("%s: duplicate DRep %s", what, drep.String())
		}
		seenEncoded[string(key)] = struct{}{}
		decoded := drepKey{
			drepType:   drep.Type,
			credential: string(drep.Credential),
		}
		if _, dup := seenDrep[decoded]; dup {
			return fmt.Errorf("%s: duplicate DRep %s", what, drep.String())
		}
		seenDrep[decoded] = struct{}{}
		if err := decodeValue(dec, drep); err != nil {
			return err
		}
	}
	return nil
}

// drepKeyedValue is one entry of a DRep-keyed ledger Map with its value
// already encoded.
type drepKeyedValue struct {
	drep  lcommon.Drep
	value []byte
}

// encodeDRepKeyedResult encodes a ledger Map keyed by DRep inside the
// single-element era result wrapper, in the order and framing a node uses.
func encodeDRepKeyedResult(
	entries []drepKeyedValue,
	what string,
) ([]byte, error) {
	type encodedEntry struct {
		key        []byte
		value      []byte
		drepType   int
		credential []byte
	}
	encoded := make([]encodedEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		key, err := cbor.Encode(entry.drep)
		if err != nil {
			return nil, fmt.Errorf("%s: encoding DRep: %w", what, err)
		}
		// Keyed on the encoding, not on the Go value: an Abstain or
		// NoConfidence DRep carries no credential on the wire, so two
		// entries that differ only in a stray credential would collide
		// into one map key.
		if _, dup := seen[string(key)]; dup {
			return nil, fmt.Errorf(
				"%s: duplicate DRep %s",
				what,
				entry.drep.String(),
			)
		}
		seen[string(key)] = struct{}{}
		encoded = append(encoded, encodedEntry{
			key:        key,
			value:      entry.value,
			drepType:   entry.drep.Type,
			credential: entry.drep.Credential,
		})
	}
	// cardano-ledger walks a Map keyed by DRep in Haskell's derived Ord order
	// over DRep's constructors, which runs DRepKeyHash, DRepScriptHash,
	// DRepAlwaysAbstain, DRepAlwaysNoConfidence: the same sequence as the
	// type numbers, with the credential compared bytewise within a type.
	// Sorting by the encoded key instead would put the one-element arrays
	// first and emit bytes no node produces.
	slices.SortFunc(encoded, func(a, b encodedEntry) int {
		if a.drepType != b.drepType {
			return a.drepType - b.drepType
		}
		return bytes.Compare(a.credential, b.credential)
	})
	buf := bytes.NewBuffer(nil)
	// Single-element array wrapping the era-specific result
	buf.WriteByte(0x81)
	// encodeMap emits a definite-length header at or below the threshold and
	// an indefinite-length map above it.
	indefinite := len(encoded) > ledgerMapLenThreshold
	if indefinite {
		buf.WriteByte(cborIndefiniteMapHeader)
	} else {
		// Masked rather than converted directly: the branch already
		// bounds the count by ledgerMapLenThreshold (23), and a
		// definite-length map header carries a count below 24 in its
		// low five bits.
		buf.WriteByte(0xa0 | uint8(len(encoded)&0x1f))
	}
	for _, entry := range encoded {
		buf.Write(entry.key)
		buf.Write(entry.value)
	}
	if indefinite {
		buf.WriteByte(cborBreak)
	}
	return buf.Bytes(), nil
}
