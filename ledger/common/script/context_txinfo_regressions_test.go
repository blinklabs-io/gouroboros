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

package script_test

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

const (
	byronContextTestAddress   = "DdzFFzCqrht2ii4Vc7KRchSkVvQtCqdGkQt4nF4Yxg1NpsubFBity2Tpt2eSEGrxBH1eva8qCFKM2Y5QkwM1SFBizRwZgz1N452WYvgG"
	shelleyContextTestAddress = "addr_test1gqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqypnz75xxcrz9xs7v"
)

type contextTxWithType struct {
	lcommon.Transaction
	txType     int
	produced   []lcommon.Utxo
	references []lcommon.TransactionInput
	proposals  []lcommon.ProposalProcedure
}

func (t contextTxWithType) Type() int { return t.txType }

func (t contextTxWithType) Produced() []lcommon.Utxo { return t.produced }

func (t contextTxWithType) ReferenceInputs() []lcommon.TransactionInput {
	if t.references != nil {
		return t.references
	}
	return t.Transaction.ReferenceInputs()
}

func (t contextTxWithType) ProposalProcedures() []lcommon.ProposalProcedure {
	if t.proposals != nil {
		return t.proposals
	}
	return t.Transaction.ProposalProcedures()
}

func contextOutput(t *testing.T, address string, amount uint64) lcommon.TransactionOutput {
	t.Helper()
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress(address).
		WithLovelace(amount).
		Build()
	require.NoError(t, err)
	return output
}

func contextTestInput(t *testing.T, txID byte) lcommon.TransactionInput {
	t.Helper()
	input, err := mockledger.NewTransactionInputBuilder().
		WithTxId(bytes.Repeat([]byte{txID}, 32)).
		WithIndex(0).
		Build()
	require.NoError(t, err)
	return input
}

func TestTxInfoUsesTransactionBodyOutputs(t *testing.T) {
	bodyOutput := contextOutput(t, shelleyContextTestAddress, 10)
	collateralReturn := contextOutput(t, shelleyContextTestAddress, 7)
	input := contextTestInput(t, 1)
	base, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{2}, 32)).
		WithInputs(input).
		WithOutputs(bodyOutput).
		WithValid(false).
		Build()
	require.NoError(t, err)

	build := func(tx lcommon.Transaction, version int) ([]lcommon.TransactionOutput, error) {
		switch version {
		case 1:
			info, err := script.NewTxInfoV1FromTransaction(
				validitySlotState{}, tx, nil, false,
			)
			return info.Outputs, err
		case 2:
			info, err := script.NewTxInfoV2FromTransaction(
				validitySlotState{}, tx, nil, false,
			)
			return info.Outputs, err
		default:
			info, err := script.NewTxInfoV3FromTransaction(
				validitySlotState{}, tx, nil,
			)
			return info.Outputs, err
		}
	}

	for _, era := range []int{5, 6} {
		versions := []int{1, 2}
		if era == 6 {
			versions = append(versions, 3)
		}
		for _, version := range versions {
			for _, testCase := range []struct {
				name     string
				produced []lcommon.Utxo
			}{
				{
					name:     "collateral return produced",
					produced: []lcommon.Utxo{{Output: collateralReturn}},
				},
				{name: "no collateral return produced"},
			} {
				t.Run(testCase.name+"/era"+string(rune('0'+era))+"/v"+string(rune('0'+version)), func(t *testing.T) {
					tx := contextTxWithType{
						Transaction: base,
						txType:      era,
						produced:    testCase.produced,
					}
					outputs, err := build(tx, version)
					require.NoError(t, err)
					require.Len(t, outputs, 1)
					require.True(t, bodyOutput.ToPlutusData().Equal(outputs[0].ToPlutusData()))
				})
			}
		}
	}
}

