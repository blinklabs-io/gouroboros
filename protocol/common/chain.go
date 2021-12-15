package common

type PointX struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_          struct{} `cbor:",toarray"`
	SlotNumber uint64
	Hash       uint64
}

type Point []interface{}

// point = origin / blockHeaderHash
// origin = [ ]
// blockHeaderHash = [ slotNo , int ]
// slotNo = word64
