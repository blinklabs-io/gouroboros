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

package common

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const (
	cborTypeMask byte = 0xe0

	cborTypeUnsigned   byte = 0x00
	cborTypeNegative   byte = 0x20
	cborTypeByteString byte = 0x40
	cborTypeTextString byte = 0x60
	cborTypeArray      byte = 0x80
	cborTypeMap        byte = 0xA0
	cborTypeTag        byte = 0xC0
	cborTypeFloatSim   byte = 0xE0

	cborAdditionalMask byte = 0x1f
)

type TransactionMetadataSet struct {
	cbor.DecodeStoreCbor
	data     map[uint]cbor.RawMessage
	metadata map[uint]TransactionMetadatum
}

type TransactionMetadatum interface {
	isTransactionMetadatum()
	TypeName() string
	Cbor() []byte
}

type MetaInt struct {
	cbor.DecodeStoreCbor
	Value *big.Int
}

type MetaBytes struct {
	cbor.DecodeStoreCbor
	Value []byte
}

type MetaText struct {
	cbor.DecodeStoreCbor
	Value string
}

type MetaList struct {
	cbor.DecodeStoreCbor
	Items []TransactionMetadatum
}

type MetaPair struct {
	cbor.DecodeStoreCbor
	Key   TransactionMetadatum
	Value TransactionMetadatum
}

type MetaMap struct {
	cbor.DecodeStoreCbor
	Pairs []MetaPair
}

func (MetaInt) isTransactionMetadatum()   {}
func (MetaBytes) isTransactionMetadatum() {}
func (MetaText) isTransactionMetadatum()  {}
func (MetaList) isTransactionMetadatum()  {}
func (MetaMap) isTransactionMetadatum()   {}

func (m MetaInt) TypeName() string   { return "int" }
func (m MetaBytes) TypeName() string { return "bytes" }
func (m MetaText) TypeName() string  { return "text" }
func (m MetaList) TypeName() string  { return "list" }
func (m MetaMap) TypeName() string   { return "map" }

