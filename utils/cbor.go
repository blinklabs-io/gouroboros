package utils

import (
	"bytes"
	"github.com/fxamacker/cbor/v2"
)

func CborEncode(data interface{}) ([]byte, error) {
	dataBytes, err := cbor.Marshal(data)
	return dataBytes, err
}

func CborDecode(dataBytes []byte, dest interface{}) (int, error) {
	data := bytes.NewReader(dataBytes)
	dec := cbor.NewDecoder(data)
	err := dec.Decode(dest)
	return dec.NumBytesRead(), err
}
