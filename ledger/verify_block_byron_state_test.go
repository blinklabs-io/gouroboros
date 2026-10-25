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

package ledger_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

const byronStateProtocolMagic = uint32(byron.MainnetProtocolMagic)

func TestVerifyBlockStateRejectsByronValueCreation(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{1},
		OutputIndex: 0,
	}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		11,
	)
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "output value")
}

func TestVerifyBlockStateAcceptsAuthorizedByronUtxoSpend(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name        string
		addressType uint64
	}{
		{name: "payment", addressType: common.ByronAddressTypePubkey},
		{name: "redeem", addressType: common.ByronAddressTypeRedeem},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := byron.ByronTransactionInput{
				TxId:        common.Blake2b256{byte(test.addressType + 1)},
				OutputIndex: 0,
			}
			tx, address := signedByronStateTransaction(t, test.addressType, input, 9)
			state := byronStateWithInput(input, address, 10)

			require.NoError(t, ledger.VerifyBlockState(
				byronStateBlock(tx),
				byronStateConfig(state),
			))
		})
	}
}

func TestVerifyBlockStateRejectsUnauthorizedByronInput(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{3},
		OutputIndex: 0,
	}
	tx, _ := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)
	_, otherAddress := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)
	state := byronStateWithInput(input, otherAddress, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "does not authorize input")
}

func TestVerifyBlockStateRejectsMissingByronWitness(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{6},
		OutputIndex: 0,
	}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)
	tx.Twit = nil
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "1 inputs but 0 witnesses")
}

func TestVerifyBlockStateRejectsDuplicateByronInput(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{7},
		OutputIndex: 0,
	}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)
	tx.Body.TxInputs = append(tx.Body.TxInputs, input)
	tx.Twit = append(tx.Twit, tx.Twit[0])
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "duplicate byron transaction input")
}

func TestVerifyBlockStateRejectsByronDoubleSpendInBlock(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{4},
		OutputIndex: 0,
	}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(
		byronStateBlock(tx, tx),
		byronStateConfig(state),
	)
	require.ErrorContains(t, err, "failed to resolve input")
}

func TestVerifyBlockStateRequiresByronUtxoState(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{
		TxId:        common.Blake2b256{5},
		OutputIndex: 0,
	}
	tx, _ := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)

	err := ledger.VerifyBlockState(
		byronStateBlock(tx),
		byronStateConfig(nil),
	)
	require.ErrorContains(t, err, "ledger state is required")
}

func TestVerifyBlockStateRejectsZeroFeeByronTransaction(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{TxId: common.Blake2b256{8}}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		10,
	)
	state := byronStateWithInput(input, address, 10)

	err := ledger.VerifyBlockState(byronStateBlock(tx), byronStateConfig(state))
	require.ErrorContains(t, err, "below minimum fee")
}

func TestVerifyBlockStateEnforcesByronSizeFeePolicy(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{TxId: common.Blake2b256{9}}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		100,
	)
	minFee := uint64(len(tx.Cbor()))
	config := func(state common.LedgerState) common.VerifyConfig {
		cfg := byronStateConfig(state)
		cfg.ByronTxFeePolicy = &common.ByronTxFeePolicy{Multiplier: 1_000_000_000}
		return cfg
	}

	err := ledger.VerifyBlockState(
		byronStateBlock(tx),
		config(byronStateWithInput(input, address, 100+minFee-1)),
	)
	require.ErrorContains(t, err, "below minimum fee")
	require.NoError(t, ledger.VerifyBlockState(
		byronStateBlock(tx),
		config(byronStateWithInput(input, address, 100+minFee)),
	))
}

func TestVerifyBlockStateExemptsRedeemOnlyByronSpendFromFee(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{TxId: common.Blake2b256{10}}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypeRedeem,
		input,
		10,
	)
	state := byronStateWithInput(input, address, 10)

	require.NoError(t, ledger.VerifyBlockState(
		byronStateBlock(tx),
		byronStateConfig(state),
	))
}

