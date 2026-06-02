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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

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

func TestUtxoValidateProposalProceduresDijkstraProtocolParameterUpdate(t *testing.T) {
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
