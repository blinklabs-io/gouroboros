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

package common_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func TestResolveInputUtxoRejectsTypedNilOutput(t *testing.T) {
	input := shelley.MustNewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000000", 0,
	)
	var output *shelley.ShelleyTransactionOutput
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{Output: output}, nil
		},
	).Build()

	_, err := common.ResolveInputUtxo(state, input)
	require.ErrorIs(t, err, common.ErrInputResolution)
}

func TestValidateCollateralVKeyWitnessesAcceptsByronBootstrapWitness(t *testing.T) {
	input := shelley.MustNewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000000", 0,
	)
	publicKey := bytes.Repeat([]byte{0x01}, 32)
	chainCode := bytes.Repeat([]byte{0x02}, 32)
	attributes := []byte{0xa0}
	rootInput := []byte{0x83, 0x00, 0x82, 0x00, 0x58, 0x40}
	rootInput = append(rootInput, publicKey...)
	rootInput = append(rootInput, chainCode...)
	rootInput = append(rootInput, attributes...)
	rootDigest := sha3.Sum256(rootInput)
	root := common.Blake2b224Hash(rootDigest[:])
	address, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		root[:],
		common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{
				Output: babbage.BabbageTransactionOutput{
					OutputAddress: address,
				},
			}, nil
		},
	).Build()
	witness := common.BootstrapWitness{
		PublicKey:  publicKey,
		Signature:  bytes.Repeat([]byte{0x03}, 64),
		ChainCode:  chainCode,
		Attributes: attributes,
	}
	tx := mockledger.NewTransactionBuilder().
		WithCollateral(input).
		WithWitnesses(mockledger.NewMockTransactionWitnessSet().
			WithBootstrapWitnesses(witness))

	require.NoError(t, conway.UtxoValidateCollateralVKeyWitnesses(
		tx,
		0,
		state,
		&conway.ConwayProtocolParameters{},
	))

	witness.ChainCode[0] ^= 1
	invalidTx := mockledger.NewTransactionBuilder().
		WithCollateral(input).
		WithWitnesses(mockledger.NewMockTransactionWitnessSet().
			WithBootstrapWitnesses(witness))
	require.Error(t, conway.UtxoValidateCollateralVKeyWitnesses(
		invalidTx,
		0,
		state,
		&conway.ConwayProtocolParameters{},
	))
}

func TestValidateCollateralVKeyWitnessesRejectsNilOutput(t *testing.T) {
	input := shelley.MustNewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000000", 0,
	)
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{}, nil
		},
	).Build()
	redeemers := conway.ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend, Index: 0}: {},
		},
	}
	tx := mockledger.NewTransactionBuilder().
		WithCollateral(input).
		WithWitnesses(mockledger.NewMockTransactionWitnessSet().
			WithRedeemers(redeemers).
			WithVkeyWitnesses(common.VkeyWitness{Vkey: []byte("collateral-key")}))

	require.NotPanics(t, func() {
		err := common.ValidateCollateralVKeyWitnesses(tx, state)
		require.Error(t, err)
	})
}
