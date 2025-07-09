// Copyright 2025 Blink Labs Software
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
	"errors"
	"fmt"
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
	tagOpts := _cbor.TagOptions{
		EncTag: _cbor.EncTagRequired,
		DecTag: _cbor.DecTagRequired,
	}
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
	tmpRat := []any{}
	if _, err := Decode(cborData, &tmpRat); err != nil {
		return err
	}
	// Convert numerator to big.Int
	tmpNum := new(big.Int)
	switch v := tmpRat[0].(type) {
	case int64:
		tmpNum.SetInt64(v)
	case uint64:
		tmpNum.SetUint64(v)
	default:
		return fmt.Errorf("unsupported numerator type for cbor.Rat: %T", v)
	}
	// Convert denominator to big.Int
	tmpDenom := new(big.Int)
	switch v := tmpRat[1].(type) {
	case int64:
		tmpDenom.SetInt64(v)
	case uint64:
		tmpDenom.SetUint64(v)
	default:
		return fmt.Errorf("unsupported denominator type for cbor.Rat: %T", v)
	}
	// Create new big.Rat with num/denom set to big.Int values above
	r.Rat = new(big.Rat)
	r.SetFrac(tmpNum, tmpDenom)
	return nil
}

func (r *Rat) MarshalCBOR() ([]byte, error) {
	tmpContent := make([]any, 2)
	// Numerator
	if r.Num().IsUint64() {
		tmpContent[0] = r.Num().Uint64()
	} else if r.Num().IsInt64() {
		tmpContent[0] = r.Num().Int64()
	} else {
		return nil, errors.New("numerator cannot be represented at int64/uint64")
	}
	// Denominator
	if r.Denom().IsUint64() {
		tmpContent[1] = r.Denom().Uint64()
	} else if r.Denom().IsInt64() {
		tmpContent[1] = r.Denom().Int64()
	} else {
		return nil, errors.New("numerator cannot be represented at int64/uint64")
	}
	tmpData := _cbor.Tag{
		Number:  CborTagRational,
		Content: tmpContent,
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

// SetType is a generic type for wrapping other types in an optional CBOR set tag
type SetType[T any] struct {
	useTag bool
	items  []T
}

func NewSetType[T any](items []T, useTag bool) SetType[T] {
	return SetType[T]{
		items:  items,
		useTag: useTag,
	}
}

func (t *SetType[T]) UnmarshalCBOR(data []byte) error {
	// Check if the set is wrapped in a CBOR tag
	// This is mostly needed so we can remember whether it was Set-wrapped for CBOR encoding
	var tmpTag RawTag
	t.useTag = false
	if _, err := Decode(data, &tmpTag); err == nil {
		if tmpTag.Number != CborTagSet {
			return errors.New("unexpected tag type")
		}
		data = []byte(tmpTag.Content)
		t.useTag = true
	}
	var tmpData []T
	if _, err := Decode(data, &tmpData); err != nil {
		return err
	}
	t.items = tmpData
	return nil
}

func (t *SetType[T]) MarshalCBOR() ([]byte, error) {
	tmpItems := make([]any, len(t.items))
	for i, item := range t.items {
		tmpItems[i] = item
	}
	var tmpData any = tmpItems
	if t.useTag {
		tmpData = Set(tmpItems)
	}
	return Encode(tmpData)
}

func (t *SetType[T]) Items() []T {
	ret := make([]T, len(t.items))
	copy(ret, t.items)
	return ret
}
