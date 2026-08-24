package test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// DecodeHexString is a helper function for tests that decodes hex strings. It doesn't return
// an error value, which makes it usable inline.
func DecodeHexString(hexData string) []byte {
	// Strip off any leading/trailing whitespace in hex string
	hexData = strings.TrimSpace(hexData)
	decoded, err := hex.DecodeString(hexData)
	if err != nil {
		panic(fmt.Sprintf("error decoding hex: %s", err))
	}
	return decoded
}

// ListEncoding pairs a CBOR list encoding with a descriptive test name.
type ListEncoding struct {
	Name string
	Data []byte
}

// CanonicalAndNonShortestList returns the provided canonical short-form CBOR
// list and equivalent encodings using every wider definite-length form.
// Cardano decoders accept all of these forms, so tagged-union dispatch must not
// assume that the first item always starts at byte one.
func CanonicalAndNonShortestList(canonical []byte) []ListEncoding {
	if len(canonical) == 0 || canonical[0] < 0x80 || canonical[0] > 0x97 {
		panic("expected a canonical short-form CBOR list")
	}
	canonicalCopy := bytes.Clone(canonical)
	listLen := canonical[0] - 0x80
	withHeader := func(name string, header []byte) ListEncoding {
		data := make([]byte, len(header)+len(canonical)-1)
		copy(data, header)
		copy(data[len(header):], canonical[1:])
		return ListEncoding{Name: name, Data: data}
	}
	return []ListEncoding{
		{Name: "canonical", Data: canonicalCopy},
		withHeader("non-shortest-uint8-list-length", []byte{0x98, listLen}),
		withHeader("non-shortest-uint16-list-length", []byte{0x99, 0, listLen}),
		withHeader(
			"non-shortest-uint32-list-length",
			[]byte{0x9a, 0, 0, 0, listLen},
		),
		withHeader(
			"non-shortest-uint64-list-length",
			[]byte{0x9b, 0, 0, 0, 0, 0, 0, 0, listLen},
		),
	}
}

// JsonStringsEqual is a helper function for tests that compares JSON strings. To account for
// differences in whitespace, map key ordering, etc., we unmarshal the JSON strings into
// objects and then compare the objects
func JsonStringsEqual(jsonData1 []byte, jsonData2 []byte) bool {
	// Short-circuit for the happy path where they match exactly
	if bytes.Equal(jsonData1, jsonData2) {
		return true
	}
	// Decode provided JSON strings
	var tmpObj1 any
	if err := json.Unmarshal(jsonData1, &tmpObj1); err != nil {
		return false
	}
	var tmpObj2 any
	if err := json.Unmarshal(jsonData2, &tmpObj2); err != nil {
		return false
	}
	return reflect.DeepEqual(tmpObj1, tmpObj2)
}
