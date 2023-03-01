package cbor

import (
	"bytes"
	"fmt"

	_cbor "github.com/fxamacker/cbor/v2"
)

func Decode(dataBytes []byte, dest interface{}) (int, error) {
	data := bytes.NewReader(dataBytes)
	dec := _cbor.NewDecoder(data)
	err := dec.Decode(dest)
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
		return 0, fmt.Errorf("cannot return first item from empty list")
	}
	if listLen < int(CBOR_MAX_UINT_SIMPLE) {
		if cborData[1] <= CBOR_MAX_UINT_SIMPLE {
			return int(cborData[1]), nil
		}
	}
	// If we couldn't use the shortcut above, actually decode the list
	var tmp Value
	if _, err := Decode(cborData, &tmp); err != nil {
		return 0, err
	}
	// Make sure that the value is actually numeric
	switch v := tmp.Value.([]interface{})[0].(type) {
	// The upstream CBOR library uses uint64 by default for numeric values
	case uint64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("first list item was not numeric, found: %v", v)
	}
}

// Determine the length of a CBOR list
func ListLength(cborData []byte) (int, error) {
	// If the list length is <= the max simple uint, then we can extract the length
	// value straight from the byte slice (with a little math)
	if cborData[0] >= CBOR_TYPE_ARRAY && cborData[0] <= (CBOR_TYPE_ARRAY+CBOR_MAX_UINT_SIMPLE) {
		return int(cborData[0]) - int(CBOR_TYPE_ARRAY), nil
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
func DecodeById(cborData []byte, idMap map[int]interface{}) (interface{}, error) {
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