func TestConwayV3ScriptContextsShareCanonicalProposalData(t *testing.T) {
	makeCredential := func(typ uint, value byte) lcommon.Credential {
		var hash lcommon.CredentialHash
		copy(hash[:], bytes.Repeat([]byte{value}, len(hash)))
		return lcommon.Credential{CredType: typ, Credential: hash}
	}
	key := makeCredential(lcommon.CredentialTypeAddrKeyHash, 1)
	scriptHigh := makeCredential(lcommon.CredentialTypeScriptHash, 2)
	scriptLow := makeCredential(lcommon.CredentialTypeScriptHash, 1)
	testAddress, err := lcommon.NewAddressFromParts(
		lcommon.AddressTypeNoneKey,
		lcommon.AddressNetworkTestnet,
		nil,
		key.Credential[:],
	)
	require.NoError(t, err)

	treasury := &conway.ConwayProposalProcedure{
		PPDeposit:       1,
		PPRewardAccount: testAddress,
		PPGovAction: conway.ConwayGovAction{
			Type: uint(lcommon.GovActionTypeTreasuryWithdrawal),
			Action: &lcommon.TreasuryWithdrawalGovAction{
				Withdrawals: map[*lcommon.Address]uint64{
					addressFromCredential(t, lcommon.AddressNetworkTestnet, lcommon.AddressTypeNoneKey, key):           10,
					addressFromCredential(t, lcommon.AddressNetworkTestnet, lcommon.AddressTypeNoneScript, scriptHigh): 20,
					addressFromCredential(t, lcommon.AddressNetworkTestnet, lcommon.AddressTypeNoneScript, scriptLow):  30,
				},
			},
		},
	}
	committee := &conway.ConwayProposalProcedure{
		PPDeposit:       2,
		PPRewardAccount: testAddress,
		PPGovAction: conway.ConwayGovAction{
			Type: uint(lcommon.GovActionTypeUpdateCommittee),
			Action: &lcommon.UpdateCommitteeGovAction{
				Credentials: []lcommon.Credential{key, scriptHigh, scriptLow},
				CredEpochs: map[*lcommon.Credential]uint64{
					&key:        10,
					&scriptHigh: 20,
					&scriptLow:  30,
				},
				Quorum: cbor.Rat{Rat: big.NewRat(1, 2)},
			},
		},
	}
	input := contextTestInput(t, 1)
	base, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{2}, 32)).
		WithInputs(input).
		WithOutputs(contextOutput(t, shelleyContextTestAddress, 10)).
		Build()
	require.NoError(t, err)
	tx := contextTxWithType{
		Transaction: base,
		proposals:   []lcommon.ProposalProcedure{treasury, committee},
	}
	info, err := script.NewTxInfoV3FromTransaction(
		validitySlotState{},
		tx,
		[]lcommon.Utxo{{Id: input, Output: contextOutput(t, shelleyContextTestAddress, 5)}},
	)
	require.NoError(t, err)
	purposes := []script.ScriptPurpose{
		script.ScriptPurposeMinting{},
		script.ScriptPurposeSpending{},
		script.ScriptPurposeRewarding{},
		script.ScriptPurposeCertifying{},
		script.ScriptPurposeVoting{},
		script.ScriptPurposeProposing{
			ProposalProcedure: treasury,
		},
	}
	var proposalData data.PlutusData
	for idx, purpose := range purposes {
		ctx := script.NewScriptContextV3(info, script.Redeemer{}, purpose).(script.ScriptContextV3)
		txInfo := ctx.TxInfo.ToPlutusData().(*data.Constr)
		if idx == 0 {
			proposalData = txInfo.Fields[14]
		} else {
			require.True(t, proposalData.Equal(txInfo.Fields[14]))
		}
	}
}

func addressFromCredential(
	t *testing.T,
	network uint8,
	addressType uint8,
	credential lcommon.Credential,
) *lcommon.Address {
	t.Helper()
	address, err := lcommon.NewAddressFromParts(
		addressType,
		network,
		nil,
		credential.Credential[:],
	)
	require.NoError(t, err)
	return &address
}

func TestAlonzoV1FiltersByronInputsAndOutputs(t *testing.T) {
	byronOutput := contextOutput(t, byronContextTestAddress, 10)
	shelleyOutput := contextOutput(t, shelleyContextTestAddress, 20)
	input := contextTestInput(t, 1)
	base, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{2}, 32)).
		WithInputs(input).
		WithOutputs(byronOutput, shelleyOutput).
		Build()
	require.NoError(t, err)
	tx := contextTxWithType{Transaction: base, txType: 4}

	info, err := script.NewTxInfoV1FromTransaction(
		validitySlotState{},
		tx,
		[]lcommon.Utxo{{Id: input, Output: byronOutput}},
		false,
	)
	require.NoError(t, err)
	require.Empty(t, info.Inputs)
	require.Len(t, info.Outputs, 1)
	require.True(t, shelleyOutput.ToPlutusData().Equal(info.Outputs[0].ToPlutusData()))
}

