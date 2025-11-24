// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package leios

// NOTE: Leios is still in development and experimental.
// Block structures and validation logic may change as the protocol evolves.
// It is acceptable to skip validation on Leios blocks, but tests must be maintained.

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

const (
	EraIdLeios   = 7
	EraNameLeios = "Leios"

	BlockTypeLeiosRanking  = 8
	BlockTypeLeiosEndorser = 9

	BlockHeaderTypeLeios = 7

	TxTypeLeios = 7
)

var EraLeios = common.Era{
	Id:   EraIdLeios,
	Name: EraNameLeios,
}

func init() {
	common.RegisterEra(EraLeios)
}

type LeiosBlockHeader struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	hash      *common.Blake2b256
	Body      LeiosBlockHeaderBody
	Signature []byte
}

type LeiosBlockHeaderBody struct {
	babbage.BabbageBlockHeaderBody
	AnnouncedEb     *common.Blake2b256
	AnnouncedEbSize *uint32
	CertifiedEb     *bool
}

type LeiosEndorserBlockBody struct {
	cbor.StructAsArray
	transactions []common.Transaction
	TxReferences map[common.Blake2b256]uint16
}

func (b *LeiosEndorserBlockBody) BlockBodyHash() common.Blake2b256 {
	// NOTE: Leios is still in development and experimental.
	// This implementation may change as the protocol evolves.
	// Compute hash of the block body content
	bodyCbor, err := cbor.Encode(b)
	if err != nil {
		// CBOR encoding failure indicates a serious structural issue
		// Panic loudly during development to catch problems early
		panic(fmt.Sprintf("Leios block body CBOR encoding failed: %v", err))
	}
	return common.Blake2b256Hash(bodyCbor)
}

type LeiosEndorserBlock struct {
	cbor.DecodeStoreCbor
	cbor.StructAsArray
	hash *common.Blake2b256
	Body *LeiosEndorserBlockBody
}

func (h *LeiosBlockHeader) UnmarshalCBOR(cborData []byte) error {
	type tLeiosBlockHeader LeiosBlockHeader
	var tmp tLeiosBlockHeader
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	*h = LeiosBlockHeader(tmp)
	h.SetCbor(cborData)
	return nil
}

func (h *LeiosBlockHeader) Hash() common.Blake2b256 {
	if h.hash == nil {
		tmpHash := common.Blake2b256Hash(h.Cbor())
		h.hash = &tmpHash
	}
	return *h.hash
}

func (h *LeiosBlockHeader) PrevHash() common.Blake2b256 {
	return h.Body.PrevHash
}

func (h *LeiosBlockHeader) BlockNumber() uint64 {
	return h.Body.BlockNumber
}

func (h *LeiosBlockHeader) SlotNumber() uint64 {
	return h.Body.Slot
}

func (h *LeiosBlockHeader) IssuerVkey() common.IssuerVkey {
	return h.Body.IssuerVkey
}

func (h *LeiosBlockHeader) BlockBodySize() uint64 {
	return h.Body.BlockBodySize
}

func (h *LeiosBlockHeader) Era() common.Era {
	return EraLeios
}

func (h *LeiosBlockHeader) BlockBodyHash() common.Blake2b256 {
	return h.Body.BlockBodyHash
}

func (LeiosEndorserBlock) Type() int {
	return BlockTypeLeiosEndorser
}

func (b *LeiosEndorserBlock) BlockBodySize() uint64 {
	// Get size for the entire block
	return uint64(len(b.Cbor()))
}

func (b *LeiosEndorserBlock) BlockNumber() uint64 {
	return 0
}

func (b *LeiosEndorserBlock) SlotNumber() uint64 {
	return 0
}

func (b *LeiosEndorserBlock) Era() common.Era {
	return EraLeios
}

func (b *LeiosEndorserBlock) Hash() common.Blake2b256 {
	if b.hash == nil {
		tmpHash := common.Blake2b256Hash(b.Cbor())
		b.hash = &tmpHash
	}
	return *b.hash
}

func (b *LeiosEndorserBlock) Header() common.BlockHeader {
	return &LeiosBlockHeader{}
}

func (b *LeiosEndorserBlock) IssuerVkey() common.IssuerVkey {
	// TODO: This will cause a problem in validation code
	return common.IssuerVkey{}
}

func (b *LeiosEndorserBlock) PrevHash() common.Blake2b256 {
	return common.Blake2b256{}
}

func (b *LeiosEndorserBlock) Transactions() []common.Transaction {
	if b.Body == nil {
		return nil
	}
	return b.Body.transactions
}

func (b *LeiosEndorserBlock) Utxorpc() (*utxorpc.Block, error) {
	// TODO: figure out how this fits into UTxO RPC
	return &utxorpc.Block{}, nil
}

func (b *LeiosEndorserBlock) BlockBodyHash() common.Blake2b256 {
	if b.Body == nil {
		// Panic on nil body to distinguish from empty body
		panic("LeiosEndorserBlock has nil body")
	}
	return b.Body.BlockBodyHash()
}

