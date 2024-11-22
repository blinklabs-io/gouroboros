package common

import utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"

type Block interface {
	BlockHeader
	Header() BlockHeader
	Type() int
	Transactions() []Transaction
	Utxorpc() *utxorpc.Block
}

type BlockHeader interface {
	Hash() string
	PrevHash() string
	BlockNumber() uint64
	SlotNumber() uint64
	IssuerVkey() IssuerVkey
	BlockBodySize() uint64
	Era() Era
	Cbor() []byte
}
