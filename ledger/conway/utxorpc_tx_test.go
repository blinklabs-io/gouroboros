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

package conway_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
	"google.golang.org/protobuf/proto"
)

func filled(n int, b byte) []byte {
	return bytes.Repeat([]byte{b}, n)
}

// utxorpcTestTx encodes a Conway transaction that sets every field the
// UTxO-RPC projection is expected to carry.
func utxorpcTestTx(t *testing.T, isValid bool) []byte {
	t.Helper()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	rewardA := append([]byte{0xe1}, filled(28, 0x01)...)
	rewardB := append([]byte{0xe1}, filled(28, 0x02)...)
	policy := cbor.NewByteString(filled(28, 0x11))
	datum := cbor.RawTag{
		Number: 121,
		Content: mustEncode(
			t,
			[]any{uint64(1), []byte{0xab}, int64(-5)},
		),
	}
	body := map[uint]any{
		0: []any{
			[]any{filled(32, 0x09), uint64(0)},
			[]any{filled(32, 0x01), uint64(0)},
		},
		1: []any{[]any{enterprise, uint64(2_000_000)}},
		2: uint64(170_000),
		3: uint64(1000),
		5: map[cbor.ByteString]uint64{
			cbor.NewByteString(rewardB): 7,
			cbor.NewByteString(rewardA): 5000,
		},
		8: uint64(10),
		9: map[cbor.ByteString]map[cbor.ByteString]int64{
			policy: {
				cbor.NewByteString([]byte("a")): 5,
				cbor.NewByteString([]byte("b")): -3,
			},
		},
		13: []any{[]any{filled(32, 0x03), uint64(1)}},
		16: []any{enterprise, uint64(1000)},
		17: uint64(3000),
		20: []any{
			[]any{
				uint64(100),
				rewardA,
				[]any{uint64(6)},
				[]any{"https://example.com", filled(32, 0x04)},
			},
		},
	}
	witnesses := map[uint]any{
		0: []any{[]any{filled(32, 0x05), filled(64, 0x06)}},
		1: []any{[]any{uint64(0), filled(28, 0x07)}},
		4: []any{datum},
		// spend 0 (42, 1/2), spend 1 (5, 5/6), mint 0 (7, 3/4)
		5: cbor.RawMessage{
			0xa3,
			0x82, 0x00, 0x00, 0x82, 0x18, 0x2a, 0x82, 0x01, 0x02,
			0x82, 0x00, 0x01, 0x82, 0x05, 0x82, 0x05, 0x06,
			0x82, 0x01, 0x00, 0x82, 0x07, 0x82, 0x03, 0x04,
		},
	}
	aux := []any{
		map[uint]any{674: "hi"},
		[]any{[]any{uint64(0), filled(28, 0x07)}},
	}
	return mustEncode(t, []any{body, witnesses, isValid, aux})
}

func mustEncode(t *testing.T, v any) []byte {
	t.Helper()
	out, err := cbor.Encode(v)
	require.NoError(t, err)
	return out
}

