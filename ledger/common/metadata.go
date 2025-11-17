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
	"math"

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

type TransactionMetadataSet map[uint]TransactionMetadatum

type TransactionMetadatum interface {
	isTransactionMetadatum()
	TypeName() string
}

type MetaInt struct{ Value int64 }

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
		var n int64
		if _, err := cbor.Decode(b, &n); err != nil {
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
		if md, ok, err := decodeMapText(b); ok || err != nil {
			return md, err
		}
		if md, ok, err := decodeMapBytes(b); ok || err != nil {
			return md, err
		}
		return nil, errors.New("unsupported map key type in metadatum")

	case cborTypeTag, cborTypeFloatSim:
		var x any
		if _, err := cbor.Decode(b, &x); err != nil {
			return nil, err
		}
		return MetaText{Value: fmt.Sprintf("%v", x)}, nil

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
		if k > math.MaxInt64 {
			return nil, true, fmt.Errorf("metadata label %d exceeds int64", k)
		}
		pairs = append(pairs, MetaPair{Key: MetaInt{Value: int64(k)}, Value: val})
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

func (s *TransactionMetadataSet) UnmarshalCBOR(cborData []byte) error {

	// Map form: map[uint]cbor.RawMessage
	{
		var tmp map[uint]cbor.RawMessage
		if _, err := cbor.Decode(cborData, &tmp); err == nil {
			out := make(TransactionMetadataSet, len(tmp))
			for k, v := range tmp {
				md, err := DecodeMetadatumRaw(v)
				if err != nil {
					return fmt.Errorf("decode metadata value for index %d: %w", k, err)
				}
				out[k] = md
			}
			*s = out
			return nil
		}
	}
	// Array form: []cbor.RawMessage  (nulls are skipped)
	{
		var arr []cbor.RawMessage
		if _, err := cbor.Decode(cborData, &arr); err == nil {
			out := make(TransactionMetadataSet)
			for i, raw := range arr {
				var probe any
				if _, err := cbor.Decode(raw, &probe); err == nil && probe == nil {
					continue // skip nulls
				}
				md, err := DecodeMetadatumRaw(raw)
				if err != nil {
					return fmt.Errorf("decode metadata list item %d: %w", i, err)
				}
				out[uint(i)] = md // #nosec G115
			}
			*s = out
			return nil
		}
	}
	return errors.New("unsupported TransactionMetadataSet encoding")
}

func (s TransactionMetadataSet) MarshalCBOR() ([]byte, error) {
	if s == nil {
		return cbor.Encode(&map[uint]any{})
	}
	tmpMap := make(map[uint]any, len(s))
	for k, v := range s {
		tmpMap[k] = metadatumToInterface(v)
	}
	return cbor.Encode(&tmpMap)
}

func metadatumToInterface(m TransactionMetadatum) any {
	switch t := m.(type) {
	case MetaInt:
		return t.Value
	case MetaBytes:
		return t.Value
	case MetaText:
		return t.Value
	case MetaList:
		out := make([]any, 0, len(t.Items))
		for _, it := range t.Items {
			out = append(out, metadatumToInterface(it))
		}
		return out
	case MetaMap:
		allText := true
		for _, p := range t.Pairs {
			if _, ok := p.Key.(MetaText); !ok {
				allText = false
				break
			}
		}
		if allText {
			mm := make(map[string]any, len(t.Pairs))
			for _, p := range t.Pairs {
				mm[p.Key.(MetaText).Value] = metadatumToInterface(p.Value)
			}
			return mm
		}
		// Try all-int keys
		allInt := true
		for _, p := range t.Pairs {
			if _, ok := p.Key.(MetaInt); !ok {
				allInt = false
				break
			}
		}
		if allInt {
			mm := make(map[int64]any, len(t.Pairs))
			for _, p := range t.Pairs {
				mm[p.Key.(MetaInt).Value] = metadatumToInterface(p.Value)
			}
			return mm
		}

		allBytes := true
		for _, p := range t.Pairs {
			if _, ok := p.Key.(MetaBytes); !ok {
				allBytes = false
				break
			}
		}
		if allBytes {
			mm := make(map[cbor.ByteString]any, len(t.Pairs))
			for _, p := range t.Pairs {
				bs := p.Key.(MetaBytes).Value
				key := cbor.NewByteString(bs)
				mm[key] = metadatumToInterface(p.Value)
			}
			return mm
		}
		return nil

	default:
		return nil
	}
}
