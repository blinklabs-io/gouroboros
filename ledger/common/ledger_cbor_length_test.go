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
	"github.com/stretchr/testify/require"
)

func TestMultiAssetMarshalCBORUsesIndefiniteMapsAbove23(t *testing.T) {
	t.Run("policy map", func(t *testing.T) {
		assets := make(map[cbor.ByteString]uint64, 1)
		assets[cbor.NewByteString([]byte{1})] = 1
		data := make(map[Blake2b224]map[cbor.ByteString]uint64, 24)
		for i := range 24 {
			var policy Blake2b224
			policy[0] = byte(i + 1)
			data[policy] = assets
		}
		multiAsset := NewMultiAsset(data)

		encoded, err := cbor.Encode(&multiAsset)
		require.NoError(t, err)
		require.Equal(t, byte(0xbf), encoded[0])
		require.Equal(t, byte(0xff), encoded[len(encoded)-1])
	})

	t.Run("asset-name map", func(t *testing.T) {
		var policy Blake2b224
		policy[0] = 1
		assets := make(map[cbor.ByteString]uint64, 24)
		for i := range 24 {
			assets[cbor.NewByteString([]byte{byte(i)})] = uint64(i + 1)
		}
		multiAsset := NewMultiAsset(map[Blake2b224]map[cbor.ByteString]uint64{
			policy: assets,
		})

		encoded, err := cbor.Encode(&multiAsset)
		require.NoError(t, err)
		encodedPolicy, err := cbor.Encode(policy)
		require.NoError(t, err)
		innerMapOffset := 1 + len(encodedPolicy)
		require.Equal(t, byte(0xa1), encoded[0])
		require.Equal(t, byte(0xbf), encoded[innerMapOffset])
		require.Equal(t, byte(0xff), encoded[len(encoded)-1])
	})
}

func TestVotingProceduresMarshalCBORUsesIndefiniteMapsAbove23(t *testing.T) {
	t.Run("voter map", func(t *testing.T) {
		procedures := make(VotingProcedures, 24)
		for i := range 24 {
			voter := Voter{Type: VoterTypeDRepKeyHash}
			voter.Hash[0] = byte(i + 1)
			var actionId GovActionId
			actionId.TransactionId[0] = 1
			procedures[&voter] = map[*GovActionId]VotingProcedure{
				&actionId: {Vote: GovVoteYes},
			}
		}

		encoded, err := cbor.Encode(procedures)
		require.NoError(t, err)
		require.Equal(t, byte(0xbf), encoded[0])
		require.Equal(t, byte(0xff), encoded[len(encoded)-1])
	})

	t.Run("action map", func(t *testing.T) {
		voter := Voter{Type: VoterTypeDRepKeyHash}
		actions := make(map[*GovActionId]VotingProcedure, 24)
		for i := range 24 {
			actionId := GovActionId{}
			actionId.TransactionId[0] = byte(i + 1)
			actions[&actionId] = VotingProcedure{Vote: GovVoteYes}
		}
		procedures := VotingProcedures{&voter: actions}

		encoded, err := cbor.Encode(procedures)
		require.NoError(t, err)
		encodedVoter, err := cbor.Encode(&voter)
		require.NoError(t, err)
		innerMapOffset := 1 + len(encodedVoter)
		require.Equal(t, byte(0xa1), encoded[0])
		require.Equal(t, byte(0xbf), encoded[innerMapOffset])
		require.Equal(t, byte(0xff), encoded[len(encoded)-1])
	})
}

func TestRewardAccountMapsUseIndefiniteEncodingAbove23(t *testing.T) {
	withdrawals := make(map[*Address]uint64, 24)
	addresses := make([]*Address, 24)
	for i := range 24 {
		hash := make([]byte, AddressHashSize)
		hash[0] = byte(i + 1)
		address, err := NewAddressFromParts(
			AddressTypeNoneKey,
			AddressNetworkTestnet,
			nil,
			hash,
		)
		require.NoError(t, err)
		addresses[i] = &address
		withdrawals[addresses[i]] = uint64(i + 1)
	}

	encodedWithdrawals, err := EncodeRewardAccountMap(withdrawals)
	require.NoError(t, err)
	wantWithdrawals := []byte{0xbf}
	for i, address := range addresses {
		encodedAddress, encodeErr := cbor.Encode(address)
		require.NoError(t, encodeErr)
		encodedAmount, encodeErr := cbor.Encode(uint64(i + 1))
		require.NoError(t, encodeErr)
		wantWithdrawals = append(wantWithdrawals, encodedAddress...)
		wantWithdrawals = append(wantWithdrawals, encodedAmount...)
	}
	wantWithdrawals = append(wantWithdrawals, 0xff)
	require.Equal(t, wantWithdrawals, encodedWithdrawals)

	body := withdrawalOrderTransactionBody{TxWithdrawals: withdrawals}
	encodedBody, err := EncodeTransactionBodyWithValidityIntervalUpperBound(&body)
	require.NoError(t, err)
	var bodyFields map[uint]cbor.RawMessage
	_, err = cbor.DecodeLedgerMap(encodedBody, &bodyFields)
	require.NoError(t, err)
	require.Equal(t, encodedWithdrawals, []byte(bodyFields[5]))

	action := TreasuryWithdrawalGovAction{
		Type:        uint(GovActionTypeTreasuryWithdrawal),
		Withdrawals: withdrawals,
	}
	encodedAction, err := cbor.Encode(action)
	require.NoError(t, err)
	var actionFields []cbor.RawMessage
	_, err = cbor.DecodeExact(encodedAction, &actionFields)
	require.NoError(t, err)
	if len(actionFields) < 2 {
		t.Fatalf("decoded treasury withdrawal action has %d fields", len(actionFields))
	}
	require.Equal(t, encodedWithdrawals, []byte(actionFields[1]))
}

