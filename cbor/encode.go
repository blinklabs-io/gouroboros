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
	"bytes"
	"fmt"
	"reflect"

	_cbor "github.com/fxamacker/cbor/v2"
	"github.com/jinzhu/copier"
)

func Encode(data interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	em, err := _cbor.CoreDetEncOptions().EncModeWithTags(customTagSet)
	if err != nil {
		return nil, err
	}
	enc := em.NewEncoder(buf)
	err = enc.Encode(data)
	return buf.Bytes(), err
}

// EncodeGeneric encodes the specified object to CBOR without using the source object's
// MarshalCBOR() function
func EncodeGeneric(src interface{}) ([]byte, error) {
	// Create a duplicate(-ish) struct from the destination
	// We do this so that we can bypass any custom UnmarshalCBOR() function on the
	// destination object
	valueSrc := reflect.ValueOf(src)
	if valueSrc.Kind() != reflect.Pointer ||
		valueSrc.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("source must be a pointer to a struct")
	}
	typeSrcElem := valueSrc.Elem().Type()
	srcTypeFields := []reflect.StructField{}
	for i := 0; i < typeSrcElem.NumField(); i++ {
		tmpField := typeSrcElem.Field(i)
		if tmpField.IsExported() && tmpField.Name != "DecodeStoreCbor" {
			srcTypeFields = append(srcTypeFields, tmpField)
		}
	}
	// Create temporary object with the type created above
	tmpSrc := reflect.New(reflect.StructOf(srcTypeFields))
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

type IndefLengthList struct {
	Items []any
}

func (i *IndefLengthList) MarshalCBOR() ([]byte, error) {
	ret := []byte{
		// Start indefinite-length list
		0x9f,
	}
	for _, item := range i.Items {
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
