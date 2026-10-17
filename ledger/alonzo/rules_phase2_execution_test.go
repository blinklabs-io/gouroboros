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

package alonzo_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

func TestAlonzoPlutusExecutionMatchesValidityFlag(t *testing.T) {
	newTx := func(fail, valid bool) *alonzo.AlonzoTransaction {
		plutus := testdata.PlutusScript(t, lang.LanguageVersionV1)
		policy := common.Blake2b224(plutus.Hash())
		mint := common.NewMultiAsset[*big.Int](
			map[common.Blake2b224]map[cbor.ByteString]*big.Int{
				policy: {cbor.NewByteString(nil): big.NewInt(1)},
			},
		)
		units := common.ExUnits{Memory: 10_000_000, Steps: 10_000_000}
		if fail {
			units = common.ExUnits{}
		}
		return &alonzo.AlonzoTransaction{
			Body: alonzo.AlonzoTransactionBody{TxMint: &mint},
			WitnessSet: alonzo.AlonzoTransactionWitnessSet{
				WsPlutusV1Scripts: []common.PlutusV1Script{
					plutus.(common.PlutusV1Script),
				},
				WsRedeemers: alonzo.AlonzoRedeemers{Redeemers: []alonzo.AlonzoRedeemer{{
					Tag:     common.RedeemerTagMint,
					Data:    common.Datum{Data: data.NewInteger(big.NewInt(1))},
					ExUnits: units,
				}}},
			},
			TxIsValid: valid,
		}
	}
	costModel := make([]int64, len(lang.CostModelParamNamesV1))
	for idx := range costModel {
		costModel[idx] = 1
	}
	params := &alonzo.AlonzoProtocolParameters{
		ProtocolMajor: common.ProtocolVersionAlonzo,
		CostModels:    map[uint][]int64{0: costModel},
	}
	state := mockledger.NewLedgerStateBuilder().Build()

	require.NoError(t, alonzo.UtxoValidatePlutusScripts(newTx(false, true), 0, state, params))
	var scriptFailure common.PlutusScriptFailedError
	require.ErrorAs(t, alonzo.UtxoValidatePlutusScripts(newTx(true, true), 0, state, params), &scriptFailure)
	require.NoError(t, alonzo.UtxoValidatePlutusScripts(newTx(true, false), 0, state, params))
	var unexpectedSuccess common.UnexpectedPlutusScriptSuccessError
	err := alonzo.UtxoValidatePlutusScripts(newTx(false, false), 0, state, params)
	require.True(t, errors.As(err, &unexpectedSuccess))
}

func TestAlonzoMalformedScriptIsPhaseOneFailure(t *testing.T) {
	plutus := common.PlutusV1Script{0xff}
	policy := common.Blake2b224(plutus.Hash())
	mint := common.NewMultiAsset[*big.Int](
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			policy: {cbor.NewByteString(nil): big.NewInt(1)},
		},
	)
	tx := &alonzo.AlonzoTransaction{
		Body: alonzo.AlonzoTransactionBody{TxMint: &mint},
		WitnessSet: alonzo.AlonzoTransactionWitnessSet{
			WsPlutusV1Scripts: []common.PlutusV1Script{plutus},
			WsRedeemers: alonzo.AlonzoRedeemers{Redeemers: []alonzo.AlonzoRedeemer{{
				Tag:     common.RedeemerTagMint,
				ExUnits: common.ExUnits{Memory: 1, Steps: 1},
			}}},
		},
		TxIsValid: false,
	}
	params := &alonzo.AlonzoProtocolParameters{
		ProtocolMajor: common.ProtocolVersionAlonzo,
	}
	state := mockledger.NewLedgerStateBuilder().Build()

	// Execution errors alone look like the declared phase-2 failure, so the
	// always-run well-formedness rule must reject malformed program bytes first.
	require.NoError(t, alonzo.UtxoValidatePlutusScripts(tx, 0, state, params))

	descriptors := alonzo.UtxoValidationRuleDescriptors()
	wellFormedIdx, phase2Idx := -1, -1
	for idx, descriptor := range descriptors {
		switch descriptor.Id {
		case common.UtxoValidationRuleMalformedReferenceScripts:
			wellFormedIdx = idx
		case common.UtxoValidationRulePlutusScripts:
			phase2Idx = idx
		}
	}
	require.NotEqual(t, -1, wellFormedIdx)
	require.Greater(t, phase2Idx, wellFormedIdx)
	err := common.VerifyTransaction(
		tx,
		0,
		state,
		params,
		alonzo.UtxoValidationRules[wellFormedIdx:phase2Idx+1],
	)
	require.ErrorIs(t, err, common.ErrMalformedScriptWitnesses)
}
