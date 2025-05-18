// Copyright 2023 Blink Labs Software
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

// The common package contains types used by multiple mini-protocols
package common

import (
	"github.com/blinklabs-io/gouroboros/cbor"
)

// The Point type represents a point on the blockchain. It consists of a slot number and block hash
type Point struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_    struct{} `cbor:",toarray"`
	Slot uint64
	Hash []byte
}

// NewPoint returns a Point object with the specified slot number and block hash
func NewPoint(slot uint64, blockHash []byte) Point {
	return Point{
		Slot: slot,
		Hash: blockHash,
	}
}

// NewPointOrigin returns an "empty" Point object which represents the origin of the blockchain
func NewPointOrigin() Point {
	return Point{}
}

// UnmarshalCBOR is a helper function for decoding a Point object from CBOR. The object content can vary,
// so we need to do some special handling when decoding. It is not intended to be called directly.
func (p *Point) UnmarshalCBOR(data []byte) error {
	var tmp []any
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	if len(tmp) > 0 {
		p.Slot = tmp[0].(uint64)
		p.Hash = tmp[1].([]byte)
	}
	return nil
}

// MarshalCBOR is a helper function for encoding a Point object to CBOR. The object content can vary, so we
// need to do some special handling when encoding. It is not intended to be called directly.
func (p *Point) MarshalCBOR() ([]byte, error) {
	var data []any
	if p.Slot == 0 && p.Hash == nil {
		// Return an empty list if values are zero
		data = make([]any, 0)
	} else {
		data = []any{p.Slot, p.Hash}
	}
	return cbor.Encode(data)
}

// Tip represents a Point combined with a block number
type Tip struct {
	cbor.StructAsArray
	Point       Point
	BlockNumber uint64
}
