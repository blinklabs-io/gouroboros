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

package dijkstra

import (
	"fmt"
	"math/big"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	commontestdata "github.com/blinklabs-io/gouroboros/ledger/common/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func dijkstraValidationRuleName(rule common.UtxoValidationRuleFunc) string {
	return runtime.FuncForPC(reflect.ValueOf(rule).Pointer()).Name()
}

func dijkstraValidationRule(
	t *testing.T,
	want string,
) (common.UtxoValidationRuleFunc, int) {
	t.Helper()
	for idx, rule := range UtxoValidationRules {
		if strings.HasSuffix(dijkstraValidationRuleName(rule), want) {
			return rule, idx
		}
	}
	t.Fatalf("validation rule %s is not registered", want)
	return nil, -1
}

func TestDijkstraGovernanceValidationRules(t *testing.T) {
	expected := []string{
		"ledger/dijkstra.UtxoValidateProposalProcedures",
		"ledger/conway.UtxoValidateGovActionWellFormedness",
		"ledger/dijkstra.UtxoValidateHardForkCanFollow",
		"ledger/conway.UtxoValidateProposalAncestry",
		"ledger/dijkstra.UtxoValidateProposalDeposit",
		"ledger/conway.UtxoValidateProposalNetworkIds",
		"ledger/conway.UtxoValidateProposalReturnAccounts",
		"ledger/conway.UtxoValidateEmptyTreasuryWithdrawals",
		"ledger/conway.UtxoValidateCommitteeCertificates",
		"ledger/conway.UtxoValidateUnknownVoters",
		"ledger/conway.UtxoValidateUnknownGovActionIds",
		"ledger/conway.UtxoValidateVotingOnExpiredGovAction",
		"ledger/dijkstra.UtxoValidateBootstrapVotingRestrictions",
		"ledger/conway.UtxoValidateStakePoolVotingRestrictions",
		"ledger/dijkstra.UtxoValidateCCVotingRestrictions",
	}

	previous := -1
	for _, ruleName := range expected {
		_, idx := dijkstraValidationRule(t, ruleName)
		require.Greater(
			t,
			idx,
			previous,
			"validation rule %s is out of order",
			ruleName,
		)
		previous = idx
	}
}

func TestConwayGovActionRejectsDijkstraParameterChange(t *testing.T) {
	_, err := conway.NewConwayGovAction(&DijkstraParameterChangeGovAction{})
	require.Error(t, err)
}

func TestDijkstraGovernanceValidationRejectsTypedNilParameterChange(
	t *testing.T,
) {
	var action *DijkstraParameterChangeGovAction
	tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
		TxProposalProcedures: []DijkstraProposalProcedure{{
			PPGovAction: DijkstraGovAction{Action: action},
		}},
	}}
	var err error
	require.NotPanics(t, func() {
		err = common.VerifyTransaction(
			tx,
			0,
			nil,
			&DijkstraProtocolParameters{},
			UtxoValidationRules,
		)
	})
	var malformedErr conway.MalformedGovActionError
	require.ErrorAs(t, err, &malformedErr)
}

func TestDijkstraBootstrapVotingRestrictionsAreRegistered(t *testing.T) {
	newTx := func(action common.GovAction) *DijkstraTransaction {
		tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPGovAction: DijkstraGovAction{Action: action},
			}},
		}}
		encodedBody, err := cbor.Encode(&tx.Body)
		require.NoError(t, err)
		tx.Body.SetCborReference(encodedBody)
		actionId := common.GovActionId{TransactionId: tx.Hash()}
		voter := common.Voter{
			Type: common.VoterTypeDRepKeyHash,
			Hash: common.Blake2b224{0x04},
		}
		tx.Body.TxVotingProcedures = common.VotingProcedures{
			&voter: {
				&actionId: {Vote: common.GovVoteYes},
			},
		}
		return tx
	}
	pp := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionConway,
			},
		},
	}

	rule, _ := dijkstraValidationRule(
		t,
		"ledger/dijkstra.UtxoValidateBootstrapVotingRestrictions",
	)
	err := rule(newTx(&DijkstraParameterChangeGovAction{}), 0, nil, pp)
	var bootstrapErr conway.BootstrapVotingRestrictionError
	require.ErrorAs(t, err, &bootstrapErr)
	require.NoError(t, rule(newTx(&common.InfoGovAction{}), 0, nil, pp))
}

func TestDijkstraParameterChangeSecurityGroupFields(t *testing.T) {
	maxRefScriptSizePerBlock := uint32(1)
	maxRefScriptSizePerTx := uint32(2)
	refScriptCostStride := uint32(3)
	action := DijkstraParameterChangeGovAction{
		ParamUpdate: DijkstraProtocolParameterUpdate{
			MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
			MaxRefScriptSizePerTx:    &maxRefScriptSizePerTx,
			RefScriptCostStride:      &refScriptCostStride,
			RefScriptCostMultiplier:  new(cbor.Rat),
		},
	}
	require.Equal(t, []string{
		"MaxRefScriptSizePerBlock",
		"MaxRefScriptSizePerTx",
		"RefScriptCostStride",
		"RefScriptCostMultiplier",
	}, action.SecurityGroupFields())
}

