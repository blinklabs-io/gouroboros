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
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func guardrailsState(scriptHash []byte) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithConstitutionValue(&common.Constitution{
			ScriptHash: bytes.Clone(scriptHash),
		}).
		Build()
}

func validGuardrailsProposalTx(
	deposit uint64,
	rewardAccount common.Address,
	action common.GovAction,
) *conway.ConwayTransaction {
	tx := mkProposalTx(deposit, rewardAccount, action)
	tx.TxIsValid = true
	return tx
}

func guardrailsParameterChange(policyHash []byte) common.GovAction {
	minFeeA := uint(44)
	return &conway.ConwayParameterChangeGovAction{
		Type: uint(common.GovActionTypeParameterChange),
		ParamUpdate: conway.ConwayProtocolParameterUpdate{
			MinFeeA: &minFeeA,
		},
		PolicyHash: bytes.Clone(policyHash),
	}
}

func guardrailsTreasuryWithdrawal(
	t *testing.T,
	policyHash []byte,
) common.GovAction {
	t.Helper()
	rewardAddress := makeConwayRewardAddress(
		t,
		common.Blake2b224Hash([]byte("guardrails-withdrawal")),
	)
	return &common.TreasuryWithdrawalGovAction{
		Type: uint(common.GovActionTypeTreasuryWithdrawal),
		Withdrawals: map[*common.Address]uint64{
			&rewardAddress: 1,
		},
		PolicyHash: bytes.Clone(policyHash),
	}
}

func TestUtxoValidateGuardrailsScriptHash(t *testing.T) {
	guardrailsHash := common.Blake2b224Hash([]byte("constitution-guardrails"))
	differentHash := common.Blake2b224Hash([]byte("different-guardrails"))

	for _, actionFactory := range []struct {
		name string
		new  func(*testing.T, []byte) common.GovAction
	}{
		{
			name: "parameter change",
			new: func(_ *testing.T, hash []byte) common.GovAction {
				return guardrailsParameterChange(hash)
			},
		},
		{
			name: "treasury withdrawal",
			new:  guardrailsTreasuryWithdrawal,
		},
	} {
		t.Run(actionFactory.name, func(t *testing.T) {
			tests := []struct {
				name         string
				constitution []byte
				policy       []byte
				wantErr      bool
				wantActual   []byte
				wantExpected []byte
			}{
				{
					name:         "matching hashes",
					constitution: guardrailsHash.Bytes(),
					policy:       guardrailsHash.Bytes(),
				},
				{name: "both absent"},
				{
					name:         "proposal absent",
					constitution: guardrailsHash.Bytes(),
					wantErr:      true,
					wantExpected: guardrailsHash.Bytes(),
				},
				{
					name:       "constitution absent",
					policy:     guardrailsHash.Bytes(),
					wantErr:    true,
					wantActual: guardrailsHash.Bytes(),
				},
				{
					name:         "different hashes",
					constitution: guardrailsHash.Bytes(),
					policy:       differentHash.Bytes(),
					wantErr:      true,
					wantActual:   differentHash.Bytes(),
					wantExpected: guardrailsHash.Bytes(),
				},
			}

			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					tx := validGuardrailsProposalTx(
						0,
						common.Address{},
						actionFactory.new(t, tc.policy),
					)
					err := conway.UtxoValidateGovActionWellFormedness(
						tx,
						0,
						guardrailsState(tc.constitution),
						nil,
					)
					if !tc.wantErr {
						require.NoError(t, err)
						return
					}
					var target conway.InvalidGuardrailsScriptHashError
					require.ErrorAs(t, err, &target)
					require.Equal(t, tc.wantActual, target.Actual)
					require.Equal(t, tc.wantExpected, target.Expected)
				})
			}
		})
	}
}

func TestUtxoValidateGuardrailsScriptHashStateFailures(t *testing.T) {
	tx := validGuardrailsProposalTx(
		0,
		common.Address{},
		guardrailsParameterChange(nil),
	)

	t.Run("nil ledger state", func(t *testing.T) {
		err := conway.UtxoValidateGuardrailsScriptHash(tx, 0, nil, nil)
		var target conway.ConstitutionLookupError
		require.ErrorAs(t, err, &target)
	})

	t.Run("lookup error", func(t *testing.T) {
		lookupErr := errors.New("constitution unavailable")
		state := mockledger.NewLedgerStateBuilder().
			WithConstitution(func() (*common.Constitution, error) {
				return nil, lookupErr
			}).
			Build()
		err := conway.UtxoValidateGuardrailsScriptHash(tx, 0, state, nil)
		var target conway.ConstitutionLookupError
		require.ErrorAs(t, err, &target)
		require.ErrorIs(t, err, lookupErr)
	})

	t.Run("malformed constitution", func(t *testing.T) {
		err := conway.UtxoValidateGuardrailsScriptHash(
			tx,
			0,
			guardrailsState([]byte{}),
			nil,
		)
		var target conway.MalformedConstitutionError
		require.ErrorAs(t, err, &target)
		require.Zero(t, target.ScriptHashLength)
	})

	t.Run(
		"unrelated action does not need constitution state",
		func(t *testing.T) {
			infoTx := validGuardrailsProposalTx(
				0,
				common.Address{},
				&common.InfoGovAction{},
			)
			require.NoError(t, conway.UtxoValidateGuardrailsScriptHash(
				infoTx,
				0,
				nil,
				nil,
			))
		},
	)
}

