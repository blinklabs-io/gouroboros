package cbor

import (
	"encoding/hex"
)

// Wrapper for bytestrings that allows them to be used as keys for a map
type ByteString struct {
	// We use a string because []byte isn't comparable, which means it can't be used as a map key
	data string
}

func NewByteString(data []byte) ByteString {
	bs := ByteString{
		data: string(data),
	}
	return bs
}

func (bs *ByteString) UnmarshalCBOR(data []byte) error {
	tmpValue := []byte{}
	if _, err := Decode(data, &tmpValue); err != nil {
		return err
	}
	bs.data = string(tmpValue)
	return nil
}

func (bs ByteString) Bytes() []byte {
	return []byte(bs.data)
}

func (bs ByteString) String() string {
	return hex.EncodeToString([]byte(bs.data))
}
