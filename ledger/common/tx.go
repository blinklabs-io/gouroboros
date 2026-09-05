// Copyright 2026 Blink Labs Software
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

// TransactionWithValidityIntervalUpperBound is implemented by transactions
// and transaction bodies that can distinguish an absent upper validity bound
// from an explicitly encoded bound of zero.
//
// TTL predates optional validity intervals and cannot represent that
// distinction by itself. Consumers should use
// TransactionValidityIntervalUpperBound when presence affects validation.
type TransactionWithValidityIntervalUpperBound interface {
	ValidityIntervalUpperBound() (uint64, bool)
}

// TransactionValidityIntervalUpperBound returns the upper validity bound and
// whether it is present. Implementations that do not expose presence retain the
// legacy behavior where a non-zero TTL is treated as present.
func TransactionValidityIntervalUpperBound(
	tx TransactionBody,
) (uint64, bool) {
	if txWithUpperBound, ok := tx.(TransactionWithValidityIntervalUpperBound); ok {
		return txWithUpperBound.ValidityIntervalUpperBound()
	}
	upperBound := tx.TTL()
	return upperBound, upperBound != 0
}

// TransactionWithCurrentTreasuryValuePresence is implemented by transactions
// and transaction bodies that can distinguish an absent current treasury value
// from an explicitly encoded value of zero.
type TransactionWithCurrentTreasuryValuePresence interface {
	CurrentTreasuryValuePresent() bool
}

