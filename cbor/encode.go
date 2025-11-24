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
	"reflect"
	"sort"
	"sync"

	_cbor "github.com/fxamacker/cbor/v2"
	"github.com/jinzhu/copier"
)

func Encode(data any) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	opts := _cbor.EncOptions{
		// Make sure that maps have ordered keys
		Sort: _cbor.SortCoreDeterministic,
	}
	em, err := opts.EncModeWithTags(customTagSet)
	if err != nil {
		return nil, err
	}
	enc := em.NewEncoder(buf)
	err = enc.Encode(data)
	return buf.Bytes(), err
}

var (
	encodeGenericTypeCache      = map[reflect.Type]reflect.Type{}
	encodeGenericTypeCacheMutex sync.RWMutex
)

// EncodeGeneric encodes the specified object to CBOR without using the source object's
// MarshalCBOR() function
func EncodeGeneric(src any) ([]byte, error) {
	// Get source type
	valueSrc := reflect.ValueOf(src)
	typeSrc := valueSrc.Elem().Type()
	// Check type cache
	encodeGenericTypeCacheMutex.RLock()
	tmpTypeSrc, ok := encodeGenericTypeCache[typeSrc]
	encodeGenericTypeCacheMutex.RUnlock()
	if !ok {
		// Create a duplicate(-ish) struct from the destination
		// We do this so that we can bypass any custom MarshalCBOR() function on the
		// source object
		if valueSrc.Kind() != reflect.Pointer ||
			valueSrc.Elem().Kind() != reflect.Struct {
			return nil, errors.New("source must be a pointer to a struct")
		}
		srcTypeFields := []reflect.StructField{}
		for i := range typeSrc.NumField() {
			tmpField := typeSrc.Field(i)
			if tmpField.IsExported() && tmpField.Name != "DecodeStoreCbor" {
				srcTypeFields = append(srcTypeFields, tmpField)
			}
		}
		tmpTypeSrc = reflect.StructOf(srcTypeFields)
		// Populate cache
		encodeGenericTypeCacheMutex.Lock()
		encodeGenericTypeCache[typeSrc] = tmpTypeSrc
		encodeGenericTypeCacheMutex.Unlock()
	}
	// Create temporary object with the type created above
	tmpSrc := reflect.New(tmpTypeSrc)
	// Copy values from source object into temporary object
	if err := copier.Copy(tmpSrc.Interface(), src); err != nil {
		return nil, err
	}
	// Encode temporary object into CBOR
	cborData, err := Encode(tmpSrc.Interface())
	if err != nil {
		return nil, err
	}
	return cborData, nil
}

type IndefLengthList []any

func (i IndefLengthList) MarshalCBOR() ([]byte, error) {
	ret := []byte{
		// Start indefinite-length list
		0x9f,
	}
	for _, item := range []any(i) {
		data, err := Encode(&item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, data...)
	}
	ret = append(
		ret,
		// End indefinite length array
		byte(0xff),
	)
	return ret, nil
}

type IndefLengthByteString []any

func (i IndefLengthByteString) MarshalCBOR() ([]byte, error) {
	ret := []byte{
		// Start indefinite-length bytestring
		0x5f,
	}
	for _, item := range []any(i) {
		data, err := Encode(&item)
		if err != nil {
			return nil, err
		}
		ret = append(ret, data...)
	}
	ret = append(
		ret,
		// End indefinite length bytestring
		byte(0xff),
	)
	return ret, nil
}

type IndefLengthMap map[any]any

func (i IndefLengthMap) MarshalCBOR() ([]byte, error) {
	ret := []byte{
		// Start indefinite-length map
		0xbf,
	}

	// Collect keys and sort them by their CBOR encoding for deterministic output
	type keyValue struct {
		key     any
		keyCbor []byte
		value   any
	}

	kvPairs := make([]keyValue, 0, len(map[any]any(i)))
	for key, value := range map[any]any(i) {
		keyData, err := Encode(key)
		if err != nil {
			return nil, err
		}
		kvPairs = append(kvPairs, keyValue{
			key:     key,
			keyCbor: keyData,
			value:   value,
		})
	}

	// Sort by CBOR-encoded key for deterministic ordering
	sort.Slice(kvPairs, func(a, b int) bool {
		return bytes.Compare(kvPairs[a].keyCbor, kvPairs[b].keyCbor) < 0
	})

	// Encode in sorted order
	for _, kv := range kvPairs {
		ret = append(ret, kv.keyCbor...)
		valueData, err := Encode(kv.value)
		if err != nil {
			return nil, err
		}
		ret = append(ret, valueData...)
	}

	ret = append(
		ret,
		// End indefinite length map
		byte(0xff),
	)
	return ret, nil
}