func TestDijkstraGovernanceValidationRulesRejectInvalidProposalsAndVotes(
	t *testing.T,
) {
	pp := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
			GovActionDeposit: 500,
		},
	}

	t.Run("proposal deposit", func(t *testing.T) {
		tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPDeposit:   1,
				PPGovAction: DijkstraGovAction{Action: &common.InfoGovAction{}},
			}},
		}}
		rule, _ := dijkstraValidationRule(
			t,
			"ledger/dijkstra.UtxoValidateProposalDeposit",
		)
		err := rule(tx, 0, nil, pp)
		var depositErr conway.ProposalDepositIncorrectError
		require.ErrorAs(t, err, &depositErr)
	})

	t.Run("parameter-change ancestry", func(t *testing.T) {
		missing := common.GovActionId{TransactionId: common.Blake2b256{0x01}}
		tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPGovAction: DijkstraGovAction{
					Action: &DijkstraParameterChangeGovAction{
						ActionId: &missing,
					},
				},
			}},
		}}
		ls := mockledger.NewLedgerStateBuilder().Build()
		rule, _ := dijkstraValidationRule(
			t,
			"ledger/conway.UtxoValidateProposalAncestry",
		)
		err := rule(tx, 0, ls, pp)
		var ancestryErr conway.InvalidGovActionAncestorError
		require.ErrorAs(t, err, &ancestryErr)
	})

	t.Run("stake-pool parameter-change vote", func(t *testing.T) {
		keyDeposit := uint(2_000_000)
		tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPGovAction: DijkstraGovAction{
					Action: &DijkstraParameterChangeGovAction{
						ParamUpdate: DijkstraProtocolParameterUpdate{
							KeyDeposit: &keyDeposit,
						},
					},
				},
			}},
		}}
		encodedBody, err := cbor.Encode(&tx.Body)
		require.NoError(t, err)
		tx.Body.SetCborReference(encodedBody)
		actionId := common.GovActionId{TransactionId: tx.Hash()}
		voter := common.Voter{
			Type: common.VoterTypeStakingPoolKeyHash,
			Hash: common.Blake2b224{0x02},
		}
		tx.Body.TxVotingProcedures = common.VotingProcedures{
			&voter: {
				&actionId: {Vote: common.GovVoteYes},
			},
		}
		ls := mockledger.NewLedgerStateBuilder().Build()
		rule, _ := dijkstraValidationRule(
			t,
			"ledger/conway.UtxoValidateStakePoolVotingRestrictions",
		)
		err = rule(tx, 0, ls, pp)
		var votingErr conway.StakePoolVotingRestrictionError
		require.ErrorAs(t, err, &votingErr)
	})

	t.Run("stake-pool Dijkstra security parameter vote", func(t *testing.T) {
		maxRefScriptSizePerTx := uint32(200_000)
		tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{{
				PPGovAction: DijkstraGovAction{
					Action: &DijkstraParameterChangeGovAction{
						ParamUpdate: DijkstraProtocolParameterUpdate{
							MaxRefScriptSizePerTx: &maxRefScriptSizePerTx,
						},
					},
				},
			}},
		}}
		encodedBody, err := cbor.Encode(&tx.Body)
		require.NoError(t, err)
		tx.Body.SetCborReference(encodedBody)
		actionId := common.GovActionId{TransactionId: tx.Hash()}
		voter := common.Voter{
			Type: common.VoterTypeStakingPoolKeyHash,
			Hash: common.Blake2b224{0x03},
		}
		tx.Body.TxVotingProcedures = common.VotingProcedures{
			&voter: {
				&actionId: {Vote: common.GovVoteYes},
			},
		}
		ls := mockledger.NewLedgerStateBuilder().Build()
		rule, _ := dijkstraValidationRule(
			t,
			"ledger/conway.UtxoValidateStakePoolVotingRestrictions",
		)
		require.NoError(t, rule(tx, 0, ls, pp))
	})
}

func TestUtxoValidateBootstrapAllowedGovActionsRejectsUnknown(t *testing.T) {
	tx := &DijkstraTransaction{}
	tx.Body.TxProposalProcedures = []DijkstraProposalProcedure{{
		PPGovAction: DijkstraGovAction{
			Action: commontestdata.UnsupportedGovAction{},
		},
	}}
	pp := &DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = common.ProtocolVersionConway
	require.Error(t, UtxoValidateBootstrapAllowedGovActions(tx, 0, nil, pp))
}

func testGuardCredential() common.Credential {
	var guardHash common.Blake2b224
	for i := range guardHash {
		guardHash[i] = byte(i + 1)
	}
	return common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: guardHash,
	}
}

func testRequireGuardNativeScript(
	t *testing.T,
	credential common.Credential,
) common.NativeScript {
	t.Helper()
	scriptCbor, err := cbor.Encode(common.NativeScriptRequireGuard{
		Type:       6,
		Credential: credential,
	})
	require.NoError(t, err)
	var script common.NativeScript
	require.NoError(t, script.UnmarshalCBOR(scriptCbor))
	return script
}

