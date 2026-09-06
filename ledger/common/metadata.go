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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"strings"
	"unicode/utf8"

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

	// cborBreak terminates an indefinite-length item
	cborBreak byte = 0xff
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

// DecodeMetadatumRaw decodes a transaction metadatum from its CBOR encoding.
//
// The decode is single pass: the initial byte of each item is read once, each
// node references a subslice of b rather than a copy, and no nested item is
// scanned more than once. That matters because the reference decoder places no
// bound on metadatum nesting (see cbor.MaxNestedLevels in the cbor package for
// the reference and the derivation), so decode cost has to stay linear in the
// size of the value rather than growing with its depth.
//
// The input is copied before decoding so each node owns the bytes returned by
// Cbor() and remains stable if the caller reuses b.
func DecodeMetadatumRaw(b []byte) (TransactionMetadatum, error) {
	b = slices.Clone(b)
	md, n, err := decodeMetadatumAt(b, 0, 0)
	if err != nil {
		return nil, err
	}
	if n != len(b) {
		return nil, fmt.Errorf(
			"extraneous data after metadatum: %d byte(s)",
			len(b)-n,
		)
	}
	return md, nil
}

// cborItemHead reads the initial byte of a CBOR item at offset along with its
// argument. It returns the major type, the argument value, the offset of the
// first byte after the head, and whether the item uses the indefinite-length
// form.
func cborItemHead(
	b []byte,
	offset int,
) (major byte, arg uint64, next int, indefinite bool, err error) {
	if offset < 0 || offset >= len(b) {
		return 0, 0, 0, false, io.ErrUnexpectedEOF
	}
	major = b[offset] & cborTypeMask
	additional := b[offset] & cborAdditionalMask
	offset++
	switch {
	case additional < 24:
		return major, uint64(additional), offset, false, nil
	case additional == 31:
		return major, 0, offset, true, nil
	case additional >= 28:
		return 0, 0, 0, false, fmt.Errorf(
			"invalid CBOR additional information %d in metadata",
			additional,
		)
	}
	width := 1 << (additional - 24)
	if len(b)-offset < width {
		return 0, 0, 0, false, io.ErrUnexpectedEOF
	}
	for i := range width {
		arg = arg<<8 | uint64(b[offset+i])
	}
	return major, arg, offset + width, false, nil
}

// decodeMetadatumStringAt decodes a definite- or indefinite-length byte or text
// string starting at offset, returning its bytes and the offset just past it.
func decodeMetadatumStringAt(
	b []byte,
	major byte,
	arg uint64,
	next int,
	indefinite bool,
) ([]byte, int, error) {
	if !indefinite {
		remaining := len(b) - next
		//nolint:gosec // next is at most len(b), so remaining is non-negative
		if remaining < 0 || arg > uint64(remaining) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		end := next + int(arg) //nolint:gosec // bounded by remaining above
		return b[next:end], end, nil
	}
	// An indefinite-length string is a sequence of definite-length chunks of
	// the same major type, terminated by a break.
	var chunks []byte
	pos := next
	for {
		if pos >= len(b) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		if b[pos] == cborBreak {
			return chunks, pos + 1, nil
		}
		chunkMajor, chunkArg, chunkNext, chunkIndef, err := cborItemHead(b, pos)
		if err != nil {
			return nil, 0, err
		}
		if chunkMajor != major || chunkIndef {
			return nil, 0, errors.New(
				"invalid chunk in indefinite-length string in metadata",
			)
		}
		remaining := len(b) - chunkNext
		//nolint:gosec // chunkNext is at most len(b), so remaining is non-negative
		if remaining < 0 || chunkArg > uint64(remaining) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		end := chunkNext + int(chunkArg) //nolint:gosec // bounded above
		if major == cborTypeTextString && !utf8.Valid(b[chunkNext:end]) {
			return nil, 0, errors.New("invalid UTF-8 in metadata text string")
		}
		chunks = append(chunks, b[chunkNext:end]...)
		pos = end
	}
}

// metadatumKeyIdentity returns a comparable identity for a map key, used to
// reject duplicate keys. Scalars compare by value so that two encodings of the
// same number collide, matching the duplicate-key enforcement the CBOR decode
// modes apply elsewhere; containers compare by their decoded semantics.
func metadatumKeyIdentity(md TransactionMetadatum) string {
	switch k := md.(type) {
	case MetaInt:
		return "i:" + k.Value.String()
	case MetaText:
		return fmt.Sprintf("t:%d:%s", len(k.Value), k.Value)
	case MetaBytes:
		return fmt.Sprintf("b:%d:%x", len(k.Value), k.Value)
	case MetaList:
		parts := make([]string, len(k.Items))
		for i, item := range k.Items {
			identity := metadatumKeyIdentity(item)
			parts[i] = fmt.Sprintf("%d:%s", len(identity), identity)
		}
		return "l:" + strings.Join(parts, "")
	case MetaMap:
		parts := make([]string, len(k.Pairs))
		for i, pair := range k.Pairs {
			key := metadatumKeyIdentity(pair.Key)
			value := metadatumKeyIdentity(pair.Value)
			parts[i] = fmt.Sprintf("%d:%s%d:%s", len(key), key, len(value), value)
		}
		slices.Sort(parts)
		return "m:" + strings.Join(parts, "")
	default:
		return "r:" + hex.EncodeToString(md.Cbor())
	}
}

