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

package script_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

type scriptContextTransaction struct {
	common.Transaction
	txType          int
	outputs         []common.TransactionOutput
	produced        []common.Utxo
	referenceInputs []common.TransactionInput
}

func (t scriptContextTransaction) Type() int { return t.txType }

func (t scriptContextTransaction) Outputs() []common.TransactionOutput {
	return t.outputs
}

func (t scriptContextTransaction) Produced() []common.Utxo {
	return t.produced
}

func (t scriptContextTransaction) ReferenceInputs() []common.TransactionInput {
	if t.referenceInputs != nil {
		return t.referenceInputs
	}
	return t.Transaction.ReferenceInputs()
}

func TestTxInfoOutputsUseTransactionBodyOutputs(t *testing.T) {
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		bytes.Repeat([]byte{0x31}, common.AddressHashSize),
		nil,
	)
	require.NoError(t, err)
	bodyOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(address.String()).WithLovelace(1).Build()
	require.NoError(t, err)
	collateralOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(address.String()).WithLovelace(2).Build()
	require.NoError(t, err)
	for _, produced := range [][]common.Utxo{
		{{Output: collateralOutput}},
		nil,
	} {
		for _, era := range []struct {
			name   string
			typeID int
			build  func(common.Transaction) ([]common.TransactionOutput, error)
		}{
			{
				name: "Plutus V1", typeID: 5,
				build: func(tx common.Transaction) ([]common.TransactionOutput, error) {
					info, err := script.NewTxInfoV1FromTransaction(nil, tx, nil, false)
					return info.Outputs, err
				},
			},
			{
				name: "Plutus V2", typeID: 5,
				build: func(tx common.Transaction) ([]common.TransactionOutput, error) {
					info, err := script.NewTxInfoV2FromTransaction(nil, tx, nil, false)
					return info.Outputs, err
				},
			},
			{
				name: "Plutus V3", typeID: 6,
				build: func(tx common.Transaction) ([]common.TransactionOutput, error) {
					info, err := script.NewTxInfoV3FromTransaction(nil, tx, nil)
					return info.Outputs, err
				},
			},
		} {
			t.Run(era.name, func(t *testing.T) {
				tx := scriptContextTransaction{
					Transaction: mockledger.NewTransactionBuilder(),
					txType:      era.typeID,
					outputs:     []common.TransactionOutput{bodyOutput},
					produced:    produced,
				}
				outputs, err := era.build(tx)
				require.NoError(t, err)
				require.Len(t, outputs, 1)
				require.Same(t, bodyOutput, outputs[0])
			})
		}
	}
}

func TestTxInfoV1FiltersByronInputsAcrossEras(t *testing.T) {
	byronAddress, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		bytes.Repeat([]byte{0x42}, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	byronOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(byronAddress.String()).WithLovelace(1).Build()
	require.NoError(t, err)
	byronInput := shelley.NewShelleyTransactionInput(
		hex.EncodeToString(bytes.Repeat([]byte{0x43}, common.Blake2b256Size)), 0,
	)
	shelleyAddress, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		bytes.Repeat([]byte{0x31}, common.AddressHashSize),
		nil,
	)
	require.NoError(t, err)
	shelleyOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(shelleyAddress.String()).WithLovelace(1).Build()
	require.NoError(t, err)
	base := mockledger.NewTransactionBuilder()
	base.WithInputs(byronInput)
	resolved := []common.Utxo{{Id: byronInput, Output: byronOutput}}
	tx := scriptContextTransaction{
		Transaction: base,
		txType:      4,
		outputs:     []common.TransactionOutput{shelleyOutput},
	}
	info, err := script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, false)
	require.NoError(t, err)
	require.Empty(t, info.Inputs)
	require.Len(t, info.Outputs, 1)

	for _, eraType := range []int{5, 6, 7} {
		tx.txType = eraType
		info, err = script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, eraType >= 6)
		require.NoError(t, err)
		require.Empty(t, info.Inputs)
		_, err = script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, eraType >= 6)
		require.ErrorContains(t, err, "cannot represent a Byron TxOut")
	}
	tx.txType = 6
	_, err = script.NewTxInfoV3FromTransaction(validitySlotState{}, tx, resolved)
	require.ErrorContains(t, err, "cannot represent a Byron TxOut")
}

func TestTxInfoV1FiltersByronOutputsAcrossEras(t *testing.T) {
	byronAddress, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		bytes.Repeat([]byte{0x42}, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	byronOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(byronAddress.String()).WithLovelace(1).Build()
	require.NoError(t, err)
	shelleyAddress, err := common.NewAddressFromParts(
		common.AddressTypeKeyNone,
		common.AddressNetworkTestnet,
		bytes.Repeat([]byte{0x31}, common.AddressHashSize),
		nil,
	)
	require.NoError(t, err)
	shelleyOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(shelleyAddress.String()).WithLovelace(1).Build()
	require.NoError(t, err)
	input := shelley.NewShelleyTransactionInput(
		hex.EncodeToString(bytes.Repeat([]byte{0x43}, common.Blake2b256Size)), 0,
	)
	base := mockledger.NewTransactionBuilder()
	base.WithInputs(input)
	resolved := []common.Utxo{{Id: input, Output: shelleyOutput}}
	tx := scriptContextTransaction{
		Transaction: base,
		txType:      4,
		outputs:     []common.TransactionOutput{byronOutput},
	}
	info, err := script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, false)
	require.NoError(t, err)
	require.Len(t, info.Inputs, 1)
	require.Empty(t, info.Outputs)

	for _, eraType := range []int{5, 6, 7} {
		tx.txType = eraType
		info, err = script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, eraType >= 6)
		require.NoError(t, err)
		require.Empty(t, info.Outputs)
		_, err = script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, eraType >= 6)
		require.ErrorContains(t, err, "cannot represent a Byron TxOut")
	}
	tx.txType = 6
	_, err = script.NewTxInfoV3FromTransaction(validitySlotState{}, tx, resolved)
	require.ErrorContains(t, err, "cannot represent a Byron TxOut")
}

func TestTxInfoRejectsByronReferenceInputs(t *testing.T) {
	byronAddress, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		bytes.Repeat([]byte{0x42}, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	byronOutput, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(byronAddress.String()).WithLovelace(1).Build()
	require.NoError(t, err)
	reference := shelley.NewShelleyTransactionInput(
		hex.EncodeToString(bytes.Repeat([]byte{0x44}, common.Blake2b256Size)), 0,
	)
	resolved := []common.Utxo{{Id: reference, Output: byronOutput}}
	tx := scriptContextTransaction{
		Transaction:     mockledger.NewTransactionBuilder(),
		txType:          5,
		referenceInputs: []common.TransactionInput{reference},
	}
	for _, eraType := range []int{5, 6} {
		tx.txType = eraType
		_, err = script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, eraType >= 6)
		require.ErrorContains(t, err, "cannot represent a Byron TxOut")
	}
	tx.txType = 6
	_, err = script.NewTxInfoV3FromTransaction(validitySlotState{}, tx, resolved)
	require.ErrorContains(t, err, "cannot represent a Byron TxOut")
}