// TransactionCurrentTreasuryValuePresent reports whether a transaction body's
// current treasury value is present. A nonzero value implies presence for
// legacy implementations. Zero is present only when the optional presence
// capability reports it explicitly, so legacy implementations that return a
// non-nil zero value for an absent field remain compatible.
func TransactionCurrentTreasuryValuePresent(tx TransactionBody) bool {
	value := tx.CurrentTreasuryValue()
	if value == nil {
		return false
	}
	if value.Sign() != 0 {
		return true
	}
	txWithPresence, ok := tx.(TransactionWithCurrentTreasuryValuePresence)
	return ok && txWithPresence.CurrentTreasuryValuePresent()
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

type TransactionWitnessSetWithPlutusV4 interface {
	PlutusV4Scripts() []PlutusV4Script
}

// TransactionWithSubTransactionWitnessSets exposes witness sets whose scripts
// are available to the top-level transaction. Dijkstra sub-transactions can
// provide a script needed by a top-level script purpose.
type TransactionWithSubTransactionWitnessSets interface {
	SubTransactionWitnessSets() []TransactionWitnessSet
}

// TransactionWithSubTransactionBodies exposes sub-transaction bodies in
// ledger-transition order. Dijkstra validates each sub-transaction before the
// top-level transaction.
type TransactionWithSubTransactionBodies interface {
	SubTransactionBodies() []TransactionBody
}

// TransactionWithSubTransactionOutputs exposes outputs created by nested
// transactions. Their reference scripts undergo the same phase-1 admission
// checks as top-level output reference scripts.
type TransactionWithSubTransactionOutputs interface {
	SubTransactionOutputs() []TransactionOutput
}

func PlutusV4ScriptsFromWitnessSet(
	w TransactionWitnessSet,
) []PlutusV4Script {
	if w == nil {
		return nil
	}
	w4, ok := w.(TransactionWitnessSetWithPlutusV4)
	if !ok {
		return nil
	}
	return w4.PlutusV4Scripts()
}

func SubTransactionWitnessSetsFromTransaction(
	t Transaction,
) []TransactionWitnessSet {
	if t == nil {
		return nil
	}
	withSubTxs, ok := t.(TransactionWithSubTransactionWitnessSets)
	if !ok {
		return nil
	}
	return withSubTxs.SubTransactionWitnessSets()
}

func SubTransactionBodiesFromTransaction(
	t Transaction,
) []TransactionBody {
	if t == nil {
		return nil
	}
	withSubTxs, ok := t.(TransactionWithSubTransactionBodies)
	if !ok {
		return nil
	}
	return withSubTxs.SubTransactionBodies()
}

func SubTransactionOutputsFromTransaction(
	t Transaction,
) []TransactionOutput {
	if t == nil {
		return nil
	}
	withSubTxs, ok := t.(TransactionWithSubTransactionOutputs)
	if !ok {
		return nil
	}
	return withSubTxs.SubTransactionOutputs()
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
	hash                              *Blake2b256
	validityIntervalUpperBoundPresent bool
	currentTreasuryValuePresent       bool
}

type transactionBodyFieldPresence struct {
	validityIntervalUpperBound bool
	currentTreasuryValue       bool
}

// decodeTransactionBodyFieldPresence scans a transaction-body map once and
// returns the presence of optional scalar fields whose zero values cannot be
// distinguished by the typed decode.
func decodeTransactionBodyFieldPresence(
	cborData []byte,
) (transactionBodyFieldPresence, error) {
	var bodyFields map[uint]cbor.RawMessage
	if _, err := cbor.Decode(cborData, &bodyFields); err != nil {
		return transactionBodyFieldPresence{}, err
	}
	_, upperBoundPresent := bodyFields[3]
	_, currentTreasuryValuePresent := bodyFields[21]
	return transactionBodyFieldPresence{
		validityIntervalUpperBound: upperBoundPresent,
		currentTreasuryValue:       currentTreasuryValuePresent,
	}, nil
}

// SetValidityIntervalUpperBoundPresence records whether transaction-body key 3
// is present. Era-specific transaction body decoders use this to preserve the
// distinction between an absent upper bound and an explicit zero. Calling it
// for a programmatically constructed body invalidates any stored CBOR.
func (b *TransactionBodyBase) SetValidityIntervalUpperBoundPresence(
	present bool,
) {
	b.validityIntervalUpperBoundPresent = present
	b.hash = nil
	b.SetCbor(nil)
}

// ValidityIntervalUpperBoundPresent reports whether transaction-body key 3 is
// present.
func (b *TransactionBodyBase) ValidityIntervalUpperBoundPresent() bool {
	return b.validityIntervalUpperBoundPresent
}

// SetCurrentTreasuryValuePresence records whether transaction-body key 21 is
// present. Era-specific transaction bodies keep the value in their existing
// scalar fields; this presence bit preserves the distinction between an absent
// value and an explicitly encoded zero. Calling it for a programmatically
// constructed body invalidates any stored CBOR.
func (b *TransactionBodyBase) SetCurrentTreasuryValuePresence(present bool) {
	b.currentTreasuryValuePresent = present
	b.hash = nil
	b.SetCbor(nil)
}

// CurrentTreasuryValuePresent reports whether transaction-body key 21 is
// present.
func (b *TransactionBodyBase) CurrentTreasuryValuePresent() bool {
	return b.currentTreasuryValuePresent
}

// DecodeValidityIntervalUpperBoundPresence records the presence of
// transaction-body key 3 from decoded CBOR. It must be called by era-specific
// body decoders after their typed decode succeeds.
func (b *TransactionBodyBase) DecodeValidityIntervalUpperBoundPresence(
	cborData []byte,
	upperBound uint64,
) error {
	if upperBound != 0 {
		b.validityIntervalUpperBoundPresent = true
		return nil
	}
	presence, err := decodeTransactionBodyFieldPresence(cborData)
	if err != nil {
		return err
	}
	b.validityIntervalUpperBoundPresent = presence.validityIntervalUpperBound
	return nil
}

// DecodeTransactionBodyFieldPresence records the presence of transaction-body
// keys 3 and 21 after a typed decode. Nonzero typed values imply presence. If
// either value is zero, the method performs one shared raw-map scan to retain
// the distinction between an absent field and an explicitly encoded zero.
func (b *TransactionBodyBase) DecodeTransactionBodyFieldPresence(
	cborData []byte,
	upperBound uint64,
	currentTreasuryValueNonzero bool,
) error {
	b.validityIntervalUpperBoundPresent = upperBound != 0
	b.currentTreasuryValuePresent = currentTreasuryValueNonzero
	if b.validityIntervalUpperBoundPresent &&
		b.currentTreasuryValuePresent {
		return nil
	}
	presence, err := decodeTransactionBodyFieldPresence(cborData)
	if err != nil {
		return err
	}
	if !b.validityIntervalUpperBoundPresent {
		b.validityIntervalUpperBoundPresent = presence.validityIntervalUpperBound
	}
	if !b.currentTreasuryValuePresent {
		b.currentTreasuryValuePresent = presence.currentTreasuryValue
	}
	return nil
}

// EncodeTransactionBodyWithValidityIntervalUpperBound encodes a constructed
// transaction body while retaining explicitly present zero values for the
// validity upper bound and current treasury value. Non-zero and absent values
// retain the normal generic transaction-body encoding.
func EncodeTransactionBodyWithValidityIntervalUpperBound(
	body TransactionBody,
) ([]byte, error) {
	cborData, err := cbor.EncodeGeneric(body)
	if err != nil {
		return nil, err
	}
	upperBound, present := TransactionValidityIntervalUpperBound(body)
	preserveUpperBoundZero := present && upperBound == 0
	treasuryValue := body.CurrentTreasuryValue()
	preserveTreasuryZero := TransactionCurrentTreasuryValuePresent(body) &&
		treasuryValue != nil && treasuryValue.Sign() == 0
	networkIdValue, networkIdPresent := body.(interface {
		NetworkIdPresent() bool
	})
	preserveNetworkIdZero := networkIdPresent && networkIdValue.NetworkIdPresent()
	if !preserveUpperBoundZero && !preserveTreasuryZero && !preserveNetworkIdZero {
		return cborData, nil
	}
	bodyFields := make(map[uint]cbor.RawMessage)
	if _, err := cbor.Decode(cborData, &bodyFields); err != nil {
		return nil, err
	}
	if preserveUpperBoundZero {
		encodedUpperBound, err := cbor.Encode(upperBound)
		if err != nil {
			return nil, err
		}
		bodyFields[3] = encodedUpperBound
	}
	if preserveTreasuryZero {
		encodedTreasuryValue, err := cbor.Encode(uint64(0))
		if err != nil {
			return nil, err
		}
		bodyFields[21] = encodedTreasuryValue
	}
	if preserveNetworkIdZero {
		if networkIdValue, ok := body.(interface{ TransactionNetworkId() *uint8 }); ok {
			if value := networkIdValue.TransactionNetworkId(); value != nil && *value == 0 {
				encodedNetworkId, err := cbor.Encode(uint8(0))
				if err != nil {
					return nil, err
				}
				bodyFields[15] = encodedNetworkId
			}
		}
	}
	return cbor.Encode(bodyFields)
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
	inputs := tx.Inputs()
	outputs := tx.Outputs()
	referenceInputs := tx.ReferenceInputs()
	certificates := tx.Certificates()

	txi := make([]*utxorpc.TxInput, 0, len(inputs))
	txo := make([]*utxorpc.TxOutput, 0, len(outputs))
	for _, i := range inputs {
		input, err := i.Utxorpc()
		if err != nil {
			return nil, err
		}
		txi = append(txi, input)
	}
	for _, o := range outputs {
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
	if len(referenceInputs) > 0 {
		ret.ReferenceInputs = make(
			[]*utxorpc.TxInput,
			0,
			len(referenceInputs),
		)
	}
	for _, ri := range referenceInputs {
		input, err := ri.Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.ReferenceInputs = append(ret.ReferenceInputs, input)
	}
	if len(certificates) > 0 {
		ret.Certificates = make(
			[]*utxorpc.Certificate,
			0,
			len(certificates),
		)
	}
	for _, c := range certificates {
		cert, err := c.Utxorpc()
		if err != nil {
			return nil, err
		}
		ret.Certificates = append(ret.Certificates, cert)
	}

	return ret, nil
}
