package cbor

import (
	"bytes"

	_cbor "github.com/fxamacker/cbor/v2"
)

func Encode(data interface{}) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	em, err := _cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	enc := em.NewEncoder(buf)
	err = enc.Encode(data)
	return buf.Bytes(), err
}
