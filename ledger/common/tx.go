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

import (
	"github.com/blinklabs-io/gouroboros/cbor"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type Transaction interface {
	TransactionBody
	Type() int
	Cbor() []byte
	Metadata() *cbor.LazyValue
	IsValid() bool
	Consumed() []TransactionInput
	Produced() []Utxo
	Witnesses() TransactionWitnessSet
}

type TransactionBody interface {
	Cbor() []byte
	Fee() uint64
	Hash() string
	Inputs() []TransactionInput
	Outputs() []TransactionOutput
	TTL() uint64
	ProtocolParameterUpdates() (uint64, map[Blake2b224]ProtocolParameterUpdate)
	ValidityIntervalStart() uint64
	ReferenceInputs() []TransactionInput
	Collateral() []TransactionInput
	CollateralReturn() TransactionOutput
	TotalCollateral() uint64
	Certificates() []Certificate
	Withdrawals() map[*Address]uint64
	AuxDataHash() *Blake2b256
	RequiredSigners() []Blake2b224
	AssetMint() *MultiAsset[MultiAssetTypeMint]
	ScriptDataHash() *Blake2b256
	VotingProcedures() VotingProcedures
	ProposalProcedures() []ProposalProcedure
	CurrentTreasuryValue() int64
	Donation() uint64
	Utxorpc() *utxorpc.Tx
}

type TransactionInput interface {
	Id() Blake2b256
	Index() uint32
	String() string
	Utxorpc() *utxorpc.TxInput
}

type TransactionOutput interface {
	Address() Address
	Amount() uint64
	Assets() *MultiAsset[MultiAssetTypeOutput]
	Datum() *cbor.LazyValue
	DatumHash() *Blake2b256
	Cbor() []byte
	Utxorpc() *utxorpc.TxOutput
}

type TransactionWitnessSet interface {
	Vkey() []VkeyWitness
	NativeScripts() []NativeScript
	Bootstrap() []BootstrapWitness
	PlutusData() []cbor.Value
	PlutusV1Scripts() [][]byte
	PlutusV2Scripts() [][]byte
	PlutusV3Scripts() [][]byte
	Redeemers() TransactionWitnessRedeemers
}

type TransactionWitnessRedeemers interface {
	Indexes(RedeemerTag) []uint
	Value(uint, RedeemerTag) (cbor.LazyValue, ExUnits)
}

type Utxo struct {
	Id     TransactionInput
	Output TransactionOutput
}
