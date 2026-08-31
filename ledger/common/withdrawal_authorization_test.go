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
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
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
		mockledger.NewMockTransactionWitnessSet().
			WithNativeScripts(nativeScript),
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

func TestShelleyFamilyValidationRulesEnforceWithdrawalScriptAuthorization(
	t *testing.T,
) {
	vkey := bytes.Repeat([]byte{0x52}, 32)
	nativeScriptCbor, err := cbor.Encode(common.NativeScriptPubkey{
		Type: 0,
		Hash: common.Blake2b224Hash(vkey).Bytes(),
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
	ls := mockledger.NewLedgerStateBuilder().Build()

	tests := []struct {
		name  string
		rules []common.UtxoValidationRuleFunc
	}{
		{name: "Shelley", rules: shelley.UtxoValidationRules},
		{name: "Allegra", rules: allegra.UtxoValidationRules},
		{name: "Mary", rules: mary.UtxoValidationRules},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authRules := make([]common.UtxoValidationRuleFunc, 0, 2)
			for _, rule := range test.rules {
				name := runtime.FuncForPC(reflect.ValueOf(rule).Pointer()).
					Name()
				if strings.HasSuffix(name, ".UtxoValidateScriptWitnesses") ||
					strings.HasSuffix(name, ".UtxoValidateNativeScripts") {
					authRules = append(authRules, rule)
				}
			}
			require.Len(
				t,
				authRules,
				2,
				"authorization rules are not registered",
			)

			tx := mockledger.NewTransactionBuilder().WithWithdrawals(
				map[*common.Address]uint64{&scriptAddr: 1},
			)
			err := common.VerifyTransaction(tx, 0, ls, nil, authRules)
			var missing common.MissingScriptWitnessesError
			require.ErrorAs(t, err, &missing)
			require.Equal(t, scriptHash, missing.ScriptHash)

			tx.WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().WithNativeScripts(
					nativeScript,
				),
			)
			require.ErrorContains(
				t,
				common.VerifyTransaction(tx, 0, ls, nil, authRules),
				"native script failed",
			)

			tx.WithWitnesses(
				mockledger.NewMockTransactionWitnessSet().
					WithNativeScripts(nativeScript).
					WithVkeyWitnesses(common.VkeyWitness{Vkey: vkey}),
			)
			require.NoError(
				t,
				common.VerifyTransaction(tx, 0, ls, nil, authRules),
			)
		})
	}
}
