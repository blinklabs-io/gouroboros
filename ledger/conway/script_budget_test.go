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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

func zeroBudgetConwayScript(t *testing.T) common.PlutusV3Script {
	t.Helper()
	program := &syn.Program[syn.DeBruijn]{
		Version: lang.LanguageVersion{1, 1, 0},
		Term: &syn.Lambda[syn.DeBruijn]{
			Body: &syn.Constant{Con: &syn.Unit{}},
		},
	}
	flat, err := syn.Encode(program)
	require.NoError(t, err)
	wrapped, err := cbor.Encode(flat)
	require.NoError(t, err)
	return common.PlutusV3Script(wrapped)
}

func TestUtxoValidatePlutusScriptsUsesDeclaredBudget(t *testing.T) {
	plutusScript := zeroBudgetConwayScript(t)
	policyID := common.Blake2b224(plutusScript.Hash())
	mint := common.NewMultiAsset[*big.Int](
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{
			policyID: {cbor.NewByteString([]byte("budget-test")): big.NewInt(1)},
		},
	)
	newTx := func(units common.ExUnits) *conway.ConwayTransaction {
		return &conway.ConwayTransaction{
			TxIsValid: true,
			Body: conway.ConwayTransactionBody{
				TxMint: &mint,
			},
			WitnessSet: conway.ConwayTransactionWitnessSet{
				WsPlutusV3Scripts: cbor.NewSetType(
					[]common.PlutusV3Script{plutusScript},
					true,
				),
				WsRedeemers: conway.ConwayRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagMint, Index: 0}: {
							Data: common.Datum{
								Data: data.NewInteger(big.NewInt(0)),
							},
							ExUnits: units,
						},
					},
				},
			},
		}
	}
	pp := &conway.ConwayProtocolParameters{
		ProtocolVersion: common.ProtocolParametersProtocolVersion{Major: 11},
		CostModels: map[uint][]int64{
			2: testdata.Epoch653PlutusV3CostModel,
		},
	}
	state := mockledger.NewLedgerStateBuilder().Build()

	zeroErr := conway.UtxoValidatePlutusScripts(
		newTx(common.ExUnits{}),
		0,
		state,
		pp,
	)
	var scriptErr conway.PlutusScriptFailedError
	require.ErrorAs(t, zeroErr, &scriptErr)

	defaultBudget := common.ExUnits{
		Memory: cek.DefaultExBudget.Mem,
		Steps:  cek.DefaultExBudget.Cpu,
	}
	require.NoError(t, conway.UtxoValidatePlutusScripts(
		newTx(defaultBudget),
		0,
		state,
		pp,
	))
}
