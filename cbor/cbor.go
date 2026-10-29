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

package cbor

import (
	"bytes"

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
type (
	Tag    = _cbor.Tag
	RawTag = _cbor.RawTag
)

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

// SetCborReference stores a direct reference to CBOR bytes without copying.
// Callers must ensure the referenced slice remains immutable for the lifetime
// of any object that relies on it.
func (d *DecodeStoreCbor) SetCborReference(cborData []byte) {
	d.cborData = cborData
}

// ShareWithinParent replaces the stored CBOR with a view into parent when the
// stored bytes are exactly the first element of the array parent encodes (a
// one-byte definite or indefinite array head followed by the element). It
// leaves the stored CBOR untouched otherwise. The view is capacity-limited so
// an append cannot write into parent. Callers must treat parent as immutable
// for the lifetime of the object, as with SetCborReference.
func (d *DecodeStoreCbor) ShareWithinParent(parent []byte) {
	n := len(d.cborData)
	if n == 0 || len(parent) < 1+n {
		return
	}
	if parent[0]&CborTypeMask != CborTypeArray {
		return
	}
	// 0x80-0x97 are definite heads of up to 23 elements; 0x9f is indefinite.
	if parent[0] > 0x97 && parent[0] != 0x9f {
		return
	}
	view := parent[1 : 1+n : 1+n]
	if !bytes.Equal(view, d.cborData) {
		return
	}
	d.cborData = view
}

// ViewWithin returns a capacity-limited sub-slice of parent holding the same
// bytes as b, so a decoded byte-string field can share the backing array of
// the CBOR its owner already retains instead of keeping a separate copy. It
// returns b unchanged when b is empty or its bytes do not occur in parent.
// Callers must treat parent as immutable for the lifetime of the result.
func ViewWithin(parent, b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	i := bytes.Index(parent, b)
	if i < 0 {
		return b
	}
	return parent[i : i+len(b) : i+len(b)]
}

// Cbor returns the original CBOR for the object
func (d DecodeStoreCbor) Cbor() []byte {
	return d.cborData
}
