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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestWithdrawalAuthorizationUsesRewardCredentialType(t *testing.T) {
	vkey := bytes.Repeat([]byte{0x31}, 32)
	keyHash := common.Blake2b224Hash(vkey)
	keyAddr, err := common.NewAddressFromParts(
		common.AddressTypeNoneKey,
		common.AddressNetworkMainnet,
		nil,
		keyHash.Bytes(),
	)
	require.NoError(t, err)

	nativeScriptCbor, err := cbor.Encode(common.NativeScriptPubkey{
		Type: 0,
		Hash: keyHash.Bytes(),
	})
	require.NoError(t, err)
	var nativeScript common.NativeScript
	require.NoError(t, nativeScript.UnmarshalCBOR(nativeScriptCbor))
	scriptHash := nativeScript.Hash()
	scriptAddr, err := common.NewAddressFromParts(
		common.AddressTypeNoneScript,
		common.AddressNetworkMainnet,
		nil,
		scriptHash.Bytes(),
	)
	require.NoError(t, err)

	keyTx := mockledger.NewTransactionBuilder().WithWithdrawals(
		map[*common.Address]uint64{&keyAddr: 1},
	)
	require.ErrorAs(
		t,
		common.ValidateRequiredVKeyWitnesses(keyTx),
		&common.MissingVKeyWitnessesError{},
	)
	keyTx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().WithVkeyWitnesses(
			common.VkeyWitness{Vkey: vkey},
		),
	)
	require.NoError(t, common.ValidateRequiredVKeyWitnesses(keyTx))

	scriptTx := mockledger.NewTransactionBuilder().WithWithdrawals(
		map[*common.Address]uint64{&scriptAddr: 1},
	)
	require.NoError(t, common.ValidateRequiredVKeyWitnesses(scriptTx))
	ls := mockledger.NewLedgerStateBuilder().Build()
	require.ErrorAs(
		t,
		common.ValidateScriptWitnesses(scriptTx, ls),
		&common.MissingScriptWitnessesError{},
	)
	scriptTx.WithWitnesses(
		mockledger.NewMockTransactionWitnessSet().WithNativeScripts(nativeScript),
	)
	require.NoError(t, common.ValidateScriptWitnesses(scriptTx, ls))
}

func TestWithdrawalAuthorizationRejectsNonRewardAddress(t *testing.T) {
	hash := bytes.Repeat([]byte{0x44}, common.AddressHashSize)
	baseAddr, err := common.NewAddressFromParts(
		common.AddressTypeKeyKey,
		common.AddressNetworkMainnet,
		hash,
		hash,
	)
	require.NoError(t, err)
	tx := mockledger.NewTransactionBuilder().WithWithdrawals(
		map[*common.Address]uint64{&baseAddr: 1},
	)
	require.ErrorContains(
		t,
		common.ValidateRequiredVKeyWitnesses(tx),
		"not a reward account",
	)
	require.ErrorContains(
		t,
		common.ValidateScriptWitnesses(
			tx,
			mockledger.NewLedgerStateBuilder().Build(),
		),
		"not a reward account",
	)
}