func testDijkstraWitnessSet(
	t *testing.T,
	script common.Script,
) DijkstraTransactionWitnessSet {
	t.Helper()
	var witnesses DijkstraTransactionWitnessSet
	if script != nil {
		switch script := script.(type) {
		case common.NativeScript:
			witnesses.WsNativeScripts = cbor.NewSetType(
				[]common.NativeScript{script},
				false,
			)
		case common.PlutusV1Script:
			witnesses.WsPlutusV1Scripts = cbor.NewSetType(
				[]common.PlutusV1Script{script},
				false,
			)
		case common.PlutusV2Script:
			witnesses.WsPlutusV2Scripts = cbor.NewSetType(
				[]common.PlutusV2Script{script},
				false,
			)
		case common.PlutusV3Script:
			witnesses.WsPlutusV3Scripts = cbor.NewSetType(
				[]common.PlutusV3Script{script},
				false,
			)
		case common.PlutusV4Script:
			witnesses.WsPlutusV4Scripts = cbor.NewSetType(
				[]common.PlutusV4Script{script},
				false,
			)
		default:
			t.Fatalf("unsupported withdrawal script type %T", script)
		}
	}
	return witnesses
}

func testDijkstraWithdrawalTx(
	t *testing.T,
	withdrawal uint64,
	withdrawalScript common.Script,
) (*DijkstraTransaction, common.Credential) {
	t.Helper()
	credentialHash := common.Blake2b224Hash([]byte("withdrawal-stake-key"))
	addressType := uint8(common.AddressTypeNoneKey)
	if withdrawalScript != nil {
		credentialHash = withdrawalScript.Hash()
		addressType = common.AddressTypeNoneScript
	}
	rewardAddr, err := common.NewAddressFromParts(
		addressType,
		common.AddressNetworkTestnet,
		nil,
		credentialHash.Bytes(),
	)
	require.NoError(t, err)
	credential, ok := rewardAddr.StakeCredential()
	require.True(t, ok)
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxWithdrawals: map[*common.Address]uint64{
				&rewardAddr: withdrawal,
			},
		},
		WitnessSet: testDijkstraWitnessSet(t, withdrawalScript),
		TxIsValid:  true,
	}, credential
}

func TestUtxoValidateWithdrawalsDijkstraAmountModes(t *testing.T) {
	const balance = uint64(1_000_000)
	pp := &DijkstraProtocolParameters{}
	pp.ProtocolVersion.Major = common.ProtocolVersionDijkstra

	for _, tc := range []struct {
		name                 string
		withdrawalScript     common.Script
		scriptInSubTx        bool
		unrelatedSubTxScript common.Script
		partialAllowed       bool
	}{
		{name: "key withdrawal", partialAllowed: true},
		{
			name:             "native script withdrawal",
			withdrawalScript: testRequireGuardNativeScript(t, testGuardCredential()),
			partialAllowed:   true,
		},
		{
			name:             "Plutus V1 withdrawal",
			withdrawalScript: common.PlutusV1Script{0x41, 0x00},
		},
		{
			name:             "Plutus V2 withdrawal",
			withdrawalScript: common.PlutusV2Script{0x41, 0x00},
		},
		{
			name:             "Plutus V3 withdrawal",
			withdrawalScript: common.PlutusV3Script{0x41, 0x00},
		},
		{
			name:             "Plutus V4 withdrawal",
			withdrawalScript: common.PlutusV4Script{0x41, 0x00},
			partialAllowed:   true,
		},
		{
			name:             "Plutus V1 supplied by sub-transaction",
			withdrawalScript: common.PlutusV1Script{0x41, 0x00},
			scriptInSubTx:    true,
		},
		{
			name:             "Plutus V4 supplied by sub-transaction",
			withdrawalScript: common.PlutusV4Script{0x41, 0x00},
			scriptInSubTx:    true,
			partialAllowed:   true,
		},
		{
			name:                 "unrelated Plutus V1 in sub-transaction",
			unrelatedSubTxScript: common.PlutusV1Script{0x41, 0x00},
			partialAllowed:       true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, credential := testDijkstraWithdrawalTx(
				t,
				balance/2,
				tc.withdrawalScript,
			)
			if tc.scriptInSubTx {
				subTxWitnesses := tx.WitnessSet
				tx.WitnessSet = DijkstraTransactionWitnessSet{}
				tx.Body.TxSubTransactions = cbor.NewSetType(
					[]DijkstraSubTransaction{{
						WitnessSet: subTxWitnesses,
					}},
					false,
				)
			} else if tc.unrelatedSubTxScript != nil {
				tx.Body.TxSubTransactions = cbor.NewSetType(
					[]DijkstraSubTransaction{{
						WitnessSet: testDijkstraWitnessSet(
							t,
							tc.unrelatedSubTxScript,
						),
					}},
					false,
				)
			}
			ls := mockledger.NewLedgerStateBuilder().
				WithRewardAccountCredentialBalance(credential, balance).
				Build()

			err := conway.UtxoValidateWithdrawals(tx, 0, ls, pp)
			if tc.partialAllowed {
				require.NoError(t, err)
			} else {
				var target shelley.IncorrectWithdrawalAmountError
				require.ErrorAs(t, err, &target)
			}

			for rewardAddr := range tx.Body.TxWithdrawals {
				tx.Body.TxWithdrawals[rewardAddr] = balance + 1
			}
			var target shelley.IncorrectWithdrawalAmountError
			require.ErrorAs(
				t,
				conway.UtxoValidateWithdrawals(tx, 0, ls, pp),
				&target,
			)

			for rewardAddr := range tx.Body.TxWithdrawals {
				tx.Body.TxWithdrawals[rewardAddr] = balance
			}
			require.NoError(t, conway.UtxoValidateWithdrawals(tx, 0, ls, pp))
		})
	}
}

