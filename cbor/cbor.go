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
	_cbor "github.com/fxamacker/cbor/v2"
)

const (
	CborTypeByteString uint8 = 0x40
	CborTypeTextString uint8 = 0x60
	CborTypeArray      uint8 = 0x80
	CborTypeMap        uint8 = 0xa0
	CborTypeTag        uint8 = 0xc0

	// Only the top 3 bytes are used to specify the type
	CborTypeMask uint8 = 0xe0

	// Max value able to be stored in a single byte without type prefix
	CborMaxUintSimple uint8 = 0x17
)

// Create an alias for RawMessage for convenience
type RawMessage = _cbor.RawMessage

// Alias for Tag for convenience
type Tag = _cbor.Tag
type RawTag = _cbor.RawTag

// Useful for embedding and easier to remember
type StructAsArray struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_ struct{} `cbor:",toarray"`
}

type DecodeStoreCborInterface interface {
	Cbor() []byte
	SetCbor([]byte)
}

type DecodeStoreCbor struct {
	cborData []byte
}

func (d *DecodeStoreCbor) SetCbor(cborData []byte) {
	if cborData == nil {
		d.cborData = nil
		return
	}
	d.cborData = make([]byte, len(cborData))
	copy(d.cborData, cborData)
}

// Cbor returns the original CBOR for the object
func (d *DecodeStoreCbor) Cbor() []byte {
	return d.cborData
}

// UnmarshalCbor decodes the specified CBOR into the destination object and saves the original CBOR
func (d *DecodeStoreCbor) UnmarshalCbor(
	cborData []byte,
	dest DecodeStoreCborInterface,
) error {
	if err := DecodeGeneric(cborData, dest); err != nil {
		return err
	}
	// Store a copy of the original CBOR data
	// This must be done after we copy from the temp object above, or it gets wiped out
	// when using struct embedding and the DecodeStoreCbor struct is embedded at a deeper level
	d.SetCbor(cborData)
	return nil
}
