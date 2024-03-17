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
	"sort"
	"strings"
)

// Helpful wrapper for parsing arbitrary CBOR data which may contain types that
// cannot be easily represented in Go (such as maps with bytestring keys)
type Value struct {
	value interface{}
	// We store this as a string so that the type is still hashable for use as map keys
	cborData string
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
		if (tmpTag.Number >= CborTagAlternative1Min && tmpTag.Number <= CborTagAlternative1Max) ||
			(tmpTag.Number >= CborTagAlternative2Min && tmpTag.Number <= CborTagAlternative2Max) ||
			tmpTag.Number == CborTagAlternative3 {
			// Constructors/alternatives
			var tmpConstr Constructor
			if _, err := Decode(data, &tmpConstr); err != nil {
				return err
			}
			v.value = tmpConstr
		} else {
			// Fall back to standard CBOR tag parsing for our supported types
			var tmpTagDecode interface{}
			if _, err := Decode(data, &tmpTagDecode); err != nil {
				return err
			}
			switch tmpTagDecode.(type) {
			case int, uint, int64, uint64, bool, big.Int, WrappedCbor, Rat, Set, Map:
				v.value = tmpTagDecode
			default:
				return fmt.Errorf("unsupported CBOR tag number: %d", tmpTag.Number)
			}
		}
	default:
		var tmpValue interface{}
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

func (v Value) Value() interface{} {
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
	tmpValue := map[Value]Value{}
	if _, err = Decode(data, &tmpValue); err != nil {
		return err
	}
	// Extract actual value from each child value
	newValue := map[interface{}]interface{}{}
	for key, value := range tmpValue {
		newValue[key.Value()] = value.Value()
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
	newValue := []interface{}{}
	for _, value := range tmpValue {
		newValue = append(newValue, value.Value())
	}
	v.value = newValue
	return nil
}

func generateAstJson(obj interface{}) ([]byte, error) {
	tmpJsonObj := map[string]interface{}{}
	switch v := obj.(type) {
	case ByteString:
		tmpJsonObj["bytes"] = hex.EncodeToString(v.Bytes())
	case WrappedCbor:
		tmpJsonObj["bytes"] = hex.EncodeToString(v.Bytes())
	case []interface{}:
		return generateAstJsonList[[]any](v)
	case Set:
		return generateAstJsonList[Set](v)
	case map[interface{}]interface{}:
		return generateAstJsonMap[map[any]any](v)
	case Map:
		return generateAstJsonMap[Map](v)
	case Constructor:
		return json.Marshal(obj)
	case big.Int:
		tmpJson := fmt.Sprintf(
			`{"int":%s}`,
			v.String(),
		)
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
	tmpJson := `{"list":[`
	for idx, val := range v {
		tmpVal, err := generateAstJson(val)
		if err != nil {
			return nil, err
		}
		tmpJson += string(tmpVal)
		if idx != (len(v) - 1) {
			tmpJson += `,`
		}
	}
	tmpJson += `]}`
	return []byte(tmpJson), nil
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
		// NOTE: Github CodeQL hates this due to "potentially unsafe quoting", but it
		// won't happen in practice since both values injected are auto-generated
		tmpJson := fmt.Sprintf(
			`{"k":%s,"v":%s}`,
			keyAstJson,
			valAstJson,
		)
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

type Constructor struct {
	DecodeStoreCbor
	constructor uint
	value       *Value
}

func NewConstructor(constructor uint, value any) Constructor {
	c := Constructor{
		constructor: constructor,
	}
	if value != nil {
		c.value = &Value{
			value: value,
		}
	}
	return c
}

func (v Constructor) Constructor() uint {
	return v.constructor
}

func (v Constructor) Fields() []any {
	return v.value.Value().([]any)
}

func (c Constructor) FieldsCbor() []byte {
	return c.value.Cbor()
}

func (c *Constructor) UnmarshalCBOR(data []byte) error {
	// Save original CBOR
	c.SetCbor(data)
	// Parse as a raw tag to get number and nested CBOR data
	tmpTag := RawTag{}
	if _, err := Decode(data, &tmpTag); err != nil {
		return err
	}
	// Parse the tag value via our custom Value object to handle problem types
	tmpValue := Value{}
	if _, err := Decode(tmpTag.Content, &tmpValue); err != nil {
		return err
	}
	if tmpTag.Number >= CborTagAlternative1Min && tmpTag.Number <= CborTagAlternative1Max {
		// Alternatives 0-6
		c.constructor = uint(tmpTag.Number - CborTagAlternative1Min)
		c.value = &tmpValue
	} else if tmpTag.Number >= CborTagAlternative2Min && tmpTag.Number <= CborTagAlternative2Max {
		// Alternatives 7-127
		c.constructor = uint(tmpTag.Number - CborTagAlternative2Min + 7)
		c.value = &tmpValue
	} else if tmpTag.Number == CborTagAlternative3 {
		// Alternatives 128+
		tmpValues := tmpValue.Value().([]any)
		c.constructor = uint(tmpValues[0].(uint64))
		newValue := Value{
			value: tmpValues[1],
		}
		c.value = &newValue
	} else {
		return fmt.Errorf("unsupported tag: %d", tmpTag.Number)
	}
	return nil
}

func (c Constructor) MarshalCBOR() ([]byte, error) {
	var tmpTag Tag
	if c.constructor <= 6 {
		// Alternatives 0-6
		tmpTag.Number = uint64(c.constructor + CborTagAlternative1Min)
		tmpTag.Content = c.value.Value()
	} else if c.constructor >= 7 && c.constructor <= 127 {
		// Alternatives 7-127
		tmpTag.Number = uint64(c.constructor + CborTagAlternative2Min - 7)
		tmpTag.Content = c.value.Value()
	} else if c.constructor >= 128 {
		tmpTag.Number = CborTagAlternative3
		tmpTag.Content = []any{
			c.constructor,
			c.value.Value(),
		}
	}
	return Encode(&tmpTag)
}

func (v Constructor) MarshalJSON() ([]byte, error) {
	tmpJson := fmt.Sprintf(`{"constructor":%d,"fields":[`, v.constructor)
	tmpList := [][]byte{}
	for _, val := range v.value.Value().([]any) {
		tmpVal, err := generateAstJson(val)
		if err != nil {
			return nil, err
		}
		tmpList = append(tmpList, tmpVal)
	}
	for idx, val := range tmpList {
		tmpJson += string(val)
		if idx != (len(tmpList) - 1) {
			tmpJson += `,`
		}
	}
	tmpJson += `]}`
	return []byte(tmpJson), nil
}

type LazyValue struct {
	value *Value
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

func (l *LazyValue) Decode() (interface{}, error) {
	err := l.value.UnmarshalCBOR([]byte(l.value.cborData))
	return l.Value(), err
}

func (l *LazyValue) Value() interface{} {
	return l.value.Value()
}

func (l *LazyValue) Cbor() []byte {
	return l.value.Cbor()
}
