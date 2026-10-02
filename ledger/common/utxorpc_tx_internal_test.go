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

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func TestPlutusDataToUtxorpcConstrTags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		index     uint64
		tag       uint32
		anyConstr uint64
	}{
		{0, 121, 0},
		{6, 127, 0},
		{7, 1280, 0},
		{127, 1400, 0},
		{128, 102, 128},
	} {
		got, err := plutusDataToUtxorpc(data.NewConstr(tc.index))
		require.NoError(t, err)
		require.Equal(t, tc.tag, got.GetConstr().Tag, "index %d", tc.index)
		require.Equal(t, tc.anyConstr, got.GetConstr().AnyConstructor)
	}
}

func TestPlutusDataToUtxorpcContainers(t *testing.T) {
	t.Parallel()
	pd := data.NewMap([][2]data.PlutusData{{
		data.NewByteString([]byte{1}),
		data.NewList(
			data.NewInteger(
				new(big.Int).Sub(big.NewInt(-1<<63), big.NewInt(1)),
			),
			data.NewConstr(1),
		),
	}})
	got, err := plutusDataToUtxorpc(pd)
	require.NoError(t, err)
	pair := got.GetMap().Pairs[0]
	require.Equal(t, []byte{1}, pair.Key.GetBoundedBytes())
	items := pair.Value.GetArray().Items
	require.Len(t, items, 2)
	require.Equal(
		t,
		new(big.Int).Lsh(big.NewInt(1), 63).Bytes(),
		items[0].GetBigInt().GetBigNInt(),
	)
	require.Equal(t, uint32(122), items[1].GetConstr().Tag)
}

func TestNativeScriptToUtxorpcNested(t *testing.T) {
	t.Parallel()
	hash := make([]byte, 28)
	pub := NativeScript{item: &NativeScriptPubkey{Hash: hash}}
	script := &NativeScriptAll{Scripts: []NativeScript{
		pub,
		{item: &NativeScriptAny{Scripts: []NativeScript{pub}}},
		{item: &NativeScriptNofK{N: -1, Scripts: []NativeScript{pub}}},
		{item: &NativeScriptInvalidBefore{Slot: 5}},
		{item: &NativeScriptInvalidHereafter{Slot: 9}},
	}}
	got, err := nativeScriptToUtxorpc(script)
	require.NoError(t, err)
	items := got.GetScriptAll().Items
	require.Len(t, items, 5)
	require.Equal(t, hash, items[0].GetScriptPubkey())
	require.Len(t, items[1].GetScriptAny().Items, 1)
	// A non-positive threshold is always satisfied and is carried as zero.
	require.Equal(t, uint32(0), items[2].GetScriptNOfK().K)
	require.Equal(t, uint64(5), items[3].GetInvalidBefore())
	require.Equal(t, uint64(9), items[4].GetInvalidHereafter())

	_, err = nativeScriptToUtxorpc(&NativeScriptRequireGuard{})
	require.ErrorContains(t, err, "unsupported native script")
}

func TestMetadatumToUtxorpc(t *testing.T) {
	t.Parallel()
	m := MetaMap{Pairs: []MetaPair{{
		Key: MetaText{Value: "k"},
		Value: MetaList{Items: []TransactionMetadatum{
			MetaInt{Value: big.NewInt(-7)},
			MetaBytes{Value: []byte{9}},
		}},
	}}}
	got, err := metadatumToUtxorpc(m)
	require.NoError(t, err)
	pair := got.GetMap().Pairs[0]
	require.Equal(t, "k", pair.Key.GetText())
	items := pair.Value.GetArray().Items
	require.Equal(t, int64(-7), items[0].GetInt())
	require.Equal(t, []byte{9}, items[1].GetBytes())

	_, err = metadatumToUtxorpc(
		MetaInt{Value: new(big.Int).Lsh(big.NewInt(1), 63)},
	)
	require.ErrorContains(t, err, "does not fit in int64")
}

func TestGovActionToUtxorpc(t *testing.T) {
	t.Parallel()
	id := &GovActionId{GovActionIdx: 3}
	id.TransactionId[0] = 0xaa
	addr := &Address{}
	cred := Credential{CredType: CredentialTypeAddrKeyHash}
	quorum := cbor.Rat{Rat: big.NewRat(2, 3)}

	hardFork := &HardForkInitiationGovAction{ActionId: id}
	hardFork.ProtocolVersion.Major = 10
	got, err := govActionToUtxorpc(hardFork)
	require.NoError(t, err)
	require.Equal(t, uint32(10), got.GetHardForkInitiationAction().ProtocolVersion.Major)
	require.Equal(
		t,
		uint32(3),
		got.GetHardForkInitiationAction().GovActionId.GovernanceActionIndex,
	)

	got, err = govActionToUtxorpc(&TreasuryWithdrawalGovAction{
		Withdrawals: map[*Address]uint64{addr: 11},
		PolicyHash:  []byte{1},
	})
	require.NoError(t, err)
	require.Equal(
		t,
		int64(11),
		got.GetTreasuryWithdrawalsAction().Withdrawals[0].Coin.GetInt(),
	)

	got, err = govActionToUtxorpc(&NoConfidenceGovAction{})
	require.NoError(t, err)
	require.NotNil(t, got.GetNoConfidenceAction())
	require.Nil(t, got.GetNoConfidenceAction().GovActionId)

	got, err = govActionToUtxorpc(&UpdateCommitteeGovAction{
		Credentials: []Credential{cred},
		CredEpochs:  map[*Credential]uint64{&cred: 42},
		Quorum:      quorum,
	})
	require.NoError(t, err)
	update := got.GetUpdateCommitteeAction()
	require.Len(t, update.RemoveCommitteeCredentials, 1)
	require.Equal(t, uint32(42), update.NewCommitteeCredentials[0].ExpiresEpoch)
	require.Equal(t, int32(2), update.NewCommitteeThreshold.Numerator)
	require.Equal(t, uint32(3), update.NewCommitteeThreshold.Denominator)

	constitution := &NewConstitutionGovAction{}
	constitution.Constitution.Anchor.Url = "u"
	constitution.Constitution.ScriptHash = []byte{7}
	got, err = govActionToUtxorpc(constitution)
	require.NoError(t, err)
	require.Equal(
		t,
		"u",
		got.GetNewConstitutionAction().Constitution.Anchor.Url,
	)
	require.Equal(t, []byte{7}, got.GetNewConstitutionAction().Constitution.Hash)

	_, err = govActionToUtxorpc(nil)
	require.ErrorContains(t, err, "unsupported governance action")
}