func testGuardScriptCredential(script common.PlutusV4Script) common.Credential {
	return common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: script.Hash(),
	}
}

func TestUtxoValidateNativeScriptsRequireGuard(t *testing.T) {
	guardCred := testGuardCredential()
	nativeScript := testRequireGuardNativeScript(t, guardCred)

	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guardCred},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsNativeScripts: cbor.NewSetType(
				[]common.NativeScript{nativeScript},
				false,
			),
		},
		TxIsValid: true,
	}
	require.NoError(t, UtxoValidateNativeScripts(tx, 0, nil, nil))

	tx.Body.TxGuards = nil
	require.Error(t, UtxoValidateNativeScripts(tx, 0, nil, nil))
}

func TestUtxoValidateGuardingRedeemerRejectsNativeScriptGuard(t *testing.T) {
	guardCred := testGuardCredential()
	nativeScript := testRequireGuardNativeScript(t, guardCred)
	nativeScriptCred := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: nativeScript.Hash(),
	}
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{
					nativeScriptCred,
					guardCred,
				},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsNativeScripts: cbor.NewSetType(
				[]common.NativeScript{nativeScript},
				false,
			),
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}

	err := UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
	require.NoError(t, err)

	err = UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
	require.ErrorAs(t, err, &conway.ExtraRedeemerError{})

	err = UtxoValidatePlutusScripts(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		&DijkstraProtocolParameters{},
	)
	require.ErrorAs(t, err, &conway.ExtraRedeemerError{})
}

// The witness case above pins the guard's native script arriving through the
// witness set. A native script arriving as a *reference* script on a
// resolved input exercises a different path -- script.AvailablePlutusScripts
// filters it out by PlutusScriptVersion, not by TransactionWitnessSet.
// NativeScripts -- so it needs its own rule-level case rather than assuming
// the witness test covers it too.
func TestUtxoValidateGuardingRedeemerRejectsNativeReferenceScriptGuard(
	t *testing.T,
) {
	guardCred := testGuardCredential()
	nativeScript := testRequireGuardNativeScript(t, guardCred)
	nativeScriptCred := common.Credential{
		CredType:   common.CredentialTypeScriptHash,
		Credential: nativeScript.Hash(),
	}
	refInput := shelley.NewShelleyTransactionInput(
		"4444444444444444444444444444444444444444444444444444444444444444",
		0,
	)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{
					nativeScriptCred,
					guardCred,
				},
			},
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{refInput},
				false,
			),
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}
	refOutput := babbage.BabbageTransactionOutput{
		TxOutScriptRef: &common.ScriptRef{
			Type:   common.ScriptRefTypeNativeScript,
			Script: nativeScript,
		},
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{Id: input, Output: &refOutput}, nil
		}).
		Build()

	err := UtxoValidatePlutusScripts(
		tx,
		0,
		ls,
		&DijkstraProtocolParameters{},
	)
	require.ErrorAs(t, err, &conway.ExtraRedeemerError{})
}

func TestUtxoValidateCostModelsPresentPlutusV4(t *testing.T) {
	tx := &DijkstraTransaction{
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusV4Scripts: cbor.NewSetType(
				[]common.PlutusV4Script{{0x41, 0x00}},
				false,
			),
		},
		TxIsValid: true,
	}

	err := UtxoValidateCostModelsPresent(
		tx,
		0,
		nil,
		&DijkstraProtocolParameters{
			ConwayProtocolParameters: conway.ConwayProtocolParameters{
				CostModels: map[uint][]int64{3: {1}},
			},
		},
	)
	require.NoError(t, err)

	err = UtxoValidateCostModelsPresent(
		tx,
		0,
		nil,
		&DijkstraProtocolParameters{},
	)
	require.ErrorAs(t, err, &common.MissingCostModelError{})
}

func TestUtxoValidateCostModelsPresentSubTransactionPlutus(t *testing.T) {
	cases := []struct {
		name       string
		witnessSet DijkstraTransactionWitnessSet
		version    uint
	}{
		{
			name: "plutus v1",
			witnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV1Scripts: cbor.NewSetType(
					[]common.PlutusV1Script{{0x41, 0x00}},
					false,
				),
			},
			version: 0,
		},
		{
			name: "plutus v2",
			witnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV2Scripts: cbor.NewSetType(
					[]common.PlutusV2Script{{0x41, 0x00}},
					false,
				),
			},
			version: 1,
		},
		{
			name: "plutus v3",
			witnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV3Scripts: cbor.NewSetType(
					[]common.PlutusV3Script{{0x41, 0x00}},
					false,
				),
			},
			version: 2,
		},
		{
			name: "plutus v4",
			witnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV4Scripts: cbor.NewSetType(
					[]common.PlutusV4Script{{0x41, 0x00}},
					false,
				),
			},
			version: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx := &DijkstraTransaction{
				Body: DijkstraTransactionBody{
					TxSubTransactions: cbor.NewSetType(
						[]DijkstraSubTransaction{
							{WitnessSet: tc.witnessSet},
						},
						false,
					),
				},
				TxIsValid: true,
			}

			err := UtxoValidateCostModelsPresent(
				tx,
				0,
				nil,
				&DijkstraProtocolParameters{},
			)
			var missing common.MissingCostModelError
			require.ErrorAs(t, err, &missing)
			require.Equal(t, tc.version, missing.Version)

			err = UtxoValidateCostModelsPresent(
				tx,
				0,
				nil,
				&DijkstraProtocolParameters{
					ConwayProtocolParameters: conway.ConwayProtocolParameters{
						CostModels: map[uint][]int64{tc.version: {1}},
					},
				},
			)
			require.NoError(t, err)
		})
	}
}

