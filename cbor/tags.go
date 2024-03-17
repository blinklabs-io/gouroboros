// Copyright 2024 Blink Labs Software
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

package cbor

import (
	"math/big"
	"reflect"

	_cbor "github.com/fxamacker/cbor/v2"
)

const (
	// Useful tag numbers
	CborTagCbor     = 24
	CborTagRational = 30
	CborTagSet      = 258
	CborTagMap      = 259

	// Tag ranges for "alternatives"
	// https://www.ietf.org/archive/id/draft-bormann-cbor-notable-tags-07.html#name-enumerated-alternative-data
	CborTagAlternative1Min = 121
	CborTagAlternative1Max = 127
	CborTagAlternative2Min = 1280
	CborTagAlternative2Max = 1400
	CborTagAlternative3    = 101
)

var customTagSet _cbor.TagSet

func init() {
	// Build custom tagset
	customTagSet = _cbor.NewTagSet()
	tagOpts := _cbor.TagOptions{EncTag: _cbor.EncTagRequired, DecTag: _cbor.DecTagRequired}
	// Wrapped CBOR
	if err := customTagSet.Add(
		tagOpts,
		reflect.TypeOf(WrappedCbor{}),
		CborTagCbor,
	); err != nil {
		panic(err)
	}
	// Rational numbers
	if err := customTagSet.Add(
		tagOpts,
		reflect.TypeOf(Rat{}),
		CborTagRational,
	); err != nil {
		panic(err)
	}
	// Sets
	if err := customTagSet.Add(
		tagOpts,
		reflect.TypeOf(Set{}),
		CborTagSet,
	); err != nil {
		panic(err)
	}
	// Maps
	if err := customTagSet.Add(
		tagOpts,
		reflect.TypeOf(Map{}),
		CborTagMap,
	); err != nil {
		panic(err)
	}
}

// WrappedCbor corresponds to CBOR tag 24 and is used to encode nested CBOR data
type WrappedCbor []byte

func (w WrappedCbor) Bytes() []byte {
	return w[:]
}

// Rat corresponds to CBOR tag 30 and is used to represent a rational number
type Rat struct {
	*big.Rat
}

func (r *Rat) UnmarshalCBOR(cborData []byte) error {
	tmpRat := []int64{}
	if _, err := Decode(cborData, &tmpRat); err != nil {
		return err
	}
	r.Rat = big.NewRat(tmpRat[0], tmpRat[1])
	return nil
}

func (r *Rat) MarshalCBOR() ([]byte, error) {
	tmpData := _cbor.Tag{
		Number: CborTagRational,
		Content: []uint64{
			r.Num().Uint64(),
			r.Denom().Uint64(),
		},
	}
	return Encode(&tmpData)
}

func (r *Rat) ToBigRat() *big.Rat {
	return r.Rat
}

// Set corresponds to CBOR tag 258 and is used to represent a mathematical finite set
type Set []any

// Map corresponds to CBOR tag 259 and is used to represent a map with key/value operations
type Map map[any]any
