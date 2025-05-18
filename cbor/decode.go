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

package cbor

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"

	_cbor "github.com/fxamacker/cbor/v2"
	"github.com/jinzhu/copier"
)

func Decode(dataBytes []byte, dest any) (int, error) {
	data := bytes.NewReader(dataBytes)
	// Create a custom decoder that returns an error on unknown fields
	decOptions := _cbor.DecOptions{
		ExtraReturnErrors: _cbor.ExtraDecErrorUnknownField,
		// This defaults to 32, but there are blocks in the wild using >64 nested levels
		MaxNestedLevels: 256,
	}
	decMode, err := decOptions.DecModeWithTags(customTagSet)
	if err != nil {
		return 0, err
	}
	dec := decMode.NewDecoder(data)
	err = dec.Decode(dest)
	return dec.NumBytesRead(), err
}

// Extract the first item from a CBOR list. This will return the first item from the
// provided list if it's numeric and an error otherwise
func DecodeIdFromList(cborData []byte) (int, error) {
	// If the list length is <= the max simple uint and the first list value
	// is <= the max simple uint, then we can extract the value straight from
	// the byte slice
	listLen, err := ListLength(cborData)
	if err != nil {
		return 0, err
	}
	if listLen == 0 {
		return 0, errors.New("cannot return first item from empty list")
	}
	if listLen < int(CborMaxUintSimple) {
		if cborData[1] <= CborMaxUintSimple {
			return int(cborData[1]), nil
		}
	}
	// If we couldn't use the shortcut above, actually decode the list
	var tmp Value
	if _, err := Decode(cborData, &tmp); err != nil {
		return 0, err
	}
	// Make sure that the value is actually numeric
	switch v := tmp.Value().([]any)[0].(type) {
	// The upstream CBOR library uses uint64 by default for numeric values
	case uint64:
		if v > uint64(math.MaxInt) {
			return 0, errors.New("decoded numeric value too large: uint64 > int")
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("first list item was not numeric, found: %v", v)
	}
}

// Determine the length of a CBOR list
func ListLength(cborData []byte) (int, error) {
	// If the list length is <= the max simple uint, then we can extract the length
	// value straight from the byte slice (with a little math)
	if cborData[0] >= CborTypeArray &&
		cborData[0] <= (CborTypeArray+CborMaxUintSimple) {
		return int(cborData[0]) - int(CborTypeArray), nil
	}
	// If we couldn't use the shortcut above, actually decode the list
	var tmp []RawMessage
	if _, err := Decode(cborData, &tmp); err != nil {
		return 0, err
	}
	return len(tmp), nil
}

// Decode CBOR list data by the leading value of each list item. It expects CBOR data and
// a map of numbers to object pointers to decode into
func DecodeById(
	cborData []byte,
	idMap map[int]any,
) (any, error) {
	id, err := DecodeIdFromList(cborData)
	if err != nil {
		return nil, err
	}
	ret, ok := idMap[id]
	if !ok || ret == nil {
		return nil, fmt.Errorf("found unknown ID: %x", id)
	}
	if _, err := Decode(cborData, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

var (
	decodeGenericTypeCache      = map[reflect.Type]reflect.Type{}
	decodeGenericTypeCacheMutex sync.RWMutex
)

// DecodeGeneric decodes the specified CBOR into the destination object without using the
// destination object's UnmarshalCBOR() function
func DecodeGeneric(cborData []byte, dest any) error {
	// Get destination type
	valueDest := reflect.ValueOf(dest)
	typeDest := valueDest.Elem().Type()
	// Check type cache
	decodeGenericTypeCacheMutex.RLock()
	tmpTypeDest, ok := decodeGenericTypeCache[typeDest]
	decodeGenericTypeCacheMutex.RUnlock()
	if !ok {
		// Create a duplicate(-ish) struct from the destination
		// We do this so that we can bypass any custom UnmarshalCBOR() function on the
		// destination object
		if valueDest.Kind() != reflect.Pointer ||
			valueDest.Elem().Kind() != reflect.Struct {
			return errors.New("destination must be a pointer to a struct")
		}
		destTypeFields := []reflect.StructField{}
		for i := range typeDest.NumField() {
			tmpField := typeDest.Field(i)
			if tmpField.IsExported() && tmpField.Name != "DecodeStoreCbor" {
				destTypeFields = append(destTypeFields, tmpField)
			}
		}
		tmpTypeDest = reflect.StructOf(destTypeFields)
		// Populate cache
		decodeGenericTypeCacheMutex.Lock()
		decodeGenericTypeCache[typeDest] = tmpTypeDest
		decodeGenericTypeCacheMutex.Unlock()
	}
	// Create temporary object with the type created above
	tmpDest := reflect.New(tmpTypeDest)
	// Decode CBOR into temporary object
	if _, err := Decode(cborData, tmpDest.Interface()); err != nil {
		return err
	}
	// Copy values from temporary object into destination object
	if err := copier.Copy(dest, tmpDest.Interface()); err != nil {
		return err
	}
	return nil
}
