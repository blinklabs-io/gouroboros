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

package babbage_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

func TestBabbagePlutusExecutionMatchesValidityFlag(t *testing.T) {
	newTx := func(fail, valid bool) *babbage.BabbageTransaction {
		plutus := testdata.PlutusScript(t, lang.LanguageVersionV2)
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
		return &babbage.BabbageTransaction{
			Body: babbage.BabbageTransactionBody{TxMint: &mint},
			WitnessSet: babbage.BabbageTransactionWitnessSet{
				WsPlutusV2Scripts: []common.PlutusV2Script{
					plutus.(common.PlutusV2Script),
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
	costModel := make([]int64, len(lang.CostModelParamNamesV2))
	for idx := range costModel {
		costModel[idx] = 1
	}
	params := &babbage.BabbageProtocolParameters{
		ProtocolMajor: common.ProtocolVersionBabbage,
		CostModels:    map[uint][]int64{1: costModel},
	}
	state := mockledger.NewLedgerStateBuilder().Build()

	require.NoError(t, babbage.UtxoValidatePlutusScripts(newTx(false, true), 0, state, params))
	var scriptFailure common.PlutusScriptFailedError
	require.ErrorAs(t, babbage.UtxoValidatePlutusScripts(newTx(true, true), 0, state, params), &scriptFailure)
	require.NoError(t, babbage.UtxoValidatePlutusScripts(newTx(true, false), 0, state, params))
	var unexpectedSuccess common.UnexpectedPlutusScriptSuccessError
	err := babbage.UtxoValidatePlutusScripts(newTx(false, false), 0, state, params)
	require.True(t, errors.As(err, &unexpectedSuccess))
}
