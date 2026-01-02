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
	mm := MetaMap{Pairs: pairs}
	mm.SetCbor(b)
	return mm, true, nil
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
	// Try to decode as a generic map to handle unsupported key types
	var m map[any]cbor.RawMessage
	if _, err := cbor.Decode(b, &m); err != nil {
		return nil, false, nil //nolint:nilerr // not this shape
	}
	pairs := make([]MetaPair, 0, len(m))
	var errs []error
	for k, rv := range m {
		// Try to convert the key to a TransactionMetadatum
		keyBytes, err := cbor.Encode(k)
		if err != nil {
			errs = append(errs, fmt.Errorf("encode map key %v: %w", k, err))
			continue
		}
		keyMd, err := DecodeMetadatumRaw(keyBytes)
		if err != nil {
			// If key can't be decoded as metadatum, skip this pair
			errs = append(errs, fmt.Errorf("decode map key %v: %w", k, err))
			continue
		}
		val, err := DecodeMetadatumRaw(rv)
		if err != nil {
			errs = append(
				errs,
				fmt.Errorf("decode map(generic) value for key %v: %w", k, err),
			)
			continue
		}
		pairs = append(pairs, MetaPair{Key: keyMd, Value: val})
	}
	if len(pairs) > 0 {
		mm := MetaMap{Pairs: pairs}
		mm.SetCbor(b)
		// Partial success: Some pairs decoded successfully, others failed.
		// Return the successfully decoded pairs with no error to preserve valid data.
		// This allows clients to work with the valid metadata while silently skipping
		// invalid pairs. The errs slice is not returned - only fully valid maps or
		// complete decoding failures are reported. This behavior is intentional and
		// prioritizes data availability over strictness.
		return mm, true, nil
	}
	if len(errs) > 0 {
		return nil, true, fmt.Errorf(
			"failed to decode all map pairs: %w",
			errors.Join(errs...),
		)
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
	md, err := DecodeMetadatumRaw(data)
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

	// Decode CBOR tag 259
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

	// Decode the map
	var auxMap map[uint]cbor.RawMessage
	if _, err := cbor.Decode(tmpTag.Content, &auxMap); err != nil {
		return fmt.Errorf("failed to decode auxiliary data map: %w", err)
	}

	// Key 0: metadata
	if metadataRaw, ok := auxMap[0]; ok {
		md, err := DecodeMetadatumRaw(metadataRaw)
		if err != nil {
			return fmt.Errorf("failed to decode metadata: %w", err)
		}
		a.metadata = md
	}

	// Key 1: native scripts
	if scriptsRaw, ok := auxMap[1]; ok {
		if _, err := cbor.Decode(scriptsRaw, &a.nativeScripts); err != nil {
			return fmt.Errorf("failed to decode native scripts: %w", err)
		}
	}

	// Key 2: Plutus V1 scripts
	if scriptsRaw, ok := auxMap[2]; ok {
		if _, err := cbor.Decode(scriptsRaw, &a.plutusV1Scripts); err != nil {
			return fmt.Errorf("failed to decode Plutus V1 scripts: %w", err)
		}
	}

	// Key 3: Plutus V2 scripts
	if scriptsRaw, ok := auxMap[3]; ok {
		if _, err := cbor.Decode(scriptsRaw, &a.plutusV2Scripts); err != nil {
			return fmt.Errorf("failed to decode Plutus V2 scripts: %w", err)
		}
	}

	// Key 4: Plutus V3 scripts
	if scriptsRaw, ok := auxMap[4]; ok {
		if _, err := cbor.Decode(scriptsRaw, &a.plutusV3Scripts); err != nil {
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
