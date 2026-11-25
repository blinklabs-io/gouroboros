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
	"errors"
	"fmt"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
)

// decodeRational decodes a Haskell Rational, written as [numerator,
// denominator] with or without the tag 30 that cardano-ledger adds from
// protocol version 9. cardano-binary, which ouroboros-network uses, never
// adds the tag.
func decodeRational(data []byte) (*big.Rat, error) {
	if len(data) >= 2 && data[0] == 0xd8 && data[1] == cbor.CborTagRational {
		data = data[2:]
	}
	parts, err := decodeBoundedArray(data, 2, "rational")
	if err != nil {
		return nil, fmt.Errorf("decoding rational: %w", err)
	}
	if len(parts) != 2 {
		return nil, fmt.Errorf(
			"decoding rational: expected 2 elements, got %d",
			len(parts),
		)
	}
	var num, denom big.Int
	if _, err := cbor.Decode(parts[0], &num); err != nil {
		return nil, fmt.Errorf("decoding rational numerator: %w", err)
	}
	if _, err := cbor.Decode(parts[1], &denom); err != nil {
		return nil, fmt.Errorf("decoding rational denominator: %w", err)
	}
	if denom.Sign() <= 0 {
		return nil, errors.New(
			"decoding rational: denominator must be positive",
		)
	}
	return new(big.Rat).SetFrac(&num, &denom), nil
}

// encodeRationalUntagged encodes a Rational the way cardano-binary does:
// [numerator, denominator] with no tag.
func encodeRationalUntagged(r *big.Rat) ([]byte, error) {
	if r == nil {
		return nil, errors.New("encoding rational: nil value")
	}
	return cbor.Encode([]*big.Int{r.Num(), r.Denom()})
}

// plainRational is a Haskell Rational field: the ledger writes it untagged
// before protocol version 9 and under tag 30 from 9, so either form decodes.
// cbor.Rat requires the tag, as the ledger's bounded ratios do, so an
// untagged list is decoded as if it carried one.
type plainRational struct {
	value *cbor.Rat
}

func (r *plainRational) UnmarshalCBOR(data []byte) error {
	if len(data) > 0 && data[0]&cbor.CborTypeMask == cbor.CborTypeArray {
		data = append([]byte{0xd8, cbor.CborTagRational}, data...)
	}
	var value cbor.Rat
	if _, err := cbor.Decode(data, &value); err != nil {
		return err
	}
	r.value = &value
	return nil
}

// poolStakeEntry is one pool's [stake fraction, VRF key hash] in a
// GetStakeDistribution or GetPoolDistr reply.
type poolStakeEntry = struct {
	cbor.StructAsArray
	StakeFraction *cbor.Rat
	VrfHash       ledger.Blake2b256
}

// decodePoolStakeMap decodes the single-element result wrapper around a map
// of pool stake entries, reading each stake fraction as a plain rational. The
// map grows only with entries that decode, as growingMap does.
func decodePoolStakeMap(data []byte) (map[ledger.PoolId]poolStakeEntry, error) {
	var tmp struct {
		cbor.StructAsArray
		Results cbor.RawMessage
	}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return nil, err
	}
	var raw map[ledger.PoolId]struct {
		cbor.StructAsArray
		StakeFraction plainRational
		VrfHash       ledger.Blake2b256
	}
	if err := decodeGrowingMap(tmp.Results, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	results := make(map[ledger.PoolId]poolStakeEntry, len(raw))
	for poolID, entry := range raw {
		results[poolID] = poolStakeEntry{
			StakeFraction: entry.StakeFraction.value,
			VrfHash:       entry.VrfHash,
		}
	}
	return results, nil
}
