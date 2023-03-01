// The common package contains types used by multiple mini-protocols
package common

import (
	"github.com/cloudstruct/go-ouroboros-network/cbor"
)

// The Point type represents a point on the blockchain. It consists of a slot number and block hash
type Point struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_    struct{} `cbor:",toarray"`
	Slot uint64
	Hash []byte
}

// NewPoint returns a Point object with the specified slot number and block hash
func NewPoint(slot uint64, blockHash []byte) Point {
	return Point{
		Slot: slot,
		Hash: blockHash,
	}
}

// NewPointOrigin returns an "empty" Point object which represents the origin of the blockchain
func NewPointOrigin() Point {
	return Point{}
}

// UnmarshalCBOR is a helper function for decoding a Point object from CBOR. The object content can vary,
// so we need to do some special handling when decoding. It is not intended to be called directly.
func (p *Point) UnmarshalCBOR(data []byte) error {
	var tmp []interface{}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	if len(tmp) > 0 {
		p.Slot = tmp[0].(uint64)
		p.Hash = tmp[1].([]byte)
	}
	return nil
}

// MarshalCBOR is a helper function for encoding a Point object to CBOR. The object content can vary, so we
// need to do some special handling when encoding. It is not intended to be called directly.
func (p *Point) MarshalCBOR() ([]byte, error) {
	var data []interface{}
	if p.Slot == 0 && p.Hash == nil {
		// Return an empty list if values are zero
		data = make([]interface{}, 0)
	} else {
		data = []interface{}{p.Slot, p.Hash}
	}
	return cbor.Encode(data)
}
