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
	"encoding/binary"
	"fmt"
	"math"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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
// keep the order in which they arrived, which for a node reply is the
// deterministic order of RFC 8949 section 4.2.1.
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
	entryCount, _, _, err := dec.DecodeMapHeader()
	if err != nil {
		return fmt.Errorf("DRep stake distribution: %w", err)
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
	// A CBOR map's keys are unique by their encoding, so that is what the
	// check compares.
	seen := make(map[string]struct{}, entryCount)
	for range entryCount {
		var entry DRepStakeDistrEntry
		_, key, err := dec.DecodeRaw(&entry.Drep)
		if err != nil {
			return fmt.Errorf(
				"DRep stake distribution: decoding DRep: %w",
				err,
			)
		}
		if _, dup := seen[string(key)]; dup {
			return fmt.Errorf(
				"DRep stake distribution: duplicate DRep %s",
				entry.Drep.String(),
			)
		}
		seen[string(key)] = struct{}{}
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
		key   []byte
		value []byte
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
		encoded = append(encoded, encodedEntry{key: key, value: value})
	}
	// cbor.Encode sorts Go map keys by their encoding (RFC 8949 section
	// 4.2.1). This map is assembled by hand, so it has to sort itself the
	// same way to produce the bytes a node would.
	slices.SortFunc(encoded, func(a, b encodedEntry) int {
		return bytes.Compare(a.key, b.key)
	})
	header, err := cborMapHeader(len(encoded))
	if err != nil {
		return nil, fmt.Errorf("DRep stake distribution: %w", err)
	}
	buf := bytes.NewBuffer(nil)
	// Single-element array wrapping the era-specific result
	buf.WriteByte(0x81)
	buf.Write(header)
	for _, entry := range encoded {
		buf.Write(entry.key)
		buf.Write(entry.value)
	}
	return buf.Bytes(), nil
}

// cborMapHeader returns the head of a definite-length CBOR map (major type 5)
// holding n pairs. The DRep stake distribution has to be assembled byte by
// byte because its keys are not Go-comparable and so cannot be handed to
// cbor.Encode as a map.
func cborMapHeader(n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("negative map length: %d", n)
	}
	switch {
	case n < 24:
		return []byte{0xa0 | byte(n)}, nil
	case n <= math.MaxUint8:
		return []byte{0xb8, byte(n)}, nil
	case n <= math.MaxUint16:
		return binary.BigEndian.AppendUint16(
			[]byte{0xb9},
			uint16(n), //nolint:gosec // bounded by the case above
		), nil
	case n <= math.MaxUint32:
		return binary.BigEndian.AppendUint32(
			[]byte{0xba},
			uint32(n), //nolint:gosec // bounded by the case above
		), nil
	default:
		return binary.BigEndian.AppendUint64(
			[]byte{0xbb},
			uint64(n), //nolint:gosec // n is non-negative
		), nil
	}
}
