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
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"runtime"
	"sort"
	"strings"
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
	_, err := v.unmarshalCBOR(data, true, 0)
	return err
}

func (v *Value) unmarshalCBOR(
	data []byte,
	retainCbor bool,
	depth int,
) (int, error) {
	if len(data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if depth > cborMaxNestedLevels {
		return 0, fmt.Errorf(
			"exceeded maximum CBOR nesting depth: %d",
			cborMaxNestedLevels,
		)
	}
	if retainCbor {
		// Save the original CBOR
		v.cborData = string(data)
	} else {
		v.cborData = ""
	}
	cborType := data[0] & CborTypeMask
	switch cborType {
	case CborTypeMap:
		return v.processMap(data, depth)
	case CborTypeArray:
		return v.processArray(data, depth)
	case CborTypeTextString:
		var tmpValue string
		decodedLength, err := Decode(data, &tmpValue)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		return decodedLength, nil
	case CborTypeByteString:
		// Use our custom type which stores the bytestring in a way that allows it to be used as a map key
		var tmpValue ByteString
		decodedLength, err := Decode(data, &tmpValue)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		return decodedLength, nil
	case CborTypeTag:
		// Parse as a raw tag to get number and nested CBOR data
		tmpTag := RawTag{}
		decodedLength, err := Decode(data, &tmpTag)
		if err != nil {
			return 0, err
		}
		if IsAlternativeTag(tmpTag.Number) {
			// Constructors/alternatives
			var tmpConstr ConstructorDecoder
			if _, err := Decode(data, &tmpConstr); err != nil {
				return 0, err
			}
			v.value = tmpConstr
		} else {
			// Fall back to standard CBOR tag parsing for our supported types
			var tmpTagDecode any
			if _, err := Decode(data, &tmpTagDecode); err != nil {
				return 0, err
			}
			v.value = tmpTagDecode
		}
		return decodedLength, nil
	default:
		var tmpValue any
		decodedLength, err := Decode(data, &tmpValue)
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

func (v *Value) processMap(data []byte, depth int) (decodedLength int, err error) {
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
		keyLength, keyErr := key.unmarshalCBOR(data[position:], false, depth+1)
		if keyErr != nil {
			return 0, keyErr
		}
		position += keyLength
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}

		var value Value
		valueLength, valueErr := value.unmarshalCBOR(data[position:], false, depth+1)
		if valueErr != nil {
			return 0, valueErr
		}
		position += valueLength

		// CBOR null/undefined map keys decode to a nil *Value, which is
		// represented as a nil map key
		var keyValue any
		keyValue = key.Value()
		// Use a pointer for unhashable key types
		if keyValue != nil && !reflect.TypeOf(keyValue).Comparable() {
			keyValue = &keyValue
		}
		newValue[keyValue] = value.Value()
	}
	v.value = newValue
	return position, nil
}

func (v *Value) processArray(data []byte, depth int) (int, error) {
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
		valueLength, err := value.unmarshalCBOR(data[position:], false, depth+1)
		if err != nil {
			return 0, err
		}
		position += valueLength
		newValue = append(newValue, value.Value())
	}
	v.value = newValue
	return position, nil
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
	err := l.value.UnmarshalCBOR([]byte(l.value.cborData))
	return l.Value(), err
}

func (l *LazyValue) Value() any {
	return l.value.Value()
}

func (l *LazyValue) Cbor() []byte {
	return l.value.Cbor()
}