type LeiosRankingBlock struct {
	conway.ConwayBlock
	BlockHeader   *LeiosBlockHeader          `cbor:"0,keyasint"`
	EbCertificate *common.LeiosEbCertificate `cbor:"5,keyasint,omitempty,omitzero"`
}

func (LeiosRankingBlock) Type() int {
	return BlockTypeLeiosRanking
}

func (b *LeiosRankingBlock) BlockBodySize() uint64 {
	return b.BlockHeader.BlockBodySize()
}

func (b *LeiosRankingBlock) BlockNumber() uint64 {
	return b.BlockHeader.BlockNumber()
}

func (b *LeiosRankingBlock) SlotNumber() uint64 {
	return b.BlockHeader.SlotNumber()
}

func (b *LeiosRankingBlock) Era() common.Era {
	return EraLeios
}

func (b *LeiosRankingBlock) Hash() common.Blake2b256 {
	return b.BlockHeader.Hash()
}

func (b *LeiosRankingBlock) Header() common.BlockHeader {
	return b.BlockHeader
}

func (b *LeiosRankingBlock) IssuerVkey() common.IssuerVkey {
	return b.BlockHeader.IssuerVkey()
}

func (b *LeiosRankingBlock) PrevHash() common.Blake2b256 {
	return b.BlockHeader.PrevHash()
}

func (b *LeiosRankingBlock) Transactions() []common.Transaction {
	// TODO: decide if we resolve EB transactions here and return them
	invalidTxMap := make(map[uint]bool, len(b.InvalidTransactions))
	for _, invalidTxIdx := range b.InvalidTransactions {
		invalidTxMap[invalidTxIdx] = true
	}

	ret := make([]common.Transaction, len(b.TransactionBodies))
	// #nosec G115
	for idx := range b.TransactionBodies {
		ret[idx] = &conway.ConwayTransaction{
			Body:       b.TransactionBodies[idx],
			WitnessSet: b.TransactionWitnessSets[idx],
			TxMetadata: b.TransactionMetadataSet[uint(idx)],
			TxIsValid:  !invalidTxMap[uint(idx)],
		}
	}
	return ret
}

func (b *LeiosRankingBlock) Utxorpc() (*utxorpc.Block, error) {
	txs := []*utxorpc.Tx{}
	for _, t := range b.Transactions() {
		tx, err := t.Utxorpc()
		if err != nil {
			return nil, err
		}
		txs = append(txs, tx)
	}
	body := &utxorpc.BlockBody{
		Tx: txs,
	}
	header := &utxorpc.BlockHeader{
		Hash:   b.Hash().Bytes(),
		Height: b.BlockNumber(),
		Slot:   b.SlotNumber(),
	}
	block := &utxorpc.Block{
		Body:   body,
		Header: header,
	}
	return block, nil
}

func (b *LeiosRankingBlock) BlockBodyHash() common.Blake2b256 {
	if b.BlockHeader == nil {
		panic("LeiosRankingBlock has nil BlockHeader")
	}
	return b.Header().BlockBodyHash()
}

func NewLeiosEndorserBlockFromCbor(data []byte) (*LeiosEndorserBlock, error) {
	var leiosEndorserBlock LeiosEndorserBlock
	if _, err := cbor.Decode(data, &leiosEndorserBlock); err != nil {
		return nil, fmt.Errorf("decode Leios endorser block error: %w", err)
	}
	return &leiosEndorserBlock, nil
}

func NewLeiosRankingBlockFromCbor(data []byte) (*LeiosRankingBlock, error) {
	var leiosRankingBlock LeiosRankingBlock
	if _, err := cbor.Decode(data, &leiosRankingBlock); err != nil {
		return nil, fmt.Errorf("decode Leios ranking block error: %w", err)
	}
	return &leiosRankingBlock, nil
}

func NewLeiosBlockHeaderFromCbor(data []byte) (*LeiosBlockHeader, error) {
	var leiosBlockHeader LeiosBlockHeader
	if _, err := cbor.Decode(data, &leiosBlockHeader); err != nil {
		return nil, fmt.Errorf("decode Leios block header error: %w", err)
	}
	return &leiosBlockHeader, nil
}

func NewLeiosTransactionBodyFromCbor(
	data []byte,
) (*LeiosTransactionBody, error) {
	var leiosTx LeiosTransactionBody
	if _, err := cbor.Decode(data, &leiosTx); err != nil {
		return nil, fmt.Errorf("decode Leios transaction body error: %w", err)
	}
	return &leiosTx, nil
}

func NewLeiosTransactionFromCbor(data []byte) (*LeiosTransaction, error) {
	var leiosTx LeiosTransaction
	if _, err := cbor.Decode(data, &leiosTx); err != nil {
		return nil, fmt.Errorf("decode Leios transaction error: %w", err)
	}
	return &leiosTx, nil
}