func TestConwayTransactionUtxorpcProjectsEveryField(t *testing.T) {
	t.Parallel()
	tx, err := conway.NewConwayTransactionFromCbor(utxorpcTestTx(t, true))
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)

	rewardA := append([]byte{0xe1}, filled(28, 0x01)...)
	rewardB := append([]byte{0xe1}, filled(28, 0x02)...)
	nativeScript := &utxorpc.Script{
		Script: &utxorpc.Script_Native{
			Native: &utxorpc.NativeScript{
				NativeScript: &utxorpc.NativeScript_ScriptPubkey{
					ScriptPubkey: filled(28, 0x07),
				},
			},
		},
	}
	int64Big := func(v int64) *utxorpc.BigInt {
		return &utxorpc.BigInt{BigInt: &utxorpc.BigInt_Int{Int: v}}
	}

	require.True(t, got.Successful)
	require.True(t, proto.Equal(
		&utxorpc.TxValidity{Start: 10, Ttl: 1000},
		got.Validity,
	), "validity: %v", got.Validity)

	// Withdrawals use the ledger reward-account order, not map iteration.
	require.Len(t, got.Withdrawals, 2)
	require.Equal(t, rewardA, got.Withdrawals[0].RewardAccount)
	require.Equal(t, int64(5000), got.Withdrawals[0].Coin.GetInt())
	require.Equal(t, rewardB, got.Withdrawals[1].RewardAccount)
	require.Equal(t, int64(7), got.Withdrawals[1].Coin.GetInt())

	// Mint carries signed quantities, burns included.
	require.Len(t, got.Mint, 1)
	require.Equal(t, filled(28, 0x11), got.Mint[0].PolicyId)
	require.Len(t, got.Mint[0].Assets, 2)
	require.Equal(t, []byte("a"), got.Mint[0].Assets[0].Name)
	require.Equal(t, int64(5), got.Mint[0].Assets[0].GetMintCoin().GetInt())
	require.Equal(t, []byte("b"), got.Mint[0].Assets[1].Name)
	require.Equal(t, int64(-3), got.Mint[0].Assets[1].GetMintCoin().GetInt())
	require.Nil(t, got.Mint[0].Assets[1].GetOutputCoin())

	require.NotNil(t, got.Collateral)
	require.Len(t, got.Collateral.Collateral, 1)
	require.Equal(t, filled(32, 0x03), got.Collateral.Collateral[0].TxHash)
	require.Equal(t, uint32(1), got.Collateral.Collateral[0].OutputIndex)
	require.Equal(t, int64(1000), got.Collateral.CollateralReturn.Coin.GetInt())
	require.Equal(t, int64(3000), got.Collateral.TotalCollateral.GetInt())

	require.NotNil(t, got.Witnesses)
	require.True(t, proto.Equal(
		&utxorpc.VKeyWitness{
			Vkey:      filled(32, 0x05),
			Signature: filled(64, 0x06),
		},
		got.Witnesses.Vkeywitness[0],
	))
	require.Len(t, got.Witnesses.Vkeywitness, 1)
	require.Len(t, got.Witnesses.Script, 1)
	require.True(t, proto.Equal(nativeScript, got.Witnesses.Script[0]))
	require.Len(t, got.Witnesses.PlutusDatums, 1)
	require.True(t, proto.Equal(
		&utxorpc.PlutusData{
			PlutusData: &utxorpc.PlutusData_Constr{
				Constr: &utxorpc.Constr{
					Tag: 121,
					Fields: []*utxorpc.PlutusData{
						{
							PlutusData: &utxorpc.PlutusData_BigInt{
								BigInt: int64Big(1),
							},
						},
						{
							PlutusData: &utxorpc.PlutusData_BoundedBytes{
								BoundedBytes: []byte{0xab},
							},
						},
						{
							PlutusData: &utxorpc.PlutusData_BigInt{
								BigInt: int64Big(-5),
							},
						},
					},
				},
			},
		},
		got.Witnesses.PlutusDatums[0],
	), "datum: %v", got.Witnesses.PlutusDatums[0])

	// Redeemer indexes address the lexicographically sorted inputs, so spend
	// 0 belongs to the 0x01 input even though it is listed second.
	require.Len(t, got.Inputs, 2)
	first := got.Inputs[1].Redeemer
	require.Equal(t, filled(32, 0x01), got.Inputs[1].TxHash)
	require.Equal(
		t,
		utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_SPEND,
		first.Purpose,
	)
	require.Equal(t, uint32(0), first.Index)
	require.Equal(t, uint64(1), first.ExUnits.Memory)
	require.Equal(t, uint64(2), first.ExUnits.Steps)
	require.Equal(t, int64(42), first.Payload.GetBigInt().GetInt())
	second := got.Inputs[0].Redeemer
	require.Equal(t, uint32(1), second.Index)
	require.Equal(t, int64(5), second.Payload.GetBigInt().GetInt())
	mintRedeemer := got.Mint[0].Redeemer
	require.Equal(
		t,
		utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_MINT,
		mintRedeemer.Purpose,
	)
	require.Equal(t, int64(7), mintRedeemer.Payload.GetBigInt().GetInt())
	require.Equal(t, uint64(3), mintRedeemer.ExUnits.Memory)

	require.NotNil(t, got.Auxiliary)
	require.Len(t, got.Auxiliary.Metadata, 1)
	require.Equal(t, uint64(674), got.Auxiliary.Metadata[0].Label)
	require.Equal(t, "hi", got.Auxiliary.Metadata[0].Value.GetText())
	require.Len(t, got.Auxiliary.Scripts, 1)
	require.True(t, proto.Equal(nativeScript, got.Auxiliary.Scripts[0]))

	require.Len(t, got.Proposals, 1)
	proposal := got.Proposals[0]
	require.Equal(t, int64(100), proposal.Deposit.GetInt())
	require.Equal(t, rewardA, proposal.RewardAccount)
	require.NotNil(t, proposal.GovAction.GetInfoAction())
	require.Equal(t, uint32(6), proposal.GovAction.GetInfoAction())
	require.Equal(t, "https://example.com", proposal.Anchor.Url)
	require.Equal(t, filled(32, 0x04), proposal.Anchor.ContentHash)
}

