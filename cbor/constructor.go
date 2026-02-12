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
	"fmt"
	"strings"
)

// alternativeToTag converts a constructor/alternative number to its CBOR tag number.
// Returns the tag number and whether the fields must be wrapped as [alt_number, fields]
// (true for alternatives 128+).
func alternativeToTag(alt uint) (uint64, bool) {
	switch {
	case alt <= 6:
		return uint64(alt) + CborTagAlternative1Min, false
	case alt <= 127:
		return uint64(alt) - 7 + CborTagAlternative2Min, false
	default:
		return CborTagAlternative3, true
	}
}

// IsAlternativeTag returns true if the given CBOR tag number represents
// a constructor/alternative (tags 121-127, 1280-1400, or 101).
func IsAlternativeTag(tagNum uint64) bool {
	return (tagNum >= CborTagAlternative1Min && tagNum <= CborTagAlternative1Max) ||
		(tagNum >= CborTagAlternative2Min && tagNum <= CborTagAlternative2Max) ||
		tagNum == CborTagAlternative3
}

// ConstructorEncoder builds a CBOR constructor/alternative for encoding.
// Use this when constructing a new constructor value from typed fields.
type ConstructorEncoder struct {
	tag    uint
	fields any
}

// NewConstructorEncoder creates a ConstructorEncoder with the given alternative
// number and fields value. The fields value is typically a []any.
func NewConstructorEncoder(tag uint, fields any) ConstructorEncoder {
	return ConstructorEncoder{tag: tag, fields: fields}
}

// Tag returns the alternative/constructor number.
func (ce ConstructorEncoder) Tag() uint {
	return ce.tag
}

// MarshalCBOR encodes the constructor as a CBOR tagged value.
func (ce ConstructorEncoder) MarshalCBOR() ([]byte, error) {
	tagNum, wrap := alternativeToTag(ce.tag)
	var content any
	if wrap {
		content = []any{ce.tag, ce.fields}
	} else {
		content = ce.fields
	}
	tmpTag := Tag{Number: tagNum, Content: content}
	return Encode(&tmpTag)
}

// ConstructorDecoder decodes a CBOR constructor/alternative, keeping fields as
// raw CBOR bytes for type-safe deferred decoding.
//
// This replaces the pattern of decoding into cbor.Value and then type-asserting
// to Constructor. Instead, embed ConstructorDecoder and call DecodeFields to
// decode into typed structs:
//
//	type DatumOption struct {
//	    cbor.ConstructorDecoder
//	}
//
//	func (d *DatumOption) IsDatumHash() bool { return d.Tag() == 0 }
type ConstructorDecoder struct {
	DecodeStoreCbor
	tag    uint
	fields RawMessage
}

// Tag returns the alternative/constructor number.
func (cd ConstructorDecoder) Tag() uint {
	return cd.tag
}

// Fields returns the raw CBOR bytes of the constructor fields.
func (cd ConstructorDecoder) Fields() RawMessage {
	return cd.fields
}

// DecodeFields decodes the constructor fields into the destination.
// The destination should match the CBOR structure of the fields
// (e.g., a slice for an array, a struct with StructAsArray for a fixed-length array).
func (cd ConstructorDecoder) DecodeFields(dest any) error {
	_, err := Decode(cd.fields, dest)
	return err
}

// ParsedFields parses the fields through Value for proper handling of
// CBOR types that need special Go representation (ByteStrings, nested constructors, etc.)
// and returns the result as []any.
func (cd ConstructorDecoder) ParsedFields() ([]any, error) {
	var tmpValue Value
	if _, err := Decode(cd.fields, &tmpValue); err != nil {
		return nil, err
	}
	fields, ok := tmpValue.Value().([]any)
	if !ok {
		return nil, fmt.Errorf(
			"constructor fields are not an array, got %T",
			tmpValue.Value(),
		)
	}
	return fields, nil
}

// MarshalJSON produces Cardano-style AST JSON:
// {"constructor":N,"fields":[...]}
func (cd ConstructorDecoder) MarshalJSON() ([]byte, error) {
	fields, err := cd.ParsedFields()
	if err != nil {
		return nil, err
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`{"constructor":%d,"fields":[`, cd.tag))
	for idx, val := range fields {
		tmpVal, err := generateAstJson(val)
		if err != nil {
			return nil, err
		}
		sb.WriteString(string(tmpVal))
		if idx != len(fields)-1 {
			sb.WriteString(`,`)
		}
	}
	sb.WriteString(`]}`)
	return []byte(sb.String()), nil
}

// UnmarshalCBOR decodes a CBOR constructor/alternative tag.
func (cd *ConstructorDecoder) UnmarshalCBOR(data []byte) error {
	cd.SetCbor(data)
	tmpTag := RawTag{}
	if _, err := Decode(data, &tmpTag); err != nil {
		return err
	}
	switch {
	case tmpTag.Number >= CborTagAlternative1Min && tmpTag.Number <= CborTagAlternative1Max:
		// Alternatives 0-6 (tags 121-127)
		cd.tag = uint(tmpTag.Number - CborTagAlternative1Min)
		cd.fields = RawMessage(tmpTag.Content)
	case tmpTag.Number >= CborTagAlternative2Min && tmpTag.Number <= CborTagAlternative2Max:
		// Alternatives 7-127 (tags 1280-1400)
		cd.tag = uint(tmpTag.Number - CborTagAlternative2Min + 7)
		cd.fields = RawMessage(tmpTag.Content)
	case tmpTag.Number == CborTagAlternative3:
		// Alternatives 128+ (tag 101): content is [constructor_number, fields]
		var outerArray []RawMessage
		if _, err := Decode(tmpTag.Content, &outerArray); err != nil {
			return fmt.Errorf("decode alternative 128+ content: %w", err)
		}
		if len(outerArray) != 2 {
			return fmt.Errorf(
				"expected 2 elements for alternative 128+, got %d",
				len(outerArray),
			)
		}
		var altNum uint64
		if _, err := Decode(outerArray[0], &altNum); err != nil {
			return fmt.Errorf("decode alternative number: %w", err)
		}
		cd.tag = uint(altNum)
		cd.fields = outerArray[1]
	default:
		return fmt.Errorf("unsupported constructor tag: %d", tmpTag.Number)
	}
	return nil
}

// MarshalCBOR encodes the constructor as CBOR. If original bytes are available
// (from a previous UnmarshalCBOR), they are returned for perfect round-trip fidelity.
func (cd ConstructorDecoder) MarshalCBOR() ([]byte, error) {
	if stored := cd.Cbor(); len(stored) > 0 {
		return stored, nil
	}
	// Rebuild from tag + raw fields
	tagNum, wrap := alternativeToTag(cd.tag)
	if wrap {
		// Alternative 128+: content is [alt_number, fields]
		altBytes, err := Encode(uint64(cd.tag))
		if err != nil {
			return nil, err
		}
		// Build CBOR array(2): header byte + alt_number_cbor + fields_cbor
		arrayContent := make([]byte, 0, 1+len(altBytes)+len(cd.fields))
		arrayContent = append(arrayContent, 0x82) // definite-length array of 2
		arrayContent = append(arrayContent, altBytes...)
		arrayContent = append(arrayContent, cd.fields...)
		tmpTag := RawTag{Number: tagNum, Content: RawMessage(arrayContent)}
		return Encode(&tmpTag)
	}
	tmpTag := RawTag{Number: tagNum, Content: cd.fields}
	return Encode(&tmpTag)
}
