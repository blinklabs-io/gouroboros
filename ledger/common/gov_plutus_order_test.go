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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func TestTreasuryWithdrawalsPlutusDataUsesLedgerAddressOrder(t *testing.T) {
	hash := func(value byte) []byte { return bytes.Repeat([]byte{value}, AddressHashSize) }
	makeAddress := func(addressType, network uint8, credential []byte) *Address {
		address, err := NewAddressFromParts(addressType, network, nil, credential)
		require.NoError(t, err)
		return &address
	}
	keyTest := makeAddress(AddressTypeNoneKey, AddressNetworkTestnet, hash(1))
	scriptTestHigh := makeAddress(AddressTypeNoneScript, AddressNetworkTestnet, hash(2))
	scriptTestLow := makeAddress(AddressTypeNoneScript, AddressNetworkTestnet, hash(1))
	scriptMain := makeAddress(AddressTypeNoneScript, AddressNetworkMainnet, hash(1))
	action := TreasuryWithdrawalGovAction{
		Withdrawals: map[*Address]uint64{
			keyTest:        10,
			scriptTestHigh: 20,
			scriptTestLow:  30,
			scriptMain:     40,
		},
	}
	encoded := action.ToPlutusData().(*data.Constr)
	withdrawals := encoded.Fields[0].(*data.Map)
	expected := []*Address{scriptTestLow, scriptTestHigh, keyTest, scriptMain}
	require.Len(t, withdrawals.Pairs, len(expected))
	for idx, address := range expected {
		require.True(t, address.ToPlutusData().Equal(withdrawals.Pairs[idx][0]))
	}

	first, err := data.Encode(action.ToPlutusData())
	require.NoError(t, err)
	for range 20 {
		next, err := data.Encode(action.ToPlutusData())
		require.NoError(t, err)
		require.Equal(t, first, next)
	}
}

func TestUpdateCommitteePlutusDataUsesLedgerCredentialOrder(t *testing.T) {
	credential := func(typ uint, value byte) Credential {
		var hash CredentialHash
		copy(hash[:], bytes.Repeat([]byte{value}, len(hash)))
		return Credential{CredType: typ, Credential: hash}
	}
	key := credential(CredentialTypeAddrKeyHash, 1)
	scriptHigh := credential(CredentialTypeScriptHash, 2)
	scriptLow := credential(CredentialTypeScriptHash, 1)
	action := UpdateCommitteeGovAction{
		Credentials: []Credential{key, scriptHigh, scriptLow},
		CredEpochs: map[*Credential]uint64{
			&key:        10,
			&scriptHigh: 20,
			&scriptLow:  30,
		},
		Quorum: cbor.Rat{Rat: big.NewRat(1, 2)},
	}
	encoded := action.ToPlutusData().(*data.Constr)
	removed := encoded.Fields[1].(*data.List)
	added := encoded.Fields[2].(*data.Map)
	expected := []Credential{scriptLow, scriptHigh, key}
	require.Len(t, removed.Items, len(expected))
	require.Len(t, added.Pairs, len(expected))
	for idx, cred := range expected {
		credData := cred.ToPlutusData()
		require.True(t, credData.Equal(removed.Items[idx]))
		require.True(t, credData.Equal(added.Pairs[idx][0]))
	}

	first, err := data.Encode(action.ToPlutusData())
	require.NoError(t, err)
	for range 20 {
		next, err := data.Encode(action.ToPlutusData())
		require.NoError(t, err)
		require.Equal(t, first, next)
	}
}
