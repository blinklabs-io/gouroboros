package common

import (
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"github.com/fxamacker/cbor/v2"
)

type Point struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_    struct{} `cbor:",toarray"`
	Slot uint64
	Hash []byte
}

func NewPoint(slot uint64, blockHash []byte) Point {
	return Point{
		Slot: slot,
		Hash: blockHash,
	}
}

func NewPointOrigin() Point {
	return Point{}
}

// A "point" can sometimes be empty, but the CBOR library gets grumpy about this
// when doing automatic decoding from an array, so we have to handle this case specially
func (p *Point) UnmarshalCBOR(data []byte) error {
	var tmp []interface{}
	if err := cbor.Unmarshal(data, &tmp); err != nil {
		return err
	}
	if len(tmp) > 0 {
		p.Slot = tmp[0].(uint64)
		p.Hash = tmp[1].([]byte)
	}
	return nil
}

func (p *Point) MarshalCBOR() ([]byte, error) {
	var data []interface{}
	if p.Slot == 0 && p.Hash == nil {
		// Return an empty list if values are zero
		data = make([]interface{}, 0)
	} else {
		data = []interface{}{p.Slot, p.Hash}
	}
	return utils.CborEncode(data)
}
