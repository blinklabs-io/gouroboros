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
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"reflect"
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
	// Save the original CBOR
	v.cborData = string(data[:])
	cborType := data[0] & CborTypeMask
	switch cborType {
	case CborTypeMap:
		return v.processMap(data)
	case CborTypeArray:
		return v.processArray(data)
	case CborTypeTextString:
		var tmpValue string
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.value = tmpValue
	case CborTypeByteString:
		// Use our custom type which stores the bytestring in a way that allows it to be used as a map key
		var tmpValue ByteString
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.value = tmpValue
	case CborTypeTag:
		// Parse as a raw tag to get number and nested CBOR data
		tmpTag := RawTag{}
		if _, err := Decode(data, &tmpTag); err != nil {
			return err
		}
		if IsAlternativeTag(tmpTag.Number) {
			// Constructors/alternatives
			var tmpConstr ConstructorDecoder
			if _, err := Decode(data, &tmpConstr); err != nil {
				return err
			}
			v.value = tmpConstr
		} else {
			// Fall back to standard CBOR tag parsing for our supported types
			var tmpTagDecode any
			if _, err := Decode(data, &tmpTagDecode); err != nil {
				return err
			}
			v.value = tmpTagDecode
		}
	default:
		var tmpValue any
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.value = tmpValue
	}
	return nil
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

func (v *Value) processMap(data []byte) (err error) {
	// There are certain types that cannot be used as map keys in Go but are valid in CBOR. Trying to
	// parse CBOR containing a map with keys of one of those types will cause a panic. We setup this
	// deferred function to recover from a possible panic and return an error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf(
				"decode failure, probably due to type unsupported by Go: %v",
				r,
			)
		}
	}()
	tmpValue := map[*Value]Value{}
	if _, err = Decode(data, &tmpValue); err != nil {
		return err
	}
	// Extract actual value from each child value
	newValue := map[any]any{}
	for key, value := range tmpValue {
		keyValue := key.Value()
		// Use a pointer for unhashable key types
		if !reflect.TypeOf(keyValue).Comparable() {
			keyValue = &keyValue
		}
		newValue[keyValue] = value.Value()
	}
	v.value = newValue
	return nil
}

func (v *Value) processArray(data []byte) error {
	tmpValue := []Value{}
	if _, err := Decode(data, &tmpValue); err != nil {
		return err
	}
	// Extract actual value from each child value
	newValue := []any{}
	for _, value := range tmpValue {
		newValue = append(newValue, value.Value())
	}
	v.value = newValue
	return nil
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
				v.Num().Uint64(),
				v.Denom().Uint64(),
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
	if l.Value() == nil {
		// Try to decode if we can, but don't blow up if we can't
		_, _ = l.Decode()
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