func TestUtxoValidateGuardrailsRejectsExplicitEmptyPolicy(t *testing.T) {
	for _, action := range []common.GovAction{
		guardrailsParameterChange([]byte{}),
		guardrailsTreasuryWithdrawal(t, []byte{}),
	} {
		tx := validGuardrailsProposalTx(0, common.Address{}, action)
		err := conway.UtxoValidateGovActionWellFormedness(
			tx,
			0,
			guardrailsState(nil),
			nil,
		)
		var target conway.MalformedGovActionError
		require.ErrorAs(t, err, &target)
	}
}

func TestConwayValidationRulesEnforceGuardrailsOnDecodedTransaction(
	t *testing.T,
) {
	guardrailsHash := common.Blake2b224Hash([]byte("constitution-guardrails"))
	state := guardrailsState(guardrailsHash.Bytes())

	decodeTx := func(
		t *testing.T,
		isValid bool,
		policyHash []byte,
	) *conway.ConwayTransaction {
		t.Helper()
		rewardAddress := makeConwayRewardAddress(
			t,
			common.Blake2b224Hash([]byte("guardrails-return-account")),
		)
		tx := mkProposalTx(
			0,
			rewardAddress,
			guardrailsParameterChange(policyHash),
		)
		tx.TxIsValid = isValid
		encoded, err := cbor.Encode(tx)
		require.NoError(t, err)
		decoded, err := conway.NewConwayTransactionFromCbor(encoded)
		require.NoError(t, err)
		return decoded
	}

	// Locate the registered rule by its valid-transaction behavior. This keeps
	// the regression attached to the production rule list without depending on
	// a rule index or on whether phase-valid rules have been composed.
	validMismatch := decodeTx(t, true, nil)
	var guardrailsRule common.UtxoValidationRuleFunc
	for _, rule := range conway.UtxoValidationRules {
		err := rule(
			validMismatch,
			0,
			state,
			&conway.ConwayProtocolParameters{},
		)
		var target conway.InvalidGuardrailsScriptHashError
		if errors.As(err, &target) {
			guardrailsRule = rule
			break
		}
	}
	require.NotNil(t, guardrailsRule, "guardrails validation rule is not registered")
	guardrailsRules := []common.UtxoValidationRuleFunc{guardrailsRule}

	require.NoError(t, common.VerifyTransaction(
		decodeTx(t, true, guardrailsHash.Bytes()),
		0,
		state,
		&conway.ConwayProtocolParameters{},
		guardrailsRules,
	))

	err := common.VerifyTransaction(
		validMismatch,
		0,
		state,
		&conway.ConwayProtocolParameters{},
		guardrailsRules,
	)
	var target conway.InvalidGuardrailsScriptHashError
	require.ErrorAs(t, err, &target)

	require.NoError(t, common.VerifyTransaction(
		decodeTx(t, false, nil),
		0,
		state,
		&conway.ConwayProtocolParameters{},
		guardrailsRules,
	))
}

func TestGuardrailsPolicyRequiresScriptAuthorization(t *testing.T) {
	nativeScriptBytes, err := cbor.Encode(&common.NativeScriptAll{
		Type:    1,
		Scripts: []common.NativeScript{},
	})
	require.NoError(t, err)
	var nativeScript common.NativeScript
	_, err = cbor.Decode(nativeScriptBytes, &nativeScript)
	require.NoError(t, err)
	policyHash := nativeScript.Hash()

	tx := mkProposalTx(
		0,
		common.Address{},
		guardrailsParameterChange(policyHash[:]),
	)
	tx.TxIsValid = true
	state := guardrailsState(policyHash[:])
	require.NoError(t, conway.UtxoValidateGovActionWellFormedness(
		tx,
		0,
		state,
		nil,
	))

	err = conway.UtxoValidateScriptWitnesses(tx, 0, state, nil)
	var missing common.MissingScriptWitnessesError
	require.ErrorAs(t, err, &missing)
	require.Equal(t, policyHash, missing.ScriptHash)

	tx.WitnessSet.WsNativeScripts = cbor.NewSetType(
		[]common.NativeScript{nativeScript},
		false,
	)
	require.NoError(t, conway.UtxoValidateScriptWitnesses(tx, 0, state, nil))
	require.NoError(t, conway.UtxoValidateNativeScripts(tx, 0, state, nil))
}
