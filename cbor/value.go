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
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"math/big"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"

	_cbor "github.com/fxamacker/cbor/v2"
)

// Helpful wrapper for parsing arbitrary CBOR data which may contain types that
// cannot be easily represented in Go (such as maps with bytestring keys)
type Value struct {
	value any
	// We store this as a string so that the type is still hashable for use as map keys
	cborData string
}

func (v *Value) MarshalCBOR() ([]byte, error) {
	// Return stored CBOR
	// This is only a stopgap, since it doesn't allow us to build values from scratch
	return []byte(v.cborData), nil
}

func (v *Value) UnmarshalCBOR(data []byte) error {
	if len(data) == 0 {
		return errors.New("empty CBOR data")
	}
	decMode, err := getDecMode()
	if err != nil {
		return err
	}
	if decMode == nil {
		return errors.New("CBOR decoder mode not initialized")
	}
	_, err = v.unmarshalCBOR(
		data,
		true,
		0,
		rejectDuplicateMapKeys,
		decMode,
	)
	return err
}

func (v *Value) unmarshalCBOR(
	data []byte,
	retainCbor bool,
	depth int,
	duplicateKeyPolicy duplicateMapKeyPolicy,
	decMode _cbor.DecMode,
) (int, error) {
	if len(data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if depth > MaxNestedLevels {
		return 0, fmt.Errorf(
			"exceeded maximum CBOR nesting depth: %d",
			MaxNestedLevels,
		)
	}
	if retainCbor {
		// The custom walker must inherit the caller's limits before allocating.
		// Decode consumes one item, so bytes after that item remain allowed.
		if err := decMode.Wellformed(data); err != nil {
			var extra *_cbor.ExtraneousDataError
			if !errors.As(err, &extra) {
				return 0, err
			}
			dec := decMode.NewDecoder(bytes.NewReader(data))
			if err := dec.Skip(); err != nil {
				return 0, err
			}
			data = data[:dec.NumBytesRead()]
		}
		// Save the original CBOR
		v.cborData = string(data)
	} else {
		v.cborData = ""
	}
	cborType := data[0] & CborTypeMask
	switch cborType {
	case CborTypeMap:
		return v.processMap(data, depth, duplicateKeyPolicy, decMode)
	case CborTypeArray:
		return v.processArray(data, depth, duplicateKeyPolicy, decMode)
	case CborTypeTextString:
		var tmpValue string
		decodedLength, err := decodeWithMode(
			data,
			&tmpValue,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		return decodedLength, nil
	case CborTypeByteString:
		// Use our custom type which stores the bytestring in a way that allows it to be used as a map key
		var tmpValue ByteString
		decodedLength, err := decodeWithMode(
			data,
			&tmpValue,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		return decodedLength, nil
	case CborTypeTag:
		// Parse as a raw tag to get number and nested CBOR data
		tmpTag := RawTag{}
		decodedLength, err := decodeWithMode(
			data,
			&tmpTag,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		if IsAlternativeTag(tmpTag.Number) {
			// Constructors/alternatives
			var tmpConstr ConstructorDecoder
			if _, err := decodeWithMode(
				data,
				&tmpConstr,
				decMode,
				duplicateKeyPolicy,
			); err != nil {
				return 0, err
			}
			v.value = tmpConstr
		} else {
			// Fall back to standard CBOR tag parsing for our supported types
			var tmpTagDecode any
			if _, err := decodeWithMode(
				data,
				&tmpTagDecode,
				decMode,
				duplicateKeyPolicy,
			); err != nil {
				return 0, err
			}
			v.value = tmpTagDecode
		}
		return decodedLength, nil
	default:
		var tmpValue any
		decodedLength, err := decodeWithMode(
			data,
			&tmpValue,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		return decodedLength, nil
	}
}

func (v Value) Cbor() []byte {
	return []byte(v.cborData)
}

func (v Value) Value() any {
	return v.value
}

func (v Value) MarshalJSON() ([]byte, error) {
	var tmpJson string
	if v.value != nil {
		astJson, err := generateAstJson(v.value)
		if err != nil {
			return nil, err
		}
		tmpJson = fmt.Sprintf(
			`{"cbor":"%s","json":%s}`,
			hex.EncodeToString([]byte(v.cborData)),
			astJson,
		)
	} else {
		tmpJson = fmt.Sprintf(
			`{"cbor":"%s"}`,
			hex.EncodeToString([]byte(v.cborData)),
		)
	}
	return []byte(tmpJson), nil
}

func (v *Value) processMap(
	data []byte,
	depth int,
	duplicateKeyPolicy duplicateMapKeyPolicy,
	decMode _cbor.DecMode,
) (decodedLength int, err error) {
	// There are certain types that cannot be used as map keys in Go but are valid in CBOR. Trying to
	// parse CBOR containing a map with keys of one of those types will cause a panic. We setup this
	// deferred function to recover from a possible panic and return an error
	defer func() {
		if r := recover(); r != nil {
			if !isUnhashableMapKeyPanic(r) {
				panic(r)
			}
			err = fmt.Errorf(
				"decode failure, probably due to type unsupported by Go: %v",
				r,
			)
		}
	}()
	itemCount, headerLength, indefinite := MapInfo(data)
	if itemCount < 0 {
		return 0, errors.New("invalid CBOR map header")
	}
	newValue := map[any]any{}
	seenKeys := make(map[string]any)
	position := int(headerLength)
	for itemIndex := 0; indefinite || itemIndex < itemCount; itemIndex++ {
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		if indefinite && data[position] == 0xff {
			position++
			v.value = newValue
			return position, nil
		}

		var key Value
		keyLength, keyErr := key.unmarshalCBOR(
			data[position:],
			false,
			depth+1,
			duplicateKeyPolicy,
			decMode,
		)
		if keyErr != nil {
			return 0, keyErr
		}
		position += keyLength
		keyValue := key.Value()
		keyComparable := isReflexiveMapKey(keyValue)
		keyIdentity := mapKeyIdentity(keyValue)
		storageKey, duplicate := seenKeys[keyIdentity]
		if duplicate && duplicateKeyPolicy == rejectDuplicateMapKeys {
			return 0, &_cbor.DupMapKeyError{
				Key:   keyValue,
				Index: itemIndex,
			}
		}
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}

		var value Value
		valueLength, valueErr := value.unmarshalCBOR(
			data[position:],
			false,
			depth+1,
			duplicateKeyPolicy,
			decMode,
		)
		if valueErr != nil {
			return 0, valueErr
		}
		position += valueLength

		if duplicate {
			newValue[storageKey] = value.Value()
			continue
		}

		newStorageKey := keyValue
		if !keyComparable {
			// Use a pointer for unhashable keys and values, such as NaN, that do
			// not compare equal to themselves.
			newStorageKey = &keyValue
		}
		seenKeys[keyIdentity] = newStorageKey
		newValue[newStorageKey] = value.Value()
	}
	v.value = newValue
	return position, nil
}

func (v *Value) processArray(
	data []byte,
	depth int,
	duplicateKeyPolicy duplicateMapKeyPolicy,
	decMode _cbor.DecMode,
) (int, error) {
	itemCount, headerLength, indefinite := ArrayInfo(data)
	if itemCount < 0 {
		return 0, errors.New("invalid CBOR array header")
	}
	newValue := []any{}
	position := int(headerLength)
	for itemIndex := 0; indefinite || itemIndex < itemCount; itemIndex++ {
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		if indefinite && data[position] == 0xff {
			position++
			v.value = newValue
			return position, nil
		}

		var value Value
		valueLength, err := value.unmarshalCBOR(
			data[position:],
			false,
			depth+1,
			duplicateKeyPolicy,
			decMode,
		)
		if err != nil {
			return 0, err
		}
		position += valueLength
		newValue = append(newValue, value.Value())
	}
	v.value = newValue
	return position, nil
}

func isReflexiveMapKey(key any) bool {
	if key == nil {
		return true
	}
	return reflect.ValueOf(key).Comparable() && key == key
}

func mapKeyIdentity(key any) string {
	digest := mapKeyDigest(key)
	return string(digest[:])
}

func mapKeyDigest(key any) [sha256.Size]byte {
	digest := sha256.New()
	appendMapKeyDigest(digest, key)
	var ret [sha256.Size]byte
	copy(ret[:], digest.Sum(nil))
	return ret
}

func appendMapKeyDigest(digest hash.Hash, key any) {
	writeBytes := func(marker byte, value []byte) {
		_, _ = digest.Write([]byte{marker})
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(value)
	}
	writeUint := func(marker byte, value uint64) {
		_, _ = digest.Write([]byte{marker})
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], value)
		_, _ = digest.Write(encoded[:])
	}
	writeSequence := func(marker byte, values []any) {
		writeUint(marker, uint64(len(values)))
		for _, value := range values {
			childDigest := mapKeyDigest(value)
			_, _ = digest.Write(childDigest[:])
		}
	}
	writeMap := func(marker byte, values map[any]any) {
		entries := make([][sha256.Size * 2]byte, 0, len(values))
		for key, value := range values {
			keyDigest := mapKeyDigest(key)
			valueDigest := mapKeyDigest(value)
			var entry [sha256.Size * 2]byte
			copy(entry[:sha256.Size], keyDigest[:])
			copy(entry[sha256.Size:], valueDigest[:])
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool {
			return bytes.Compare(entries[i][:], entries[j][:]) < 0
		})
		writeUint(marker, uint64(len(entries)))
		for _, entry := range entries {
			_, _ = digest.Write(entry[:])
		}
	}

	switch key := key.(type) {
	case nil:
		_, _ = digest.Write([]byte{'n'})
	case bool:
		if key {
			_, _ = digest.Write([]byte{'b', 1})
		} else {
			_, _ = digest.Write([]byte{'b', 0})
		}
	case uint64:
		writeBytes('i', []byte(strconv.FormatUint(key, 10)))
	case int64:
		writeBytes('i', []byte(strconv.FormatInt(key, 10)))
	case float64:
		bits := math.Float64bits(key)
		if key == 0 {
			bits = 0
		}
		writeUint('f', bits)
	case string:
		writeBytes('s', []byte(key))
	case ByteString:
		writeBytes('x', key.Bytes())
	case []byte:
		writeBytes('x', key)
	case WrappedCbor:
		writeBytes('w', key)
	case big.Int:
		writeBytes('i', []byte(key.String()))
	case *big.Int:
		if key == nil {
			_, _ = digest.Write([]byte{'G', 0})
		} else {
			writeBytes('i', []byte(key.String()))
		}
	case []any:
		writeSequence('a', key)
	case Set:
		writeSequence('e', []any(key))
	case map[any]any:
		writeMap('m', key)
	case Map:
		writeMap('M', map[any]any(key))
	case *any:
		if key == nil {
			_, _ = digest.Write([]byte{'p', 0})
		} else {
			appendMapKeyDigest(digest, *key)
		}
	case ConstructorDecoder:
		writeUint('c', uint64(key.Tag()))
		writeBytes('C', key.Fields())
	case _cbor.Tag:
		writeUint('g', key.Number)
		appendMapKeyDigest(digest, key.Content)
	case RawTag:
		writeUint('t', key.Number)
		writeBytes('T', key.Content)
	case Rat:
		if key.Rat == nil {
			_, _ = digest.Write([]byte{'r', 0})
		} else {
			writeBytes('r', []byte(key.RatString()))
		}
	default:
		writeBytes('?', []byte(fmt.Sprintf("%T:%#v", key, key)))
	}
}

func isUnhashableMapKeyPanic(r any) bool {
	runtimeErr, ok := r.(runtime.Error)
	if !ok {
		return false
	}
	return strings.Contains(runtimeErr.Error(), "hash of unhashable type")
}

func generateAstJson(obj any) ([]byte, error) {
	tmpJsonObj := map[string]any{}
	switch v := obj.(type) {
	case []byte:
		tmpJsonObj["bytes"] = hex.EncodeToString(v)
	case ByteString:
		tmpJsonObj["bytes"] = hex.EncodeToString(v.Bytes())
	case WrappedCbor:
		tmpJsonObj["bytes"] = hex.EncodeToString(v.Bytes())
	case []any:
		return generateAstJsonList(v)
	case Set:
		return generateAstJsonList(v)
	case map[any]any:
		return generateAstJsonMap(v)
	case Map:
		return generateAstJsonMap(v)
	case ConstructorDecoder:
		return json.Marshal(obj)
	case big.Int:
		tmpJson := fmt.Sprintf(
			`{"int":%s}`,
			v.String(),
		)
		return []byte(tmpJson), nil
	case *big.Int:
		if v == nil {
			tmpJson := `{"int":0}`
			return []byte(tmpJson), nil
		}
		tmpJson := fmt.Sprintf(`{"int":%s}`, v.String())
		return []byte(tmpJson), nil
	case Rat:
		return generateAstJson(
			[]any{
				v.Num(),
				v.Denom(),
			},
		)
	case int, uint, uint64, int64:
		tmpJsonObj["int"] = v
	case bool:
		tmpJsonObj["bool"] = v
	case string:
		tmpJsonObj["string"] = v
	default:
		return nil, fmt.Errorf("unknown data type (%T) for value: %#v", obj, obj)
	}
	return json.Marshal(&tmpJsonObj)
}

func generateAstJsonList[T []any | Set](v T) ([]byte, error) {
	var sb strings.Builder
	sb.WriteString(`{"list":[`)
	for idx, val := range v {
		tmpVal, err := generateAstJson(val)
		if err != nil {
			return nil, err
		}
		sb.WriteString(string(tmpVal))
		if idx != (len(v) - 1) {
			sb.WriteString(`,`)
		}
	}
	sb.WriteString(`]}`)
	return []byte(sb.String()), nil
}

func generateAstJsonMap[T map[any]any | Map](v T) ([]byte, error) {
	tmpItems := []string{}
	for key, val := range v {
		keyAstJson, err := generateAstJson(key)
		if err != nil {
			return nil, err
		}
		valAstJson, err := generateAstJson(val)
		if err != nil {
			return nil, err
		}
		tmpJsonMap := map[string]json.RawMessage{
			"k": keyAstJson,
			"v": valAstJson,
		}
		tmpJson, err := json.Marshal(tmpJsonMap)
		if err != nil {
			return nil, err
		}
		tmpItems = append(tmpItems, string(tmpJson))
	}
	// We naively sort the rendered map items to give consistent ordering
	sort.Strings(tmpItems)
	tmpJson := fmt.Sprintf(
		`{"map":[%s]}`,
		strings.Join(tmpItems, ","),
	)
	return []byte(tmpJson), nil
}

type LazyValue struct {
	value *Value
}

func (l *LazyValue) MarshalCBOR() ([]byte, error) {
	if l.value == nil {
		l.value = &Value{}
	}
	// Return stored CBOR
	// This is only a stopgap, since it doesn't allow us to build values from scratch
	return []byte(l.value.cborData), nil
}

func (l *LazyValue) UnmarshalCBOR(data []byte) error {
	if l.value == nil {
		l.value = &Value{}
	}
	l.value.cborData = string(data[:])
	return nil
}

func (l *LazyValue) MarshalJSON() ([]byte, error) {
	if l.value == nil {
		l.value = &Value{}
	}
	if l.Value() == nil && len(l.value.cborData) > 0 {
		// Try to decode if we can, but don't blow up if we can't
		if _, err := l.Decode(); err != nil {
			tmpJsonObj := map[string]any{
				"cbor":  hex.EncodeToString([]byte(l.value.cborData)),
				"json":  nil,
				"error": err.Error(),
			}
			return json.Marshal(tmpJsonObj)
		}
	}
	return l.value.MarshalJSON()
}

func (l *LazyValue) Decode() (any, error) {
	if l.value == nil {
		l.value = &Value{}
	}
	err := l.value.UnmarshalCBOR([]byte(l.value.cborData))
	return l.Value(), err
}

func (l *LazyValue) Value() any {
	if l.value == nil {
		return nil
	}
	return l.value.Value()
}

func (l *LazyValue) Cbor() []byte {
	if l.value == nil {
		return nil
	}
	return l.value.Cbor()
}