func TestVerifyBlockStateRequiresByronFeePolicy(t *testing.T) {
	t.Parallel()

	input := byron.ByronTransactionInput{TxId: common.Blake2b256{11}}
	tx, address := signedByronStateTransaction(
		t,
		common.ByronAddressTypePubkey,
		input,
		9,
	)
	cfg := byronStateConfig(byronStateWithInput(input, address, 10))
	cfg.ByronTxFeePolicy = nil

	err := ledger.VerifyBlockState(byronStateBlock(tx), cfg)
	require.ErrorContains(t, err, "requires a fee policy")
}

func TestByronTxFeePolicyMinFeeRoundsUp(t *testing.T) {
	t.Parallel()

	policy := common.ByronTxFeePolicy{
		Summand:    155_381_000_000_000,
		Multiplier: 43_946_000_000,
	}
	// 155381 + 43.946*10 = 155820.46 lovelace, rounded up.
	require.Equal(t, "155821", policy.MinFee(10).String())
	require.Equal(t, "155381", policy.MinFee(0).String())
}

func signedByronStateTransaction(
	t *testing.T,
	addressType uint64,
	input byron.ByronTransactionInput,
	outputAmount uint64,
) (byron.ByronTransaction, common.Address) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	verificationKey := append(append([]byte(nil), publicKey...), make([]byte, 32)...)
	attributes := common.ByronAddressAttributes{}
	var address common.Address
	switch addressType {
	case common.ByronAddressTypePubkey:
		rootData, err := cbor.Encode([]any{
			addressType,
			[]any{addressType, verificationKey},
			attributes,
		})
		require.NoError(t, err)
		rootDigest := sha3.Sum256(rootData)
		root := common.Blake2b224Hash(rootDigest[:])
		address, err = common.NewByronAddressFromParts(
			addressType,
			root[:],
			attributes,
		)
		require.NoError(t, err)
	case common.ByronAddressTypeRedeem:
		verificationKey = publicKey
		address, err = common.NewByronAddressRedeem(publicKey, attributes)
		require.NoError(t, err)
	default:
		t.Fatalf("unsupported Byron address type %d", addressType)
	}
	body := byron.ByronTransactionBody{
		TxInputs:   []byron.ByronTransactionInput{input},
		TxOutputs:  []byron.ByronTransactionOutput{{OutputAddress: address, OutputAmount: outputAmount}},
		Attributes: cbor.RawMessage{0xa0},
	}
	body.SetCbor([]byte{0x80})
	magicCbor, err := cbor.Encode(byronStateProtocolMagic)
	require.NoError(t, err)
	wireId := body.WireId()
	txIdCbor, err := cbor.Encode(wireId[:])
	require.NoError(t, err)
	tag := byte(byron.SignTagTx)
	if addressType == common.ByronAddressTypeRedeem {
		tag = byron.SignTagRedeemTx
	}
	signed := append([]byte{tag}, magicCbor...)
	signed = append(signed, txIdCbor...)
	payload, err := cbor.Encode([]any{
		verificationKey,
		ed25519.Sign(privateKey, signed),
	})
	require.NoError(t, err)
	witnessCbor, err := cbor.Encode([]any{
		addressType,
		cbor.WrappedCbor(payload),
	})
	require.NoError(t, err)
	var witness cbor.Value
	require.NoError(t, witness.UnmarshalCBOR(witnessCbor))
	return byron.ByronTransaction{Body: body, Twit: []cbor.Value{witness}}, address
}

func byronStateBlock(txs ...byron.ByronTransaction) *byron.ByronMainBlock {
	return &byron.ByronMainBlock{
		BlockHeader: &byron.ByronMainBlockHeader{ProtocolMagic: byronStateProtocolMagic},
		Body: byron.ByronMainBlockBody{
			TxPayload: txs,
		},
	}
}

func byronStateWithInput(
	input byron.ByronTransactionInput,
	address common.Address,
	amount uint64,
) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().WithUtxos([]common.Utxo{{
		Id: input,
		Output: &byron.ByronTransactionOutput{
			OutputAddress: address,
			OutputAmount:  amount,
		},
	}}).Build()
}

func byronStateConfig(state common.LedgerState) common.VerifyConfig {
	return common.VerifyConfig{
		LedgerState: state,
		ByronTxFeePolicy: &common.ByronTxFeePolicy{
			Summand: 1_000_000_000,
		},
		SkipStakePoolValidation:   true,
		SkipBlockLimitsValidation: true,
	}
}
