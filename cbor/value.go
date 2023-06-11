// Copyright 2023 Blink Labs, LLC.
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

// Helpful wrapper for parsing arbitrary CBOR data which may contain types that
// cannot be easily represented in Go (such as maps with bytestring keys)
type Value struct {
	Value interface{}
	// We store this as a string so that the type is still hashable for use as map keys
	cborData string
}

func (v *Value) UnmarshalCBOR(data []byte) error {
	// Save the original CBOR
	v.cborData = string(data[:])
	cborType := data[0] & CBOR_TYPE_MASK
	switch cborType {
	case CBOR_TYPE_MAP:
		tmpValue := map[Value]Value{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		// Extract actual value from each child value
		newValue := map[interface{}]interface{}{}
		for key, value := range tmpValue {
			newValue[key.Value] = value.Value
		}
		v.Value = newValue
	case CBOR_TYPE_ARRAY:
		tmpValue := []Value{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		// Extract actual value from each child value
		newValue := []interface{}{}
		for _, value := range tmpValue {
			newValue = append(newValue, value.Value)
		}
		v.Value = newValue
	case CBOR_TYPE_TEXT_STRING:
		var tmpValue string
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.Value = tmpValue
	case CBOR_TYPE_BYTE_STRING:
		// Use our custom type which stores the bytestring in a way that allows it to be used as a map key
		var tmpValue ByteString
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.Value = tmpValue
	case CBOR_TYPE_TAG:
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
		// Create new tag object with decoded value
		newValue := Tag{
			Number:  tmpTag.Number,
			Content: tmpValue.Value,
		}
		v.Value = newValue
	default:
		var tmpValue interface{}
		if _, err := Decode(data, &tmpValue); err != nil {
			return err
		}
		v.Value = tmpValue
	}
	return nil
}

func (v Value) Cbor() []byte {
	return []byte(v.cborData)
}