func TestUtxoValidateProposalProceduresDijkstraProtocolParameterUpdate(
	t *testing.T,
) {
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{
				{
					PPGovAction: DijkstraGovAction{
						Action: &DijkstraParameterChangeGovAction{
							ParamUpdate: DijkstraProtocolParameterUpdate{},
						},
					},
				},
			},
		},
	}
	err := UtxoValidateProposalProcedures(tx, 0, nil, nil)
	require.ErrorAs(t, err, &conway.ProtocolParameterUpdateEmptyError{})

	maxRefScriptSizePerBlock := uint32(1000)
	tx.Body.TxProposalProcedures[0].PPGovAction.Action =
		&DijkstraParameterChangeGovAction{
			ParamUpdate: DijkstraProtocolParameterUpdate{
				MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
			},
		}
	require.NoError(t, UtxoValidateProposalProcedures(tx, 0, nil, nil))
}

func TestUtxoValidateBootstrapParameterGroupsDijkstraFields(t *testing.T) {
	refScriptCostStride := uint32(25600)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxProposalProcedures: []DijkstraProposalProcedure{
				{
					PPGovAction: DijkstraGovAction{
						Action: &DijkstraParameterChangeGovAction{
							ParamUpdate: DijkstraProtocolParameterUpdate{
								RefScriptCostStride: &refScriptCostStride,
							},
						},
					},
				},
			},
		},
	}
	pv9Params := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionConway,
			},
		},
	}
	err := UtxoValidateBootstrapParameterGroups(tx, 0, nil, pv9Params)
	var bootstrapErr conway.BootstrapDisallowedParameterChangeError
	require.ErrorAs(t, err, &bootstrapErr)
	require.Equal(t, []string{"RefScriptCostStride"}, bootstrapErr.Fields)

	pv10Params := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionPlomin,
			},
		},
	}
	require.NoError(t, UtxoValidateBootstrapParameterGroups(
		tx,
		0,
		nil,
		pv10Params,
	))
}

func TestUtxoValidateRedeemerAndScriptWitnessesPlutusV4(t *testing.T) {
	tx := &DijkstraTransaction{
		WitnessSet: DijkstraTransactionWitnessSet{
			WsPlutusV4Scripts: cbor.NewSetType(
				[]common.PlutusV4Script{{0x41, 0x00}},
				false,
			),
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagSpend, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}

	err := UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
	require.NoError(t, err)
}

func TestUtxoValidateRedeemerAndScriptWitnessesGuardingRedeemer(t *testing.T) {
	guardScript := common.PlutusV4Script{0x41, 0x00}
	guardCred := testGuardScriptCredential(guardScript)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guardCred},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}

	err := UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
	require.ErrorAs(t, err, &common.MissingPlutusScriptWitnessesError{})

	tx.Body.TxSubTransactions = cbor.NewSetType([]DijkstraSubTransaction{
		{
			WitnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV4Scripts: cbor.NewSetType(
					[]common.PlutusV4Script{guardScript},
					false,
				),
			},
		},
	}, false)

	err = UtxoValidateRedeemerAndScriptWitnesses(tx, 0, nil, nil)
	require.NoError(t, err)
}

func TestUtxoValidateExtraneousRedeemersGuarding(t *testing.T) {
	guardScript := common.PlutusV4Script{0x41, 0x00}
	guardCred := testGuardScriptCredential(guardScript)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guardCred},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}

	err := UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
	require.NoError(t, err)

	tx.Body.TxGuards = &DijkstraGuards{
		Credentials: []common.Credential{testGuardCredential()},
	}
	err = UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
	require.ErrorAs(t, err, &conway.ExtraRedeemerError{})

	tx.Body.TxGuards = &DijkstraGuards{
		Credentials: []common.Credential{guardCred},
	}
	delete(tx.WitnessSet.WsRedeemers.Redeemers, common.RedeemerKey{
		Tag:   common.RedeemerTagGuarding,
		Index: 0,
	})
	tx.WitnessSet.WsRedeemers.Redeemers[common.RedeemerKey{
		Tag:   common.RedeemerTagGuarding,
		Index: 1,
	}] = common.RedeemerValue{
		ExUnits: common.ExUnits{Steps: 1, Memory: 1},
	}

	err = UtxoValidateExtraneousRedeemers(tx, 0, nil, nil)
	require.ErrorAs(t, err, &conway.ExtraRedeemerError{})
}

