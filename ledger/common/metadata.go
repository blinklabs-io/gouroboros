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

package common

import (
	"errors"
	"fmt"
	"math/big"

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
)

type TransactionMetadataSet struct {
	cbor.DecodeStoreCbor
	data     map[uint]cbor.RawMessage
	metadata map[uint]TransactionMetadatum
}

type TransactionMetadatum interface {
	isTransactionMetadatum()
	TypeName() string
}

type MetaInt struct{ Value *big.Int }

type MetaBytes struct{ Value []byte }

type MetaText struct{ Value string }

type MetaList struct {
	Items []TransactionMetadatum
}

type MetaPair struct {
	Key   TransactionMetadatum
	Value TransactionMetadatum
}

type MetaMap struct {
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
		return MetaInt{Value: n}, nil

	case cborTypeTextString:
		var s string
		if _, err := cbor.Decode(b, &s); err != nil {
			return nil, err
		}
		return MetaText{Value: s}, nil

	case cborTypeByteString:
		var bs []byte
		if _, err := cbor.Decode(b, &bs); err != nil {
			return nil, err
		}
		return MetaBytes{Value: bs}, nil

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
		return MetaList{Items: items}, nil

	case cborTypeMap:
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
		if md, ok, err := decodeMapGeneric(b); ok || err != nil {
			return md, err
		}
		return nil, fmt.Errorf(
			"unsupported map key type in metadatum: CBOR major type 0x%x",
			b[0]&cborTypeMask,
		)

	case cborTypeTag, cborTypeFloatSim:
		return nil, fmt.Errorf(
			"unsupported CBOR major type 0x%x in metadata",
			b[0]&cborTypeMask,
		)

	default:
		return nil, errors.New("unknown CBOR major type")
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
	return MetaMap{Pairs: pairs}, true, nil
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
	return MetaMap{Pairs: pairs}, true, nil
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
	return MetaMap{Pairs: pairs}, true, nil
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
			Key:   MetaBytes{Value: append([]byte(nil), bs...)},
			Value: val,
		})
	}
	return MetaMap{Pairs: pairs}, true, nil
}

func decodeMapGeneric(b []byte) (TransactionMetadatum, bool, error) {
	// Try to decode as a generic map to handle unsupported key types
	var m map[interface{}]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	var errors []error
	for k, rv := range m {
		// Try to convert the key to a TransactionMetadatum
		keyBytes, err := cbor.Encode(k)
		if err != nil {
			errors = append(errors, fmt.Errorf("encode map key %v: %w", k, err))
			continue
		}
		keyMd, err := DecodeMetadatumRaw(keyBytes)
		if err != nil {
			// If key can't be decoded as metadatum, skip this pair
			errors = append(errors, fmt.Errorf("decode map key %v: %w", k, err))
			continue
		}
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			errors = append(errors, fmt.Errorf("decode map(generic) value for key %v: %w", k, err))
			continue
		}
		pairs = append(pairs, MetaPair{Key: keyMd, Value: val})
	}
	if len(pairs) > 0 {
		return MetaMap{Pairs: pairs}, true, nil
	}
	if len(errors) > 0 {
		return nil, true, fmt.Errorf("failed to decode all map pairs: %v", errors)
	}
	return MetaMap{Pairs: pairs}, true, nil
}

func decodeAuxiliaryDataToMetadata(raw []byte) (TransactionMetadatum, error) {
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
		var m map[uint]cbor.RawMessage
		if _, err := cbor.Decode(tmpTag.Content, &m); err != nil {
			return nil, err
		}
		// Key 0 is metadata
		if metadataRaw, ok := m[0]; ok {
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
		md, err := decodeAuxiliaryDataToMetadata(raw)
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