func TestUpdateCommitteeMarshalCBORUsesIndefiniteSetAndMapAbove23(t *testing.T) {
	credentials := make([]Credential, 24)
	credentialPointers := make([]*Credential, 24)
	epochs := make(map[*Credential]uint64, 24)
	for i := range 24 {
		credentials[i].CredType = CredentialTypeAddrKeyHash
		credentials[i].Credential[0] = byte(24 - i)
		credentialPointers[i] = &credentials[i]
		epochs[credentialPointers[i]] = uint64(24 - i)
	}
	action := UpdateCommitteeGovAction{
		Type:        uint(GovActionTypeUpdateCommittee),
		Credentials: credentials,
		CredEpochs:  epochs,
		Quorum:      cbor.Rat{Rat: big.NewRat(1, 2)},
	}

	encoded, err := cbor.Encode(action)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.DecodeExact(encoded, &fields)
	require.NoError(t, err)
	if len(fields) <= 2 || len(fields[2]) < 4 {
		t.Fatalf("decoded update committee action does not contain its removed set")
	}
	require.Equal(t, []byte{0xd9, 0x01, 0x02, 0x9f}, []byte(fields[2])[:4])
	require.Equal(t, byte(0xff), fields[2][len(fields[2])-1])
	var removedSet cbor.SetType[Credential]
	_, err = cbor.Decode([]byte(fields[2]), &removedSet)
	require.NoError(t, err)
	wantCredentials := make([]Credential, 24)
	for i := range wantCredentials {
		wantCredentials[i].CredType = CredentialTypeAddrKeyHash
		wantCredentials[i].Credential[0] = byte(i + 1)
	}
	removedCredentials := removedSet.Items()
	require.Len(t, removedCredentials, len(wantCredentials))
	for i := range wantCredentials {
		require.Equal(t, wantCredentials[i].CredType, removedCredentials[i].CredType)
		require.Equal(t, wantCredentials[i].Credential, removedCredentials[i].Credential)
	}
	if len(fields) <= 3 || len(fields[3]) == 0 {
		t.Fatalf("decoded update committee action does not contain its epoch map")
	}
	require.Equal(t, byte(0xbf), fields[3][0])
	require.Equal(t, byte(0xff), fields[3][len(fields[3])-1])
	wantEpochMap := []byte{0xbf}
	for i := range 24 {
		credential := Credential{CredType: CredentialTypeAddrKeyHash}
		credential.Credential[0] = byte(i + 1)
		encodedCredential, encodeErr := cbor.Encode(&credential)
		require.NoError(t, encodeErr)
		encodedEpoch, encodeErr := cbor.Encode(uint64(i + 1))
		require.NoError(t, encodeErr)
		wantEpochMap = append(wantEpochMap, encodedCredential...)
		wantEpochMap = append(wantEpochMap, encodedEpoch...)
	}
	wantEpochMap = append(wantEpochMap, 0xff)
	require.Equal(t, wantEpochMap, []byte(fields[3]))
}

func TestPoolRegistrationOwnersUseIndefiniteSetAbove23(t *testing.T) {
	base := PoolRegistrationCertificate{
		CertType:      uint(CertificateTypePoolRegistration),
		Operator:      NewBlake2b224(make([]byte, Blake2b224Size)),
		VrfKeyHash:    NewBlake2b256(make([]byte, Blake2b256Size)),
		Pledge:        1,
		Cost:          1,
		Margin:        NewGenesisRat(1, 2),
		RewardAccount: NewBlake2b224(make([]byte, Blake2b224Size)),
		PoolOwners:    make([]AddrKeyHash, 24),
		Relays:        []PoolRelay{},
	}
	for i := range base.PoolOwners {
		base.PoolOwners[i][0] = byte(24 - i)
	}
	require.NoError(t, base.SetRewardAccountCredential(Credential{
		CredType: CredentialTypeAddrKeyHash,
	}, AddressNetworkTestnet))

	encoded, err := cbor.Encode(base)
	require.NoError(t, err)
	var fields []cbor.RawMessage
	_, err = cbor.DecodeExact(encoded, &fields)
	require.NoError(t, err)
	if len(fields) <= 7 || len(fields[7]) < 4 {
		t.Fatalf("decoded pool registration certificate does not contain its owner set")
	}
	require.Equal(t, []byte{0xd9, 0x01, 0x02, 0x9f}, []byte(fields[7])[:4])
	require.Equal(t, byte(0xff), fields[7][len(fields[7])-1])
	var ownerSet cbor.SetType[AddrKeyHash]
	_, err = cbor.Decode([]byte(fields[7]), &ownerSet)
	require.NoError(t, err)
	wantOwners := make([]AddrKeyHash, 24)
	for i := range wantOwners {
		wantOwners[i][0] = byte(i + 1)
	}
	require.Equal(t, wantOwners, ownerSet.Items())
}
