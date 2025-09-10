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

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
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
	babbage.BabbageBlockHeader
	hash *common.Blake2b256
	Body LeiosBlockHeaderBody
}

type LeiosBlockHeaderBody struct {
	babbage.BabbageBlockHeaderBody
	AnnouncedEb     *common.Blake2b256
	AnnouncedEbSize *uint32
	CertifiedEb     bool
}

type LeiosEndorserBlock struct {
	cbor.DecodeStoreCbor
	cbor.StructAsArray
	hash         *common.Blake2b256
	TxReferences []common.TxReference
}

func (b *LeiosEndorserBlock) BlockBodySize() uint64 {
	// Leios doesn't include the block body size in the endorser block
	return 0
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
	return LeiosBlockHeader{}
}

type LeiosRankingBlock struct {
	conway.ConwayBlock
	BlockHeader            *LeiosBlockHeader
	TransactionBodies      []conway.ConwayTransactionBody
	TransactionWitnessSets []conway.ConwayTransactionWitnessSet
	TransactionMetadataSet map[uint]*cbor.LazyValue
	InvalidTransactions    []uint
	EbCertificate          *common.LeiosEbCertificate
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