func TestBabbageAndLaterRejectByronOutputsInScriptContext(t *testing.T) {
	byronOutput := contextOutput(t, byronContextTestAddress, 10)
	input := contextTestInput(t, 1)
	base, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{2}, 32)).
		WithInputs(input).
		WithOutputs(byronOutput).
		Build()
	require.NoError(t, err)
	resolved := []lcommon.Utxo{}

	testCases := []struct {
		name  string
		era   int
		build func(lcommon.Transaction) error
	}{
		{
			name: "Babbage V1",
			era:  5,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, false)
				return err
			},
		},
		{
			name: "Babbage V2",
			era:  5,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, false)
				return err
			},
		},
		{
			name: "Conway V1",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, true)
				return err
			},
		},
		{
			name: "Conway V2",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, true)
				return err
			},
		},
		{
			name: "Conway V1",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, true)
				return err
			},
		},
		{
			name: "Conway V2",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, true)
				return err
			},
		},
		{
			name: "Conway V3",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV3FromTransaction(validitySlotState{}, tx, resolved)
				return err
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tx := contextTxWithType{Transaction: base, txType: testCase.era}
			err := testCase.build(tx)
			require.ErrorContains(t, err, "ByronTxOutInContext: Byron output in output")
		})
	}
}

func TestTxInfoSpendingInputsRejectByronOutputsInBabbageAndLater(t *testing.T) {
	byronOutput := contextOutput(t, byronContextTestAddress, 10)
	shelleyOutput := contextOutput(t, shelleyContextTestAddress, 20)
	input := contextTestInput(t, 1)
	base, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{2}, 32)).
		WithInputs(input).
		WithOutputs(shelleyOutput).
		Build()
	require.NoError(t, err)
	resolved := []lcommon.Utxo{{Id: input, Output: byronOutput}}
	for _, testCase := range []struct {
		name  string
		era   int
		build func(lcommon.Transaction) error
	}{
		{
			name: "Babbage V1",
			era:  5,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV1FromTransaction(validitySlotState{}, tx, resolved, false)
				return err
			},
		},
		{
			name: "Babbage V2",
			era:  5,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, false)
				return err
			},
		},
		{
			name: "Conway V3",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV3FromTransaction(validitySlotState{}, tx, resolved)
				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tx := contextTxWithType{Transaction: base, txType: testCase.era}
			err := testCase.build(tx)
			require.ErrorContains(t, err, "ByronTxOutInContext: Byron output in input")
		})
	}
}

func TestTxInfoReferenceInputsRejectByronOutputsInBabbageAndLater(t *testing.T) {
	byronOutput := contextOutput(t, byronContextTestAddress, 10)
	shelleyOutput := contextOutput(t, shelleyContextTestAddress, 20)
	input := contextTestInput(t, 1)
	referenceInput := contextTestInput(t, 3)
	base, err := mockledger.NewTransactionBuilder().
		WithId(bytes.Repeat([]byte{2}, 32)).
		WithInputs(input).
		WithOutputs(shelleyOutput).
		Build()
	require.NoError(t, err)
	resolved := []lcommon.Utxo{
		{Id: input, Output: shelleyOutput},
		{Id: referenceInput, Output: byronOutput},
	}
	for _, testCase := range []struct {
		name  string
		era   int
		build func(lcommon.Transaction) error
	}{
		{
			name: "Babbage V2",
			era:  5,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV2FromTransaction(validitySlotState{}, tx, resolved, false)
				return err
			},
		},
		{
			name: "Conway V3",
			era:  6,
			build: func(tx lcommon.Transaction) error {
				_, err := script.NewTxInfoV3FromTransaction(validitySlotState{}, tx, resolved)
				return err
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			tx := contextTxWithType{
				Transaction: base,
				txType:      testCase.era,
				references:  []lcommon.TransactionInput{referenceInput},
			}
			err := testCase.build(tx)
			require.ErrorContains(t, err, "ByronTxOutInContext: Byron output in reference input")
		})
	}
}
