package utils

import (
	"bytes"
	"github.com/fxamacker/cbor/v2"
)

// CborEncode encodes the provided data as CBOR using the correct mode
func CborEncode(data interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	em, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	enc := em.NewEncoder(buf)
	err = enc.Encode(data)
	return buf.Bytes(), err
}

// CborDecode decodes the provided CBOR into the given destination
func CborDecode(dataBytes []byte, dest interface{}) (int, error) {
	data := bytes.NewReader(dataBytes)
	dec := cbor.NewDecoder(data)
	err := dec.Decode(dest)
	return dec.NumBytesRead(), err
}
