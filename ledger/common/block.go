package common

import utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

type Block interface {
	BlockHeader
	Header() BlockHeader
	Type() int
	Transactions() []Transaction
	Utxorpc() (*utxorpc.Block, error)
}

type BlockHeader interface {
	Hash() Blake2b256
	PrevHash() Blake2b256
	BlockNumber() uint64
	SlotNumber() uint64
	IssuerVkey() IssuerVkey
	BlockBodySize() uint64
	Era() Era
	Cbor() []byte
	BlockBodyHash() Blake2b256
}
