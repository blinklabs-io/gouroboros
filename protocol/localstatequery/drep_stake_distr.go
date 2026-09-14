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
	"errors"
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
	var wrapper []cbor.RawMessage
	if _, err := cbor.Decode(data, &wrapper); err != nil {
		return err
	}
	if len(wrapper) != 1 {
		return fmt.Errorf(
			"DRep stake distribution: expected a single-element result array, got %d elements",
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
			return fmt.Errorf("DRep stake distribution: %w", err)
		}
	} else {
		entryCount, _, _, err = dec.DecodeMapHeader()
		if err != nil {
			return fmt.Errorf("DRep stake distribution: %w", err)
		}
	}
	// distr is a cbor.RawMessage taken out of the result array, so it is one
	// well-formed CBOR item: a definite-length map header cannot declare more
	// pairs than the item actually carries, and entryCount is therefore
	// bounded by len(distr).
	entries := make(DRepStakeDistrResult, 0, entryCount)
	// The decode modes in the cbor package reject a repeated map key
	// (DupMapKeyEnforcedAPF), so every other map-shaped result in this
	// package gets that for free. Walking this map by hand does not, and a
	// caller summing a slice that repeats a DRep counts its stake twice.
	//
	// Two checks are needed, because a repeat can take two forms. A CBOR
	// map's keys are unique by their encoding, so seenEncoded rejects a
	// literally repeated key. That is not enough on its own: lcommon.Drep
	// reads the type from the list head and ignores any further elements
	// for the predefined options, so [2] and [2, h'00'] decode to the same
	// Abstain DRep from different bytes. seenDrep rejects that pair too,
	// since a caller summing the result would otherwise count Abstain's
	// stake twice.
	type drepKey struct {
		drepType   int
		credential string
	}
	seenEncoded := make(map[string]struct{}, entryCount)
	seenDrep := make(map[drepKey]struct{}, entryCount)
	for i := 0; indefinite || i < entryCount; i++ {
		if indefinite {
			// A bounds guard before the index below, not a validation
			// path: distr is one complete well-formed CBOR item, so an
			// indefinite map inside it always carries its break byte.
			if dec.EOF() {
				return errors.New(
					"DRep stake distribution: unterminated indefinite-length map",
				)
			}
			if distr[dec.Position()] == cborBreak {
				if err := dec.Advance(1); err != nil {
					return fmt.Errorf("DRep stake distribution: %w", err)
				}
				break
			}
		}
		var entry DRepStakeDistrEntry
		_, key, err := dec.DecodeRaw(&entry.Drep)
		if err != nil {
			return fmt.Errorf(
				"DRep stake distribution: decoding DRep: %w",
				err,
			)
		}
		if _, dup := seenEncoded[string(key)]; dup {
			return fmt.Errorf(
				"DRep stake distribution: duplicate DRep %s",
				entry.Drep.String(),
			)
		}
		seenEncoded[string(key)] = struct{}{}
		decoded := drepKey{
			drepType:   entry.Drep.Type,
			credential: string(entry.Drep.Credential),
		}
		if _, dup := seenDrep[decoded]; dup {
			return fmt.Errorf(
				"DRep stake distribution: duplicate DRep %s",
				entry.Drep.String(),
			)
		}
		seenDrep[decoded] = struct{}{}
		if _, _, err := dec.Decode(&entry.Stake); err != nil {
			return fmt.Errorf(
				"DRep stake distribution: decoding stake: %w",
				err,
			)
		}
		entries = append(entries, entry)
	}
	*r = entries
	return nil
}

func (r DRepStakeDistrResult) MarshalCBOR() ([]byte, error) {
	type encodedEntry struct {
		key        []byte
		value      []byte
		drepType   int
		credential []byte
	}
	encoded := make([]encodedEntry, 0, len(r))
	seen := make(map[string]struct{}, len(r))
	for _, entry := range r {
		key, err := cbor.Encode(entry.Drep)
		if err != nil {
			return nil, fmt.Errorf(
				"DRep stake distribution: encoding DRep: %w",
				err,
			)
		}
		// Keyed on the encoding, not on the Go value: an Abstain or
		// NoConfidence DRep carries no credential on the wire, so two
		// entries that differ only in a stray credential would collide
		// into one map key.
		if _, dup := seen[string(key)]; dup {
			return nil, fmt.Errorf(
				"DRep stake distribution: duplicate DRep %s",
				entry.Drep.String(),
			)
		}
		seen[string(key)] = struct{}{}
		value, err := cbor.Encode(entry.Stake)
		if err != nil {
			return nil, fmt.Errorf(
				"DRep stake distribution: encoding stake: %w",
				err,
			)
		}
		encoded = append(encoded, encodedEntry{
			key:        key,
			value:      value,
			drepType:   entry.Drep.Type,
			credential: entry.Drep.Credential,
		})
	}
	// cardano-ledger walks Map DRep Coin in Haskell's derived Ord order over
	// DRep's constructors, which runs DRepKeyHash, DRepScriptHash,
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
