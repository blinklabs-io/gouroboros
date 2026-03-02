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

package common

// Related files:
//   - state.go: LedgerState interface for UTxO queries
//   - witness.go: TransactionWitnessSet returned by Witnesses()
//   - utxo.go: Utxo type returned by Produced()
//   - ledger/{era}/shelley.go: Era-specific Transaction implementations
//   - rules.go: Validation rules that operate on Transaction

import (
	"iter"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type Transaction interface {
	TransactionBody
	Type() int
	Cbor() []byte
	Hash() Blake2b256
	LeiosHash() Blake2b256
	Metadata() TransactionMetadatum
	AuxiliaryData() AuxiliaryData
	IsValid() bool
	Consumed() []TransactionInput
	Produced() []Utxo
	Witnesses() TransactionWitnessSet
}

type TransactionBody interface {
	Cbor() []byte
	Fee() *big.Int
	Id() Blake2b256
	Inputs() []TransactionInput
	Outputs() []TransactionOutput
	TTL() uint64
	ProtocolParameterUpdates() (uint64, map[Blake2b224]ProtocolParameterUpdate)
	ValidityIntervalStart() uint64
	ReferenceInputs() []TransactionInput
	Collateral() []TransactionInput
	CollateralReturn() TransactionOutput
	TotalCollateral() *big.Int
	Certificates() []Certificate
	Withdrawals() map[*Address]*big.Int
	AuxDataHash() *Blake2b256
	RequiredSigners() []Blake2b224
	AssetMint() *MultiAsset[MultiAssetTypeMint]
	ScriptDataHash() *Blake2b256
	VotingProcedures() VotingProcedures
	ProposalProcedures() []ProposalProcedure
	CurrentTreasuryValue() *big.Int
	Donation() *big.Int
	Utxorpc() (*utxorpc.Tx, error)
}

type TransactionInput interface {
	Id() Blake2b256
	Index() uint32
	String() string
	Utxorpc() (*utxorpc.TxInput, error)
	ToPlutusData() data.PlutusData
}

type TransactionOutput interface {
	Address() Address
	Amount() *big.Int
	Assets() *MultiAsset[MultiAssetTypeOutput]
	Datum() *Datum
	DatumHash() *Blake2b256
	Cbor() []byte
	Utxorpc() (*utxorpc.TxOutput, error)
	ScriptRef() Script
	ToPlutusData() data.PlutusData
	String() string
}

type TransactionWitnessSet interface {
	Vkey() []VkeyWitness
	NativeScripts() []NativeScript
	Bootstrap() []BootstrapWitness
	PlutusData() []Datum
	PlutusV1Scripts() []PlutusV1Script
	PlutusV2Scripts() []PlutusV2Script
	PlutusV3Scripts() []PlutusV3Script
	Redeemers() TransactionWitnessRedeemers
}

type TransactionWitnessRedeemers interface {
	Indexes(RedeemerTag) []uint
	Value(uint, RedeemerTag) RedeemerValue
	Iter() iter.Seq2[RedeemerKey, RedeemerValue]
}

type Utxo struct {
	cbor.StructAsArray
	Id     TransactionInput
	Output TransactionOutput
}

// TransactionBodyBase provides a set of functions that return empty values to satisfy the
// TransactionBody interface. It also provides functionality for generating a transaction hash
// and storing/retrieving the original CBOR
type TransactionBodyBase struct {
	cbor.DecodeStoreCbor
	hash *Blake2b256
}

func (b *TransactionBodyBase) Id() Blake2b256 {
	if b.hash == nil {
		tmpHash := Blake2b256Hash(b.Cbor())
		b.hash = &tmpHash
	}
	return *b.hash
}

func (b *TransactionBodyBase) Inputs() []TransactionInput {
	return nil
}

func (b *TransactionBodyBase) Outputs() []TransactionOutput {
	return nil
}

func (b *TransactionBodyBase) Fee() *big.Int {
	return nil
}

func (b *TransactionBodyBase) TTL() uint64 {
	return 0
}

func (b *TransactionBodyBase) ValidityIntervalStart() uint64 {
	return 0
}

func (b *TransactionBodyBase) ReferenceInputs() []TransactionInput {
	return []TransactionInput{}
}

func (b *TransactionBodyBase) Collateral() []TransactionInput {
	return nil
}

func (b *TransactionBodyBase) CollateralReturn() TransactionOutput {
	return nil
}

func (b *TransactionBodyBase) TotalCollateral() *big.Int {
	return nil
}

func (b *TransactionBodyBase) Certificates() []Certificate {
	return nil
}

func (b *TransactionBodyBase) Withdrawals() map[*Address]*big.Int {
	return nil
}

func (b *TransactionBodyBase) AuxDataHash() *Blake2b256 {
	return nil
}

func (b *TransactionBodyBase) RequiredSigners() []Blake2b224 {
	return nil
}

func (b *TransactionBodyBase) AssetMint() *MultiAsset[MultiAssetTypeMint] {
	return nil
}

func (b *TransactionBodyBase) ScriptDataHash() *Blake2b256 {
	return nil
}

func (b *TransactionBodyBase) VotingProcedures() VotingProcedures {
	return nil
}

func (b *TransactionBodyBase) ProposalProcedures() []ProposalProcedure {
	return nil
}

func (b *TransactionBodyBase) CurrentTreasuryValue() *big.Int {
	return nil
}

func (b *TransactionBodyBase) Donation() *big.Int {
	return nil
}

func (b *TransactionBodyBase) Utxorpc() (*utxorpc.Tx, error) {
	return nil, nil
}

// TransactionBodyToUtxorpc is a common helper for converting TransactionBody to utxorpc.Tx
func TransactionBodyToUtxorpc(tx TransactionBody) (*utxorpc.Tx, error) {
	txi := []*utxorpc.TxInput{}
	txo := []*utxorpc.TxOutput{}
	for _, i := range tx.Inputs() {
		input, err := i.Utxorpc()
		if err != nil {
			return nil, err
		}
		txi = append(txi, input)
	}
	for _, o := range tx.Outputs() {
		output, err := o.Utxorpc()
		if err != nil {
			return nil, err
		}
		txo = append(txo, output)
	}
	ret := &utxorpc.Tx{
		Inputs:  txi,
		Outputs: txo,
		// Certificates:    tx.Certificates(),
		// Withdrawals:     tx.Withdrawals(),
		// Mint:            tx.Mint(),
		// ReferenceInputs: tx.ReferenceInputs(),
		// Witnesses:       tx.Witnesses(),
		// Collateral:      tx.Collateral(),
		Fee: BigIntToUtxorpcBigInt(tx.Fee()),
		// Validity:        tx.Validity(),
		// Successful:      tx.Successful(),
		// Auxiliary:       tx.AuxData(),
		Hash: tx.Id().Bytes(),
		// Proposals:       tx.ProposalProcedures(),
	}
	for _, ri := range tx.ReferenceInputs() {
		input, err := ri.Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.ReferenceInputs = append(ret.ReferenceInputs, input)
	}
	for _, c := range tx.Certificates() {
		cert, err := c.Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.Certificates = append(ret.Certificates, cert)
	}

	return ret, nil
}