func TestUtxoValidatePlutusScriptsGuardingRedeemer(t *testing.T) {
	guardScript := common.PlutusV4Script{0x41, 0x00}
	guardCred := testGuardScriptCredential(guardScript)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guardCred},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}

	err := UtxoValidatePlutusScripts(
		tx,
		0,
		mockledger.NewLedgerStateBuilder().Build(),
		&DijkstraProtocolParameters{},
	)
	require.ErrorAs(t, err, &common.MissingScriptWitnessesError{})
}

// TestNewTxInfoFromTransactionGuardingRedeemer proves that
// transactionWithoutGuardingRedeemers (used by validateGuardingPlutusScripts
// when building a TxInfo for guard-script evaluation) is both necessary and
// sufficient to keep a genuine RedeemerTagGuarding entry from tripping the
// generic redeemer-purpose builder's fail-closed check.
//
// Without the wrapper, the shared script-context code can't build a
// ScriptPurpose for a guarding redeemer (it's handled separately, outside
// the generic purpose builder) and now correctly rejects it with
// UnmatchedRedeemerError. With the wrapper in place, the guarding redeemer
// is filtered out before reaching the purpose builder, so TxInfo
// construction succeeds, matching how validateGuardingPlutusScripts already
// calls it in production for the non-guarding path.
func TestNewTxInfoFromTransactionGuardingRedeemer(t *testing.T) {
	guardScript := common.PlutusV4Script{0x41, 0x00}
	guardCred := testGuardScriptCredential(guardScript)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxGuards: &DijkstraGuards{
				Credentials: []common.Credential{guardCred},
			},
		},
		WitnessSet: DijkstraTransactionWitnessSet{
			WsRedeemers: DijkstraRedeemers{
				Redeemers: map[common.RedeemerKey]common.RedeemerValue{
					{Tag: common.RedeemerTagGuarding, Index: 0}: {
						ExUnits: common.ExUnits{Steps: 1, Memory: 1},
					},
				},
			},
		},
		TxIsValid: true,
	}
	ls := mockledger.NewLedgerStateBuilder().Build()

	t.Run("unwrapped fails closed", func(t *testing.T) {
		_, err := script.NewTxInfoV1FromTransaction(ls, tx, nil)
		var unmatchedErr script.UnmatchedRedeemerError
		require.ErrorAs(t, err, &unmatchedErr)

		_, err = script.NewTxInfoV2FromTransaction(ls, tx, nil)
		require.ErrorAs(t, err, &unmatchedErr)

		_, err = script.NewTxInfoV3FromTransaction(ls, tx, nil)
		require.ErrorAs(t, err, &unmatchedErr)
	})

	t.Run("wrapped succeeds", func(t *testing.T) {
		wrapped := transactionWithoutGuardingRedeemers{Transaction: tx}

		_, err := script.NewTxInfoV1FromTransaction(ls, wrapped, nil)
		require.NoError(t, err)

		_, err = script.NewTxInfoV2FromTransaction(ls, wrapped, nil)
		require.NoError(t, err)

		_, err = script.NewTxInfoV3FromTransaction(ls, wrapped, nil)
		require.NoError(t, err)
	})
}

func txWithRefScripts(sizes ...int) *DijkstraTransaction {
	outputs := make([]DijkstraTransactionOutput, len(sizes))
	for i, size := range sizes {
		script := make(common.PlutusV4Script, size)
		outputs[i] = DijkstraTransactionOutput{
			Output: babbage.BabbageTransactionOutput{
				TxOutScriptRef: &common.ScriptRef{
					Script: script,
				},
			},
		}
	}
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{TxOutputs: outputs},
	}
}

func dijkstraRefScriptInput(
	t *testing.T,
	hashByte byte,
	index int,
	scriptSize int,
) (shelley.ShelleyTransactionInput, common.Utxo) {
	t.Helper()
	input := shelley.NewShelleyTransactionInput(
		strings.Repeat(fmt.Sprintf("%02x", hashByte), 32),
		index,
	)
	output := &babbage.BabbageTransactionOutput{
		TxOutScriptRef: &common.ScriptRef{
			Script: make(common.PlutusV4Script, scriptSize),
		},
	}
	return input, common.Utxo{Id: input, Output: output}
}

func dijkstraRefScriptLedgerState(
	t *testing.T,
	utxos ...common.Utxo,
) common.LedgerState {
	t.Helper()
	byInput := make(map[string]common.Utxo, len(utxos))
	for _, utxo := range utxos {
		byInput[utxo.Id.String()] = utxo
	}
	return mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			utxo, ok := byInput[input.String()]
			if !ok {
				return common.Utxo{}, fmt.Errorf("utxo not found: %s", input)
			}
			return utxo, nil
		}).
		Build()
}

func blockWithRefScripts(txScriptSizes ...[]int) *DijkstraBlock {
	txs := make([]DijkstraTransaction, len(txScriptSizes))
	for i, sizes := range txScriptSizes {
		txs[i] = *txWithRefScripts(sizes...)
	}
	return &DijkstraBlock{
		BlockBody: DijkstraBlockBody{Transactions: txs},
	}
}

