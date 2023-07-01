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

// JsonStringsEqual is a helper function for tests that compares JSON strings. To account for
// differences in whitespace, map key ordering, etc., we unmarshal the JSON strings into
// objects and then compare the objects
func JsonStringsEqual(jsonData1 []byte, jsonData2 []byte) bool {
	// Short-circuit for the happy path where they match exactly
	if bytes.Equal(jsonData1, jsonData2) {
		return true
	}
	// Decode provided JSON strings
	var tmpObj1 interface{}
	if err := json.Unmarshal(jsonData1, &tmpObj1); err != nil {
		return false
	}
	var tmpObj2 interface{}
	if err := json.Unmarshal(jsonData2, &tmpObj2); err != nil {
		return false
	}
	return reflect.DeepEqual(tmpObj1, tmpObj2)
}