func DecodeMetadatumRaw(b []byte) (TransactionMetadatum, error) {
	if len(b) == 0 {
		return nil, errors.New("empty cbor")
	}
	switch b[0] & cborTypeMask {
	case cborTypeUnsigned, cborTypeNegative:
		n := new(big.Int)
		if _, err := cbor.Decode(b, n); err != nil {
			return nil, err
		}
		m := MetaInt{Value: n}
		m.SetCbor(b)
		return m, nil

	case cborTypeTextString:
		var s string
		if _, err := cbor.Decode(b, &s); err != nil {
			return nil, err
		}
		m := MetaText{Value: s}
		m.SetCbor(b)
		return m, nil

	case cborTypeByteString:
		var bs []byte
		if _, err := cbor.Decode(b, &bs); err != nil {
			return nil, err
		}
		m := MetaBytes{Value: bs}
		m.SetCbor(b)
		return m, nil

	case cborTypeArray:
		var rawItems []cbor.RawMessage
		if _, err := cbor.Decode(b, &rawItems); err != nil {
			return nil, err
		}
		items := make([]TransactionMetadatum, 0, len(rawItems))
		for _, r := range rawItems {
			md, err := DecodeMetadatumRaw(r)
			if err != nil {
				return nil, err
			}
			items = append(items, md)
		}
		m := MetaList{Items: items}
		m.SetCbor(b)
		return m, nil

	case cborTypeMap:
		switch mapFirstKeyType(b) {
		case cborTypeTextString:
			if md, ok := decodeMapTextText(b); ok {
				return md, nil
			}
			if md, ok, err := decodeMapText(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapUint(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapInt(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapBytes(b); ok || err != nil {
				return md, err
			}
		case cborTypeUnsigned:
			if md, ok, err := decodeMapUint(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapInt(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapText(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapBytes(b); ok || err != nil {
				return md, err
			}
		case cborTypeNegative:
			if md, ok, err := decodeMapInt(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapUint(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapText(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapBytes(b); ok || err != nil {
				return md, err
			}
		case cborTypeByteString:
			if md, ok, err := decodeMapBytes(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapUint(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapInt(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapText(b); ok || err != nil {
				return md, err
			}
		default:
			if md, ok, err := decodeMapUint(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapInt(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapText(b); ok || err != nil {
				return md, err
			}
			if md, ok, err := decodeMapBytes(b); ok || err != nil {
				return md, err
			}
		}
		if md, ok, err := decodeMapGeneric(b); ok || err != nil {
			return md, err
		}
		return nil, errors.New("failed to decode CBOR map in metadatum")

	case cborTypeTag, cborTypeFloatSim:
		return nil, fmt.Errorf(
			"unsupported CBOR major type 0x%x in metadata",
			b[0]&cborTypeMask,
		)

	default:
		return nil, errors.New("unknown CBOR major type")
	}
}

func mapFirstKeyType(b []byte) byte {
	if len(b) == 0 || (b[0]&cborTypeMask) != cborTypeMap {
		return 0xff
	}
	additional := b[0] & cborAdditionalMask
	offset := 1
	switch {
	case additional <= 23:
		if additional == 0 {
			return 0xff
		}
	case additional == 24:
		if len(b) < offset+1 || b[offset] == 0 {
			return 0xff
		}
		offset++
	case additional == 25:
		if len(b) < offset+2 || (b[offset] == 0 && b[offset+1] == 0) {
			return 0xff
		}
		offset += 2
	case additional == 26:
		if len(b) < offset+4 ||
			(b[offset] == 0 && b[offset+1] == 0 && b[offset+2] == 0 &&
				b[offset+3] == 0) {
			return 0xff
		}
		offset += 4
	case additional == 27:
		if len(b) < offset+8 {
			return 0xff
		}
		empty := true
		for i := 0; i < 8; i++ {
			if b[offset+i] != 0 {
				empty = false
				break
			}
		}
		if empty {
			return 0xff
		}
		offset += 8
	default:
		return 0xff
	}
	if len(b) <= offset {
		return 0xff
	}
	return b[offset] & cborTypeMask
}

func decodeTag259Content(raw []byte) ([]byte, bool) {
	if len(raw) < 3 || (raw[0]&cborTypeMask) != cborTypeTag {
		return nil, false
	}
	switch raw[0] & cborAdditionalMask {
	case 25:
		if raw[1] == 0x01 && raw[2] == 0x03 {
			return raw[3:], true
		}
	}
	return nil, false
}

func decodeAuxiliaryMetadataOnly(content []byte) ([]byte, bool) {
	if len(content) < 2 || (content[0]&cborTypeMask) != cborTypeMap {
		return nil, false
	}
	count, offset, ok := decodeCBORDefiniteLength(content, 0, cborTypeMap)
	if !ok || count != 1 {
		return nil, false
	}
	if offset >= len(content) || content[offset] != 0x00 {
		return nil, false
	}
	offset++
	metadataEnd, ok := decodeCBORItemEnd(content, offset)
	if !ok || metadataEnd != len(content) {
		return nil, false
	}
	return content[offset:metadataEnd], true
}

func decodeCBORItemEnd(b []byte, offset int) (int, bool) {
	if offset >= len(b) {
		return offset, false
	}
	majorType := b[offset] & cborTypeMask
	additional := b[offset] & cborAdditionalMask
	offset++
	switch majorType {
	case cborTypeUnsigned, cborTypeNegative:
		switch {
		case additional <= 23:
			return offset, true
		case additional == 24:
			return offset + 1, offset < len(b)
		case additional == 25:
			return offset + 2, offset+1 < len(b)
		case additional == 26:
			return offset + 4, offset+3 < len(b)
		case additional == 27:
			return offset + 8, offset+7 < len(b)
		default:
			return offset, false
		}
	case cborTypeByteString, cborTypeTextString:
		length, nextOffset, ok := decodeCBORDefiniteLength(b, offset-1, majorType)
		if !ok || nextOffset+length > len(b) {
			return offset, false
		}
		return nextOffset + length, true
	case cborTypeArray:
		count, nextOffset, ok := decodeCBORDefiniteLength(b, offset-1, cborTypeArray)
		if !ok {
			return offset, false
		}
		cur := nextOffset
		for range count {
			var ok bool
			cur, ok = decodeCBORItemEnd(b, cur)
			if !ok {
				return offset, false
			}
		}
		return cur, true
	case cborTypeMap:
		count, nextOffset, ok := decodeCBORDefiniteLength(b, offset-1, cborTypeMap)
		if !ok {
			return offset, false
		}
		cur := nextOffset
		for range count {
			var ok bool
			cur, ok = decodeCBORItemEnd(b, cur)
			if !ok {
				return offset, false
			}
			cur, ok = decodeCBORItemEnd(b, cur)
			if !ok {
				return offset, false
			}
		}
		return cur, true
	case cborTypeTag:
		switch {
		case additional <= 23:
		case additional == 24:
			if offset >= len(b) {
				return offset, false
			}
			offset++
		case additional == 25:
			if offset+1 >= len(b) {
				return offset, false
			}
			offset += 2
		case additional == 26:
			if offset+3 >= len(b) {
				return offset, false
			}
			offset += 4
		case additional == 27:
			if offset+7 >= len(b) {
				return offset, false
			}
			offset += 8
		default:
			return offset, false
		}
		return decodeCBORItemEnd(b, offset)
	case cborTypeFloatSim:
		switch additional {
		case 20, 21, 22, 23:
			return offset, true
		case 24:
			return offset + 1, offset < len(b)
		case 25:
			return offset + 2, offset+1 < len(b)
		case 26:
			return offset + 4, offset+3 < len(b)
		case 27:
			return offset + 8, offset+7 < len(b)
		default:
			return offset, false
		}
	default:
		return offset, false
	}
}

func decodeMapUint(b []byte) (TransactionMetadatum, bool, error) {
	var m map[uint]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	for k, rv := range m {
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			return nil, true, fmt.Errorf("decode map(uint) value: %w", err)
		}
		pairs = append(
			pairs,
			MetaPair{
				Key:   MetaInt{Value: new(big.Int).SetUint64(uint64(k))},
				Value: val,
			},
		)
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true, nil
}

func decodeMapTextText(b []byte) (TransactionMetadatum, bool) {
	if md, ok := decodeMapTextTextFast(b); ok {
		return md, true
	}
	var m map[string]string
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	for k, v := range m {
		pairs = append(
			pairs,
			MetaPair{
				Key:   MetaText{Value: k},
				Value: MetaText{Value: v},
			},
		)
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true
}

func decodeMapTextTextFast(b []byte) (TransactionMetadatum, bool) {
	if len(b) == 0 || (b[0]&cborTypeMask) != cborTypeMap {
		return nil, false
	}
	count, offset, ok := decodeCBORDefiniteLength(b, 0, cborTypeMap)
	if !ok {
		return nil, false
	}
	pairs := make([]MetaPair, 0, count)
	for range count {
		key, nextOffset, ok := decodeCBORTextString(b, offset)
		if !ok {
			return nil, false
		}
		val, nextOffset, ok := decodeCBORTextString(b, nextOffset)
		if !ok {
			return nil, false
		}
		pairs = append(pairs, MetaPair{
			Key:   MetaText{Value: key},
			Value: MetaText{Value: val},
		})
		offset = nextOffset
	}
	if offset != len(b) {
		return nil, false
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true
}

func decodeCBORDefiniteLength(
	b []byte,
	offset int,
	expectedType byte,
) (int, int, bool) {
	if offset >= len(b) || (b[offset]&cborTypeMask) != expectedType {
		return 0, offset, false
	}
	additional := b[offset] & cborAdditionalMask
	offset++
	switch {
	case additional <= 23:
		return int(additional), offset, true
	case additional == 24:
		if offset >= len(b) {
			return 0, offset, false
		}
		return int(b[offset]), offset + 1, true
	case additional == 25:
		if offset+1 >= len(b) {
			return 0, offset, false
		}
		return int(b[offset])<<8 | int(b[offset+1]), offset + 2, true
	default:
		return 0, offset, false
	}
}

func decodeCBORTextString(b []byte, offset int) (string, int, bool) {
	length, offset, ok := decodeCBORDefiniteLength(b, offset, cborTypeTextString)
	if !ok || offset+length > len(b) {
		return "", offset, false
	}
	return string(b[offset : offset+length]), offset + length, true
}

func decodeMapInt(b []byte) (TransactionMetadatum, bool, error) {
	var m map[int]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	for k, rv := range m {
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			return nil, true, fmt.Errorf("decode map(int) value: %w", err)
		}
		pairs = append(
			pairs,
			MetaPair{
				Key:   MetaInt{Value: big.NewInt(int64(k))},
				Value: val,
			},
		)
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true, nil
}

func decodeMapText(b []byte) (TransactionMetadatum, bool, error) {
	var m map[string]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	for k, rv := range m {
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			return nil, true, fmt.Errorf("decode map(text) value: %w", err)
		}
		pairs = append(pairs, MetaPair{Key: MetaText{Value: k}, Value: val})
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true, nil
}

func decodeMapBytes(b []byte) (TransactionMetadatum, bool, error) {
	var m map[cbor.ByteString]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	for k, rv := range m {
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			return nil, true, fmt.Errorf("decode map(bytes) value: %w", err)
		}

		bs := k.Bytes()
		pairs = append(pairs, MetaPair{
			Key:   MetaBytes{Value: slices.Clone(bs)},
			Value: val,
		})
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true, nil
}

func decodeMapGeneric(b []byte) (TransactionMetadatum, bool, error) {
	// Use *cbor.Value for keys to handle any CBOR type, including types
	// that are not comparable in Go (maps, arrays) which cannot be used
	// as keys in map[any]. Pointer keys are always comparable.
	var m map[*cbor.Value]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	for k, rv := range m {
		keyMd, err := DecodeMetadatumRaw(k.Cbor())
		if err != nil {
			return nil, true, fmt.Errorf("decode map key: %w", err)
		}
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			return nil, true, fmt.Errorf("decode map(generic) value: %w", err)
		}
		pairs = append(pairs, MetaPair{Key: keyMd, Value: val})
	}
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true, nil
}

func DecodeAuxiliaryDataToMetadata(raw []byte) (TransactionMetadatum, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty auxiliary data")
	}
	typeByte := raw[0] & cborTypeMask
	switch typeByte {
	case cborTypeMap:
		// Direct metadata
		return DecodeMetadatumRaw(raw)
	case cborTypeArray:
		// auxiliary_data_array = [transaction_metadata, auxiliary_scripts]
		var arr []cbor.RawMessage
		if _, err := cbor.Decode(raw, &arr); err != nil {
			return nil, err
		}
		if len(arr) != 2 {
			return nil, errors.New("auxiliary_data_array must have 2 elements")
		}
		// First element is metadata - check for null
		if len(arr[0]) == 1 && arr[0][0] == 0xF6 {
			// CBOR null means no metadata
			return nil, nil
		}
		return DecodeMetadatumRaw(arr[0])
	case cborTypeTag:
		// auxiliary_data_map = #6.259({ ? 0 : metadata, ... })
		taggedContent, ok := decodeTag259Content(raw)
		if !ok {
			var tmpTag cbor.RawTag
			if _, err := cbor.Decode(raw, &tmpTag); err != nil {
				return nil, err
			}
			if tmpTag.Number != cbor.CborTagMap {
				return nil, fmt.Errorf(
					"expected CBOR tag %d for auxiliary_data_map, got %d",
					cbor.CborTagMap,
					tmpTag.Number,
				)
			}
			taggedContent = tmpTag.Content
		}
		if metadataRaw, ok := decodeAuxiliaryMetadataOnly(taggedContent); ok {
			return DecodeMetadatumRaw(metadataRaw)
		}
		var auxMap map[uint]cbor.RawMessage
		if _, err := cbor.Decode(taggedContent, &auxMap); err != nil {
			return nil, err
		}
		if metadataRaw := auxMap[0]; len(metadataRaw) > 0 {
			return DecodeMetadatumRaw(metadataRaw)
		}
		// If no metadata, return nil
		return nil, nil
	default:
		return nil, fmt.Errorf(
			"unsupported auxiliary_data type: 0x%x",
			typeByte,
		)
	}
}

func (s *TransactionMetadataSet) UnmarshalCBOR(cborData []byte) error {
	s.SetCbor(cborData)
	s.data = make(map[uint]cbor.RawMessage)
	if _, err := cbor.Decode(cborData, &s.data); err != nil {
		return err
	}
	s.metadata = make(map[uint]TransactionMetadatum)
	for k, raw := range s.data {
		if len(raw) == 0 {
			continue
		}
		md, err := DecodeAuxiliaryDataToMetadata(raw)
		if err != nil {
			return fmt.Errorf(
				"failed to decode metadata for key %d: %w",
				k,
				err,
			)
		}
		if md != nil {
			s.metadata[k] = md
		}
	}
	return nil
}

func (s TransactionMetadataSet) MarshalCBOR() ([]byte, error) {
	// Return stored CBOR if available to preserve received encoding for fee calculations,
	// otherwise encode canonically
	if len(s.Cbor()) > 0 {
		return s.Cbor(), nil
	}
	return cbor.Encode(s.data)
}

func (s TransactionMetadataSet) GetMetadata(
	key uint,
) (TransactionMetadatum, bool) {
	val, ok := s.metadata[key]
	return val, ok
}

func (s TransactionMetadataSet) GetRawMetadata(
	key uint,
) (cbor.RawMessage, bool) {
	val, ok := s.data[key]
	return val, ok
}

type AuxiliaryData interface {
	// Metadata returns the transaction metadata, if present
	Metadata() (TransactionMetadatum, error)
	// NativeScripts returns the native scripts, if present
	NativeScripts() ([]NativeScript, error)
	// PlutusV1Scripts returns the Plutus V1 scripts, if present (Alonzo+ only)
	PlutusV1Scripts() ([]PlutusV1Script, error)
	// PlutusV2Scripts returns the Plutus V2 scripts, if present (Alonzo+ only)
	PlutusV2Scripts() ([]PlutusV2Script, error)
	// PlutusV3Scripts returns the Plutus V3 scripts, if present (Conway+ only)
	PlutusV3Scripts() ([]PlutusV3Script, error)
	// Cbor returns the raw CBOR encoding
	Cbor() []byte
}

type ShelleyAuxiliaryData struct {
	cbor.DecodeStoreCbor
	metadata TransactionMetadatum
}

func (s *ShelleyAuxiliaryData) Metadata() (TransactionMetadatum, error) {
	return s.metadata, nil
}

func (s *ShelleyAuxiliaryData) NativeScripts() ([]NativeScript, error) {
	return nil, nil
}

func (s *ShelleyAuxiliaryData) PlutusV1Scripts() ([]PlutusV1Script, error) {
	return nil, nil
}

func (s *ShelleyAuxiliaryData) PlutusV2Scripts() ([]PlutusV2Script, error) {
	return nil, nil
}

func (s *ShelleyAuxiliaryData) PlutusV3Scripts() ([]PlutusV3Script, error) {
	return nil, nil
}

func (s *ShelleyAuxiliaryData) UnmarshalCBOR(data []byte) error {
	s.SetCbor(data)

	// Shelley auxiliary data may be wrapped in CBOR tag 259 (0xD90103)
	// If present, we need to skip the tag and extract the inner content
	// before passing to DecodeMetadatumRaw (which doesn't handle tags)
	dataToUse := data
	if len(data) >= 3 && data[0] == 0xD9 && data[1] == 0x01 && data[2] == 0x03 {
		// Tag 259 (0xD90103) detected, skip the 3-byte tag header
		dataToUse = data[3:]
	}

	md, err := DecodeMetadatumRaw(dataToUse)
	if err != nil {
		return fmt.Errorf("failed to decode Shelley auxiliary data: %w", err)
	}
	s.metadata = md
	return nil
}

func (s ShelleyAuxiliaryData) MarshalCBOR() ([]byte, error) {
	if raw := s.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if s.metadata == nil {
		return cbor.Encode(nil)
	}
	return cbor.Encode(s.metadata)
}

type ShelleyMaAuxiliaryData struct {
	cbor.DecodeStoreCbor
	metadata      TransactionMetadatum
	nativeScripts []NativeScript
}

func (s *ShelleyMaAuxiliaryData) Metadata() (TransactionMetadatum, error) {
	return s.metadata, nil
}

func (s *ShelleyMaAuxiliaryData) NativeScripts() ([]NativeScript, error) {
	return s.nativeScripts, nil
}

func (s *ShelleyMaAuxiliaryData) PlutusV1Scripts() ([]PlutusV1Script, error) {
	return nil, nil
}

func (s *ShelleyMaAuxiliaryData) PlutusV2Scripts() ([]PlutusV2Script, error) {
	return nil, nil
}

func (s *ShelleyMaAuxiliaryData) PlutusV3Scripts() ([]PlutusV3Script, error) {
	return nil, nil
}

func (s *ShelleyMaAuxiliaryData) UnmarshalCBOR(data []byte) error {
	s.SetCbor(data)
	var arr []cbor.RawMessage
	if _, err := cbor.Decode(data, &arr); err != nil {
		return fmt.Errorf(
			"failed to decode Shelley-MA auxiliary data array: %w",
			err,
		)
	}
	if len(arr) != 2 {
		return fmt.Errorf(
			"Shelley-MA auxiliary data must have 2 elements, got %d",
			len(arr),
		)
	}

	// First element is metadata (may be null)
	if len(arr[0]) > 0 && arr[0][0] != 0xF6 { // 0xF6 is CBOR null
		md, err := DecodeMetadatumRaw(arr[0])
		if err != nil {
			return fmt.Errorf("failed to decode metadata: %w", err)
		}
		s.metadata = md
	}

	// Second element is array of native scripts
	if _, err := cbor.Decode(arr[1], &s.nativeScripts); err != nil {
		return fmt.Errorf("failed to decode native scripts: %w", err)
	}

	return nil
}

func (s ShelleyMaAuxiliaryData) MarshalCBOR() ([]byte, error) {
	if raw := s.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode([]any{s.metadata, s.nativeScripts})
}

type AlonzoAuxiliaryData struct {
	cbor.DecodeStoreCbor
	metadata        TransactionMetadatum
	nativeScripts   []NativeScript
	plutusV1Scripts []PlutusV1Script
	plutusV2Scripts []PlutusV2Script
	plutusV3Scripts []PlutusV3Script
}

func (a *AlonzoAuxiliaryData) Metadata() (TransactionMetadatum, error) {
	return a.metadata, nil
}

func (a *AlonzoAuxiliaryData) NativeScripts() ([]NativeScript, error) {
	return a.nativeScripts, nil
}

func (a *AlonzoAuxiliaryData) PlutusV1Scripts() ([]PlutusV1Script, error) {
	return a.plutusV1Scripts, nil
}

func (a *AlonzoAuxiliaryData) PlutusV2Scripts() ([]PlutusV2Script, error) {
	return a.plutusV2Scripts, nil
}

func (a *AlonzoAuxiliaryData) PlutusV3Scripts() ([]PlutusV3Script, error) {
	return a.plutusV3Scripts, nil
}

func (a *AlonzoAuxiliaryData) UnmarshalCBOR(data []byte) error {
	a.SetCbor(data)

	taggedContent, ok := decodeTag259Content(data)
	if !ok {
		// Decode CBOR tag 259 via the generic path for malformed or
		// non-standard encodings.
		var tmpTag cbor.RawTag
		if _, err := cbor.Decode(data, &tmpTag); err != nil {
			return fmt.Errorf("failed to decode Alonzo auxiliary data tag: %w", err)
		}
		if tmpTag.Number != cbor.CborTagMap {
			return fmt.Errorf(
				"expected CBOR tag %d for Alonzo auxiliary data, got %d",
				cbor.CborTagMap,
				tmpTag.Number,
			)
		}
		taggedContent = tmpTag.Content
	}
	if metadataRaw, ok := decodeAuxiliaryMetadataOnly(taggedContent); ok {
		md, err := DecodeMetadatumRaw(metadataRaw)
		if err != nil {
			return fmt.Errorf("failed to decode metadata: %w", err)
		}
		a.metadata = md
		return nil
	}

	var auxMap map[uint]cbor.RawMessage
	if _, err := cbor.Decode(taggedContent, &auxMap); err != nil {
		return fmt.Errorf("failed to decode auxiliary data map: %w", err)
	}

	// Key 0: metadata
	if metadataRaw := auxMap[0]; len(metadataRaw) > 0 {
		md, err := DecodeMetadatumRaw(metadataRaw)
		if err != nil {
			return fmt.Errorf("failed to decode metadata: %w", err)
		}
		a.metadata = md
	}

	// Key 1: native scripts
	if nativeScriptsRaw := auxMap[1]; len(nativeScriptsRaw) > 0 {
		if _, err := cbor.Decode(nativeScriptsRaw, &a.nativeScripts); err != nil {
			return fmt.Errorf("failed to decode native scripts: %w", err)
		}
	}

	// Key 2: Plutus V1 scripts
	if plutusV1Raw := auxMap[2]; len(plutusV1Raw) > 0 {
		if _, err := cbor.Decode(plutusV1Raw, &a.plutusV1Scripts); err != nil {
			return fmt.Errorf("failed to decode Plutus V1 scripts: %w", err)
		}
	}

	// Key 3: Plutus V2 scripts
	if plutusV2Raw := auxMap[3]; len(plutusV2Raw) > 0 {
		if _, err := cbor.Decode(plutusV2Raw, &a.plutusV2Scripts); err != nil {
			return fmt.Errorf("failed to decode Plutus V2 scripts: %w", err)
		}
	}

	// Key 4: Plutus V3 scripts
	if plutusV3Raw := auxMap[4]; len(plutusV3Raw) > 0 {
		if _, err := cbor.Decode(plutusV3Raw, &a.plutusV3Scripts); err != nil {
			return fmt.Errorf("failed to decode Plutus V3 scripts: %w", err)
		}
	}

	return nil
}

func (a AlonzoAuxiliaryData) MarshalCBOR() ([]byte, error) {
	if raw := a.Cbor(); len(raw) > 0 {
		return raw, nil
	}

	auxMap := make(map[uint]cbor.RawMessage)
	if a.metadata != nil {
		metaCbor, err := cbor.Encode(a.metadata)
		if err != nil {
			return nil, err
		}
		auxMap[0] = metaCbor
	}
	if len(a.nativeScripts) > 0 {
		scriptsCbor, err := cbor.Encode(a.nativeScripts)
		if err != nil {
			return nil, err
		}
		auxMap[1] = scriptsCbor
	}
	if len(a.plutusV1Scripts) > 0 {
		scriptsCbor, err := cbor.Encode(a.plutusV1Scripts)
		if err != nil {
			return nil, err
		}
		auxMap[2] = scriptsCbor
	}
	if len(a.plutusV2Scripts) > 0 {
		scriptsCbor, err := cbor.Encode(a.plutusV2Scripts)
		if err != nil {
			return nil, err
		}
		auxMap[3] = scriptsCbor
	}
	if len(a.plutusV3Scripts) > 0 {
		scriptsCbor, err := cbor.Encode(a.plutusV3Scripts)
		if err != nil {
			return nil, err
		}
		auxMap[4] = scriptsCbor
	}

	// Encode the map directly, not the bytes
	mapBytes, err := cbor.Encode(&auxMap)
	if err != nil {
		return nil, err
	}

	// Create a raw tag with the map bytes as content
	var tmpTag cbor.RawTag
	tmpTag.Number = cbor.CborTagMap
	tmpTag.Content = mapBytes

	return cbor.Encode(&tmpTag)
}

func DecodeAuxiliaryData(raw []byte) (AuxiliaryData, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty auxiliary data")
	}

	typeByte := raw[0] & cborTypeMask
	switch typeByte {
	case cborTypeMap:
		// Shelley format: direct metadata
		auxData := &ShelleyAuxiliaryData{}
		if _, err := cbor.Decode(raw, auxData); err != nil {
			return nil, err
		}
		return auxData, nil

	case cborTypeArray:
		// Shelley-MA format: [metadata, native_scripts]
		auxData := &ShelleyMaAuxiliaryData{}
		if _, err := cbor.Decode(raw, auxData); err != nil {
			return nil, err
		}
		return auxData, nil

	case cborTypeTag:
		// Alonzo+ format: #6.259({...})
		auxData := &AlonzoAuxiliaryData{}
		if _, err := cbor.Decode(raw, auxData); err != nil {
			return nil, err
		}
		return auxData, nil

	default:
		return nil, fmt.Errorf(
			"unsupported auxiliary data type: 0x%x",
			typeByte,
		)
	}
}
