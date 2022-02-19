package utils

import (
	"bytes"
	"github.com/fxamacker/cbor/v2"
)

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

func CborDecode(dataBytes []byte, dest interface{}) (int, error) {
	data := bytes.NewReader(dataBytes)
	dec := cbor.NewDecoder(data)
	err := dec.Decode(dest)
	return dec.NumBytesRead(), err
}
