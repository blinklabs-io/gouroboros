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

// The common package contains types used by multiple mini-protocols
package common

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

// The Point type represents a point on the blockchain. It consists of a slot number and block hash
type Point struct {
	cbor.StructAsArray
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
	// Points use a definite-length array: [] for origin or [slot, hash].
	listLen, headerSize, indefinite := cbor.ArrayInfo(data)
	if listLen < 0 || indefinite {
		return errors.New("Point must be a definite-length array")
	}
	if listLen == 0 {
		if len(data) != int(headerSize) {
			return errors.New("Point contains trailing CBOR data")
		}
		*p = NewPointOrigin()
		return nil
	}
	if listLen != 2 {
		return fmt.Errorf("Point must contain 0 or 2 elements, got %d", listLen)
	}
	pos := int(headerSize)
	slotHead, ok, err := cborwalk.ReadHead(data, pos)
	if err != nil || !ok || slotHead.Major != 0 || slotHead.Indefinite {
		return errors.New("Point slot must be an unsigned integer")
	}
	slotEnd := pos + slotHead.EncodedSize
	if slotEnd > len(data) {
		return errors.New("Point slot is truncated")
	}
	hashHead, ok, err := cborwalk.ReadHead(data, slotEnd)
	if err != nil || !ok || hashHead.Major != 2 || hashHead.Indefinite {
		return errors.New("Point hash must be a byte string")
	}
	if hashHead.Argument != 32 {
		return fmt.Errorf("Point hash must be 32 bytes, got %d", hashHead.Argument)
	}
	hashStart := slotEnd + hashHead.EncodedSize
	if hashStart > len(data) || len(data)-hashStart != 32 {
		return errors.New("Point contains trailing or truncated CBOR data")
	}
	var slot uint64
	consumed, err := cbor.Decode(data[pos:slotEnd], &slot)
	if err != nil {
		return err
	}
	if consumed != slotEnd-pos {
		return errors.New("Point slot contains trailing CBOR data")
	}
	*p = NewPoint(slot, append([]byte(nil), data[hashStart:]...))
	return nil
}

// MarshalCBOR is a helper function for encoding a Point object to CBOR. The object content can vary, so we
// need to do some special handling when encoding. It is not intended to be called directly.
func (p *Point) MarshalCBOR() ([]byte, error) {
	var data []any
	if p.Slot == 0 && p.Hash == nil {
		// Return an empty list if values are zero
		data = []any{}
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
