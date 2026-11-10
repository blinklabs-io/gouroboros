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
)

// The ledger satisfies every needed key hash from the union of the vkey
// witness hashes and the address roots of the bootstrap witnesses, wherever
// the need comes from.

func bootstrapOnlyWitnesses(
	w common.BootstrapWitness,
) *mockledger.MockTransactionWitnessSet {
	return mockledger.NewMockTransactionWitnessSet().WithBootstrapWitnesses(w)
}

func TestValidateRequiredVKeyWitnessesAcceptsBootstrapAddressRoot(
	t *testing.T,
) {
	t.Parallel()
	witness := bootstrapTestWitness()
	root := bootstrapTestAddrRoot(witness)

	t.Run("required signer is a bootstrap address root", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().
			WithRequiredSigners(root).
			WithWitnesses(bootstrapOnlyWitnesses(witness))
		require.NoError(t, common.ValidateRequiredVKeyWitnesses(tx))
	})

	t.Run("required signer matches no witness", func(t *testing.T) {
		t.Parallel()
		other := common.Blake2b224Hash([]byte("not-the-address-root"))
		tx := mockledger.NewTransactionBuilder().
			WithRequiredSigners(other).
			WithWitnesses(bootstrapOnlyWitnesses(witness))
		var target common.MissingRequiredVKeyWitnessForSignerError
		require.ErrorAs(t, common.ValidateRequiredVKeyWitnesses(tx), &target)
		require.Equal(t, other, target.Signer)
	})

	t.Run("public key hash is not the address root", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().
			WithRequiredSigners(common.Blake2b224Hash(witness.PublicKey)).
			WithWitnesses(bootstrapOnlyWitnesses(witness))
		var target common.MissingRequiredVKeyWitnessForSignerError
		require.ErrorAs(t, common.ValidateRequiredVKeyWitnesses(tx), &target)
	})

	t.Run("malformed bootstrap witness provides no hash", func(t *testing.T) {
		t.Parallel()
		malformed := witness
		malformed.PublicKey = malformed.PublicKey[:31]
		tx := mockledger.NewTransactionBuilder().
			WithRequiredSigners(root).
			WithWitnesses(bootstrapOnlyWitnesses(malformed))
		var target common.MissingVKeyWitnessesError
		require.ErrorAs(t, common.ValidateRequiredVKeyWitnesses(tx), &target)
	})
}

func keyHashInputFixture(
	t *testing.T,
	keyHash common.Blake2b224,
) (shelley.ShelleyTransactionInput, common.LedgerState) {
	t.Helper()
	input := shelley.MustNewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000000", 0,
	)
	address, err := common.NewAddressFromParts(
		common.AddressTypeKeyKey,
		common.AddressNetworkTestnet,
		keyHash.Bytes(),
		bytes.Repeat([]byte{0x55}, common.AddressHashSize),
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
	return input, state
}

func TestValidateInputVKeyWitnessesAcceptsBootstrapAddressRoot(t *testing.T) {
	t.Parallel()
	witness := bootstrapTestWitness()
	root := bootstrapTestAddrRoot(witness)
	input, state := keyHashInputFixture(t, root)

	t.Run("Shelley key-hash input locked to the address root", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().
			WithWitnesses(bootstrapOnlyWitnesses(witness))
		tx.WithInputs(input)
		require.NoError(t, common.ValidateInputVKeyWitnesses(tx, state))
	})

	t.Run("no witness for the input", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().
			WithWitnesses(mockledger.NewMockTransactionWitnessSet())
		tx.WithInputs(input)
		require.ErrorContains(
			t,
			common.ValidateInputVKeyWitnesses(tx, state),
			"missing vkey witness for input",
		)
	})

	t.Run("bootstrap root differs from the input key hash", func(t *testing.T) {
		t.Parallel()
		otherInput, otherState := keyHashInputFixture(
			t,
			common.Blake2b224Hash([]byte("some-other-key")),
		)
		tx := mockledger.NewTransactionBuilder().
			WithWitnesses(bootstrapOnlyWitnesses(witness))
		tx.WithInputs(otherInput)
		require.ErrorContains(
			t,
			common.ValidateInputVKeyWitnesses(tx, otherState),
			"missing vkey witness for input",
		)
	})
}

func TestValidateInputVKeyWitnessesByronInputMessage(t *testing.T) {
	t.Parallel()
	witness := bootstrapTestWitness()
	root := bootstrapTestAddrRoot(witness)
	address, err := common.NewByronAddressFromParts(
		common.ByronAddressTypePubkey,
		root.Bytes(),
		common.ByronAddressAttributes{},
	)
	require.NoError(t, err)
	input := shelley.MustNewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000000", 0,
	)
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{
				Output: babbage.BabbageTransactionOutput{
					OutputAddress: address,
				},
			}, nil
		},
	).Build()

	t.Run("matching bootstrap witness", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().
			WithWitnesses(bootstrapOnlyWitnesses(witness))
		tx.WithInputs(input)
		require.NoError(t, common.ValidateInputVKeyWitnesses(tx, state))
	})

	t.Run("no witness", func(t *testing.T) {
		t.Parallel()
		tx := mockledger.NewTransactionBuilder().
			WithWitnesses(mockledger.NewMockTransactionWitnessSet())
		tx.WithInputs(input)
		require.ErrorContains(
			t,
			common.ValidateInputVKeyWitnesses(tx, state),
			"missing bootstrap witness for Byron input",
		)
	})
}

func TestValidateCollateralVKeyWitnessesAcceptsBootstrapAddressRoot(
	t *testing.T,
) {
	t.Parallel()
	witness := bootstrapTestWitness()
	root := bootstrapTestAddrRoot(witness)
	input, state := keyHashInputFixture(t, root)
	redeemers := conway.ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{Tag: common.RedeemerTagSpend}: {},
		},
	}
	newTx := func(w common.BootstrapWitness) *mockledger.MockTransaction {
		return mockledger.NewTransactionBuilder().
			WithCollateral(input).
			WithWitnesses(
				bootstrapOnlyWitnesses(w).WithRedeemers(redeemers),
			)
	}

	require.NoError(
		t,
		common.ValidateCollateralVKeyWitnesses(newTx(witness), state),
	)

	other := witness
	other.ChainCode = bytes.Repeat([]byte{0x77}, 32)
	require.ErrorContains(
		t,
		common.ValidateCollateralVKeyWitnesses(newTx(other), state),
		"missing vkey witness for collateral input",
	)
}