func dijkstraTxWithReferenceInputs(
	inputs ...shelley.ShelleyTransactionInput,
) *DijkstraTransaction {
	return &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxReferenceInputs: cbor.NewSetType(inputs, false),
		},
		TxIsValid: true,
	}
}

func dijkstraBlockWithTransactions(
	txs ...*DijkstraTransaction,
) *DijkstraBlock {
	transactions := make([]DijkstraTransaction, len(txs))
	for idx, tx := range txs {
		transactions[idx] = *tx
	}
	return &DijkstraBlock{
		BlockBody: DijkstraBlockBody{Transactions: transactions},
	}
}

// Verifies a transaction with reference scripts below the per-tx limit passes.
func TestUtxoValidateRefScriptSizePerTxBelowLimit(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 100)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 200}
	err := UtxoValidateRefScriptSizePerTx(
		dijkstraTxWithReferenceInputs(input),
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		pp,
	)
	require.NoError(t, err)
}

// Verifies a transaction with reference scripts exactly at the per-tx limit passes.
func TestUtxoValidateRefScriptSizePerTxAtLimit(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 100)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 100}
	err := UtxoValidateRefScriptSizePerTx(
		dijkstraTxWithReferenceInputs(input),
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		pp,
	)
	require.NoError(t, err)
}

// Verifies a transaction exceeding the per-tx reference-script limit fails.
func TestUtxoValidateRefScriptSizePerTxExceedsLimit(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 101)
	tx := &DijkstraTransaction{Body: DijkstraTransactionBody{
		TxReferenceInputs: cbor.NewSetType(
			[]shelley.ShelleyTransactionInput{input},
			false,
		),
	}}
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 100}
	err := UtxoValidateRefScriptSizePerTx(
		tx,
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		pp,
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerTxTooLargeError{})
}

// Verifies publishing a reference script does not consume the per-tx limit.
func TestUtxoValidateRefScriptSizePerTxPublishingOnly(t *testing.T) {
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 100}
	err := UtxoValidateRefScriptSizePerTx(
		txWithRefScripts(101),
		0,
		dijkstraRefScriptLedgerState(t),
		pp,
	)
	require.NoError(t, err)
}

// Verifies a zero per-tx reference-script limit permits no consumed scripts.
func TestUtxoValidateRefScriptSizePerTxZeroLimit(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 1)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 0}
	err := UtxoValidateRefScriptSizePerTx(
		dijkstraTxWithReferenceInputs(input),
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		pp,
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerTxTooLargeError{})
}

// Verifies Conway protocol params use Conway's static per-tx limit.
func TestUtxoValidateRefScriptSizePerTxConwayParams(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(
		t,
		0x01,
		0,
		int(conway.MaxRefScriptSizePerTx+1),
	)
	pp := &conway.ConwayProtocolParameters{}
	err := UtxoValidateRefScriptSizePerTx(
		dijkstraTxWithReferenceInputs(input),
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		pp,
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerTxTooLargeError{})
}

func TestUtxoValidateRefScriptSizePerTxOverlappingInputCountedOnce(
	t *testing.T,
) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 100)
	tx := dijkstraTxWithReferenceInputs(input)
	tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 100}
	err := UtxoValidateRefScriptSizePerTx(
		tx,
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		pp,
	)
	require.NoError(t, err)
}

func TestUtxoValidateRefScriptSizePerTxDistinctIdenticalScriptsCountedTwice(
	t *testing.T,
) {
	inputA, utxoA := dijkstraRefScriptInput(t, 0x01, 0, 60)
	inputB, utxoB := dijkstraRefScriptInput(t, 0x02, 0, 60)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 100}
	err := UtxoValidateRefScriptSizePerTx(
		dijkstraTxWithReferenceInputs(inputA, inputB),
		0,
		dijkstraRefScriptLedgerState(t, utxoA, utxoB),
		pp,
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerTxTooLargeError{})
}

func TestUtxoValidateRefScriptSizePerTxIncludesSubTransactions(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 101)
	tx := &DijkstraTransaction{
		Body: DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]DijkstraSubTransaction{
					{
						Body: DijkstraSubTransactionBody{
							TxReferenceInputs: cbor.NewSetType(
								[]shelley.ShelleyTransactionInput{input},
								false,
							),
						},
					},
				},
				false,
			),
		},
		TxIsValid: true,
	}
	err := UtxoValidateRefScriptSizePerTx(
		tx,
		0,
		dijkstraRefScriptLedgerState(t, utxo),
		&DijkstraProtocolParameters{MaxRefScriptSizePerTx: 100},
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerTxTooLargeError{})
}

// Verifies a block with reference scripts below the per-block limit passes.
func TestValidateRefScriptSizePerBlockBelowLimit(t *testing.T) {
	inputA, utxoA := dijkstraRefScriptInput(t, 0x01, 0, 100)
	inputB, utxoB := dijkstraRefScriptInput(t, 0x02, 0, 100)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerBlock: 300}
	err := ValidateRefScriptSizePerBlock(
		dijkstraBlockWithTransactions(
			dijkstraTxWithReferenceInputs(inputA),
			dijkstraTxWithReferenceInputs(inputB),
		),
		pp,
		dijkstraRefScriptLedgerState(t, utxoA, utxoB),
	)
	require.NoError(t, err)
}