func TestConwayTransactionUtxorpcSuccessfulFollowsValidity(t *testing.T) {
	t.Parallel()
	tx, err := conway.NewConwayTransactionFromCbor(utxorpcTestTx(t, false))
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.False(t, got.Successful)
}

func TestConwayTransactionUtxorpcOmitsAbsentFields(t *testing.T) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0: []any{[]any{filled(32, 0x01), uint64(0)}},
			1: []any{[]any{enterprise, uint64(2_000_000)}},
			2: uint64(170_000),
		},
		map[uint]any{},
		true,
		nil,
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.True(t, got.Successful)
	require.Nil(t, got.Validity)
	require.Nil(t, got.Collateral)
	require.Nil(t, got.Auxiliary)
	require.Empty(t, got.Withdrawals)
	require.Empty(t, got.Mint)
	require.Empty(t, got.Proposals)
}

func TestConwayTransactionUtxorpcPreservesExplicitZeroValidityEnd(
	t *testing.T,
) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0: []any{[]any{filled(32, 0x01), uint64(0)}},
			1: []any{[]any{enterprise, uint64(2_000_000)}},
			2: uint64(170_000),
			3: uint64(0),
		},
		map[uint]any{},
		true,
		nil,
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.NotNil(t, got.Validity)
	require.Zero(t, got.Validity.Ttl)
}

func TestConwayTransactionUtxorpcPreservesExplicitZeroTotalCollateral(
	t *testing.T,
) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0:  []any{[]any{filled(32, 0x01), uint64(0)}},
			1:  []any{[]any{enterprise, uint64(2_000_000)}},
			2:  uint64(170_000),
			17: uint64(0),
		},
		map[uint]any{},
		true,
		nil,
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.NotNil(t, got.Collateral)
	require.NotNil(t, got.Collateral.TotalCollateral)
	require.Zero(t, got.Collateral.TotalCollateral.GetInt())
}

// Reward redeemer indexes address withdrawals in cardano-ledger's
// reward-account order, which puts script credentials before key
// credentials; certificate redeemer indexes address the listed order.
func TestConwayTransactionUtxorpcAttachesRewardAndCertRedeemers(t *testing.T) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	keyAccount := append([]byte{0xe1}, filled(28, 0x01)...)
	scriptAccount := append([]byte{0xf1}, filled(28, 0x02)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0: []any{[]any{filled(32, 0x01), uint64(0)}},
			1: []any{[]any{enterprise, uint64(2_000_000)}},
			2: uint64(170_000),
			4: []any{
				[]any{uint64(0), []any{uint64(0), filled(28, 0x03)}},
				[]any{uint64(0), []any{uint64(1), filled(28, 0x04)}},
			},
			5: map[cbor.ByteString]uint64{
				cbor.NewByteString(keyAccount):    1,
				cbor.NewByteString(scriptAccount): 2,
			},
		},
		map[uint]any{
			// reward 0 (10, 1/1), cert 1 (11, 2/2)
			5: cbor.RawMessage{
				0xa2,
				0x82, 0x03, 0x00, 0x82, 0x0a, 0x82, 0x01, 0x01,
				0x82, 0x02, 0x01, 0x82, 0x0b, 0x82, 0x02, 0x02,
			},
		},
		true,
		nil,
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)

	require.Len(t, got.Withdrawals, 2)
	require.Equal(t, scriptAccount, got.Withdrawals[0].RewardAccount)
	require.Equal(t, keyAccount, got.Withdrawals[1].RewardAccount)
	reward := got.Withdrawals[0].Redeemer
	require.NotNil(t, reward)
	require.Equal(
		t,
		utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_REWARD,
		reward.Purpose,
	)
	require.Equal(t, int64(10), reward.Payload.GetBigInt().GetInt())
	require.Nil(t, got.Withdrawals[1].Redeemer)

	require.Len(t, got.Certificates, 2)
	require.Nil(t, got.Certificates[0].Redeemer)
	cert := got.Certificates[1].Redeemer
	require.NotNil(t, cert)
	require.Equal(
		t,
		utxorpc.RedeemerPurpose_REDEEMER_PURPOSE_CERT,
		cert.Purpose,
	)
	require.Equal(t, int64(11), cert.Payload.GetBigInt().GetInt())
	require.Equal(t, uint64(2), cert.ExUnits.Steps)
}