// decodeMetadatumAt decodes the metadatum starting at offset and returns it
// along with the offset just past its final byte.
func decodeMetadatumAt(
	b []byte,
	offset int,
	depth int,
) (TransactionMetadatum, int, error) {
	// The reference decoder has no depth bound, but this one recurses on the
	// Go stack, so it holds the same bound the CBOR decode modes apply. A
	// metadatum reached through a block is already bounded by that decode;
	// this covers a direct call on an isolated value.
	if depth > cbor.MaxNestedLevels {
		return nil, 0, fmt.Errorf(
			"metadata nesting exceeds %d levels",
			cbor.MaxNestedLevels,
		)
	}
	major, arg, next, indefinite, err := cborItemHead(b, offset)
	if err != nil {
		return nil, 0, err
	}
	switch major {
	case cborTypeUnsigned, cborTypeNegative:
		if indefinite {
			return nil, 0, errors.New("invalid indefinite-length integer in metadata")
		}
		value := new(big.Int).SetUint64(arg)
		if major == cborTypeNegative {
			// Major type 1 encodes -1 - arg
			value.Neg(value).Sub(value, big.NewInt(1))
		}
		m := MetaInt{Value: value}
		m.SetCborReference(b[offset:next])
		return m, next, nil

	case cborTypeByteString:
		content, end, err := decodeMetadatumStringAt(
			b, major, arg, next, indefinite,
		)
		if err != nil {
			return nil, 0, err
		}
		m := MetaBytes{Value: slices.Clone(content)}
		m.SetCborReference(b[offset:end])
		return m, end, nil

	case cborTypeTextString:
		content, end, err := decodeMetadatumStringAt(
			b, major, arg, next, indefinite,
		)
		if err != nil {
			return nil, 0, err
		}
		if !utf8.Valid(content) {
			return nil, 0, errors.New("invalid UTF-8 in metadata text string")
		}
		m := MetaText{Value: string(content)}
		m.SetCborReference(b[offset:end])
		return m, end, nil

	case cborTypeArray:
		items := []TransactionMetadatum{}
		pos := next
		for i := uint64(0); indefinite || i < arg; i++ {
			if pos >= len(b) {
				return nil, 0, io.ErrUnexpectedEOF
			}
			if indefinite && b[pos] == cborBreak {
				pos++
				break
			}
			item, itemEnd, err := decodeMetadatumAt(b, pos, depth+1)
			if err != nil {
				return nil, 0, err
			}
			items = append(items, item)
			pos = itemEnd
		}
		if !indefinite && pos > len(b) {
			return nil, 0, io.ErrUnexpectedEOF
		}
		m := MetaList{Items: items}
		m.SetCborReference(b[offset:pos])
		return m, pos, nil

	case cborTypeMap:
		pairs := []MetaPair{}
		seen := map[string]struct{}{}
		pos := next
		for i := uint64(0); indefinite || i < arg; i++ {
			if pos >= len(b) {
				return nil, 0, io.ErrUnexpectedEOF
			}
			if indefinite && b[pos] == cborBreak {
				pos++
				break
			}
			key, keyEnd, err := decodeMetadatumAt(b, pos, depth+1)
			if err != nil {
				return nil, 0, err
			}
			identity := metadatumKeyIdentity(key)
			if _, ok := seen[identity]; ok {
				return nil, 0, errors.New("duplicate key in metadata map")
			}
			seen[identity] = struct{}{}
			value, valueEnd, err := decodeMetadatumAt(b, keyEnd, depth+1)
			if err != nil {
				return nil, 0, err
			}
			pairs = append(pairs, MetaPair{Key: key, Value: value})
			pos = valueEnd
		}
		m := MetaMap{Pairs: pairs}
		m.SetCborReference(b[offset:pos])
		return m, pos, nil

	default:
		return nil, 0, fmt.Errorf(
			"unsupported CBOR major type 0x%x in metadata",
			major,
		)
	}
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
	// PlutusV4Scripts returns the Plutus V4 scripts, if present (Dijkstra+ only)
	PlutusV4Scripts() ([]PlutusV4Script, error)
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

func (s *ShelleyAuxiliaryData) PlutusV4Scripts() ([]PlutusV4Script, error) {
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

func (s *ShelleyMaAuxiliaryData) PlutusV4Scripts() ([]PlutusV4Script, error) {
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
	plutusV4Scripts []PlutusV4Script
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

func (a *AlonzoAuxiliaryData) PlutusV4Scripts() ([]PlutusV4Script, error) {
	return a.plutusV4Scripts, nil
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

	// Key 5: Plutus V4 scripts
	if plutusV4Raw := auxMap[5]; len(plutusV4Raw) > 0 {
		if _, err := cbor.Decode(plutusV4Raw, &a.plutusV4Scripts); err != nil {
			return fmt.Errorf("failed to decode Plutus V4 scripts: %w", err)
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
	if len(a.plutusV4Scripts) > 0 {
		scriptsCbor, err := cbor.Encode(a.plutusV4Scripts)
		if err != nil {
			return nil, err
		}
		auxMap[5] = scriptsCbor
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