// Verifies Conway protocol params do not fail Dijkstra per-block validation.
func TestValidateRefScriptSizePerBlockConwayParams(t *testing.T) {
	pp := &conway.ConwayProtocolParameters{}
	err := ValidateRefScriptSizePerBlock(
		blockWithRefScripts([]int{99999}, []int{99999}),
		pp,
	)
	require.NoError(t, err)
}

func TestValidateRefScriptSizePerBlockPublishingOnly(t *testing.T) {
	err := ValidateRefScriptSizePerBlock(
		blockWithRefScripts([]int{101}),
		&DijkstraProtocolParameters{MaxRefScriptSizePerBlock: 100},
	)
	require.NoError(t, err)
}

// Verifies a block with reference scripts exactly at the per-block limit passes.
func TestValidateRefScriptSizePerBlockAtLimit(t *testing.T) {
	inputA, utxoA := dijkstraRefScriptInput(t, 0x01, 0, 100)
	inputB, utxoB := dijkstraRefScriptInput(t, 0x02, 0, 100)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerBlock: 200}
	err := ValidateRefScriptSizePerBlock(
		dijkstraBlockWithTransactions(
			dijkstraTxWithReferenceInputs(inputA),
			dijkstraTxWithReferenceInputs(inputB),
		),
		pp,
		dijkstraRefScriptLedgerState(t, utxoA, utxoB),
	)
	require.NoError(t, err)
}

// Verifies a block exceeding the per-block reference-script limit fails.
func TestValidateRefScriptSizePerBlockExceedsLimit(t *testing.T) {
	inputA, utxoA := dijkstraRefScriptInput(t, 0x01, 0, 101)
	inputB, utxoB := dijkstraRefScriptInput(t, 0x02, 0, 100)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerBlock: 200}
	err := ValidateRefScriptSizePerBlock(
		dijkstraBlockWithTransactions(
			dijkstraTxWithReferenceInputs(inputA),
			dijkstraTxWithReferenceInputs(inputB),
		),
		pp,
		dijkstraRefScriptLedgerState(t, utxoA, utxoB),
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerBlockTooLargeError{})
}

// Verifies a zero per-block reference-script limit permits no consumed scripts.
func TestValidateRefScriptSizePerBlockZeroLimit(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 1)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerBlock: 0}
	err := ValidateRefScriptSizePerBlock(
		dijkstraBlockWithTransactions(dijkstraTxWithReferenceInputs(input)),
		pp,
		dijkstraRefScriptLedgerState(t, utxo),
	)
	require.ErrorAs(t, err, &common.RefScriptSizePerBlockTooLargeError{})
}

func TestDijkstraRefScriptFeeUsesConsumedScriptSet(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 150)
	tx := dijkstraTxWithReferenceInputs(input)
	tx.SetCbor([]byte{0x83, 0xa0, 0xa0, 0xf6})
	pp := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: big.NewRat(1, 1)},
		},
		MaxRefScriptSizePerTx:   150,
		RefScriptCostStride:     100,
		RefScriptCostMultiplier: &cbor.Rat{Rat: big.NewRat(2, 1)},
	}
	ls := dijkstraRefScriptLedgerState(t, utxo)
	minFee, err := MinFeeTxWithUtxo(tx, pp, ls)
	require.NoError(t, err)
	require.Equal(t, uint64(200), minFee)

	tx.Body.TxFee = minFee - 1
	err = UtxoValidateFeeTooSmallUtxo(tx, 0, ls, pp)
	require.ErrorAs(t, err, &shelley.FeeTooSmallUtxoError{})
	require.NoError(t, UtxoValidateRefScriptSizePerTx(tx, 0, ls, pp))

	publishingTx := txWithRefScripts(150)
	publishingTx.SetCbor([]byte{0x83, 0xa0, 0xa0, 0xf6})
	publishingFee, err := MinFeeTxWithUtxo(
		publishingTx,
		pp,
		dijkstraRefScriptLedgerState(t),
	)
	require.NoError(t, err)
	require.Zero(t, publishingFee)
}

func TestDijkstraRefScriptFeeUsesConwayDefaults(t *testing.T) {
	input, utxo := dijkstraRefScriptInput(
		t,
		0x01,
		0,
		int(conway.RefScriptCostStride*2),
	)
	tx := dijkstraTxWithReferenceInputs(input)
	tx.SetCbor([]byte{0x83, 0xa0, 0xa0, 0xf6})
	conwayPparams := conway.ConwayProtocolParameters{
		MinFeeRefScriptCostPerByte: &cbor.Rat{Rat: big.NewRat(1, 1)},
	}
	tests := []struct {
		name string
		pp   common.ProtocolParameters
	}{
		{
			name: "Dijkstra parameters",
			pp: &DijkstraProtocolParameters{
				ConwayProtocolParameters: conwayPparams,
			},
		},
		{
			name: "Conway parameters",
			pp:   &conwayPparams,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			minFee, err := MinFeeTxWithUtxo(
				tx,
				tc.pp,
				dijkstraRefScriptLedgerState(t, utxo),
			)
			require.NoError(t, err)
			require.Equal(t, uint64(56_320), minFee)
		})
	}
}