// A parameter change proposal carries the proposed update.
func TestConwayTransactionUtxorpcParameterChangeUpdate(t *testing.T) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	rewardAccount := append([]byte{0xe1}, filled(28, 0x01)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0: []any{[]any{filled(32, 0x01), uint64(0)}},
			1: []any{[]any{enterprise, uint64(2_000_000)}},
			2: uint64(170_000),
			20: []any{
				[]any{
					uint64(100),
					rewardAccount,
					[]any{
						uint64(0),
						nil,
						map[uint]any{
							0: uint64(44),
							3: uint64(16384),
							10: cbor.RawTag{
								Number:  30,
								Content: mustEncode(t, []uint64{3, 1000}),
							},
							20: []uint64{7, 8},
						},
						filled(28, 0x05),
					},
					[]any{"https://example.com", filled(32, 0x04)},
				},
			},
		},
		map[uint]any{},
		true,
		nil,
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.Len(t, got.Proposals, 1)
	change := got.Proposals[0].GovAction.GetParameterChangeAction()
	require.NotNil(t, change)
	require.Equal(t, filled(28, 0x05), change.PolicyHash)
	update := change.ProtocolParamUpdate
	require.NotNil(t, update)
	require.Equal(t, int64(44), update.MinFeeCoefficient.GetInt())
	require.Equal(t, uint64(16384), update.MaxTxSize)
	require.Equal(t, int32(3), update.MonetaryExpansion.Numerator)
	require.Equal(t, uint32(1000), update.MonetaryExpansion.Denominator)
	require.Equal(t, uint64(7), update.MaxExecutionUnitsPerTransaction.Memory)
	require.Nil(t, update.MinFeeConstant)
	require.Nil(t, update.PoolInfluence)
	require.Nil(t, update.Prices)
}

// Metadata integers outside int64 are valid on the wire and must not fail the
// transaction conversion.
func TestConwayTransactionUtxorpcWideMetadataInteger(t *testing.T) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0: []any{[]any{filled(32, 0x01), uint64(0)}},
			1: []any{[]any{enterprise, uint64(2_000_000)}},
			2: uint64(170_000),
		},
		map[uint]any{},
		true,
		map[uint]any{1: uint64(1<<64 - 1)},
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.Len(t, got.Auxiliary.Metadata, 1)
	require.Equal(t, int64(-1), got.Auxiliary.Metadata[0].Value.GetInt())
}

// A redeemer index past every item attaches to nothing, including indexes
// above the int32 range on 32-bit builds.
func TestConwayTransactionUtxorpcIgnoresOutOfRangeRedeemer(t *testing.T) {
	t.Parallel()
	enterprise := append([]byte{0x61}, filled(28, 0x0a)...)
	raw := mustEncode(t, []any{
		map[uint]any{
			0: []any{[]any{filled(32, 0x01), uint64(0)}},
			1: []any{[]any{enterprise, uint64(2_000_000)}},
			2: uint64(170_000),
		},
		map[uint]any{
			// spend 0xffffffff (1, 1/1)
			5: cbor.RawMessage{
				0xa1,
				0x82, 0x00, 0x1a, 0xff, 0xff, 0xff, 0xff,
				0x82, 0x01, 0x82, 0x01, 0x01,
			},
		},
		true,
		nil,
	})
	tx, err := conway.NewConwayTransactionFromCbor(raw)
	require.NoError(t, err)
	got, err := tx.Utxorpc()
	require.NoError(t, err)
	require.Nil(t, got.Inputs[0].Redeemer)
}
