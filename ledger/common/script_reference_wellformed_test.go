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
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

// TestValidatePlutusScriptsWellFormed_StoredReferenceScript regresses
// blinklabs-io/gouroboros#2316: a transaction that only stores a PlutusV2
// reference script whose UPLC term version is 1.1.0 -- and never
// witnesses/executes it -- must not be rejected as malformed below protocol
// major 11 (the "van Rossem" gate). Real cardano-ledger's decode-time
// well-formedness check (validateScriptsWellFormedTxOuts ->
// deserialiseScript/scriptCBORDecoder) never contains that gate; it is a
// phase-2, execution-time-only check (mkTermToEvaluate) that a
// transaction's own newly-created, stored-but-unexecuted reference script
// can never reach, since it does not exist in the UTxO set until after the
// transaction that creates it applies. A confirmed real Preview transaction
// of exactly this shape (tx
// f9253ce7bb68465fb287dffff459e144b39a7a0abe9717f981f9d8f8e9b943e9) was
// rejected by ValidatePlutusScriptsWellFormed prior to this fix.
func TestValidatePlutusScriptsWellFormed_StoredReferenceScript(t *testing.T) {
	t.Parallel()

	uplc110V2 := common.PlutusV2Script(encodePlutusContextTestScript(
		t,
		lang.LanguageVersion{1, 1, 0},
		3,
		nil,
	))

	t.Run(
		"UPLC 1.1.0 reference script below van Rossem is not malformed",
		func(t *testing.T) {
			t.Parallel()
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxOutputs: []babbage.BabbageTransactionOutput{
						{
							TxOutScriptRef: &common.ScriptRef{
								Type:   common.ScriptRefTypePlutusV2,
								Script: uplc110V2,
							},
						},
					},
				},
			}

			err := common.ValidatePlutusScriptsWellFormed(
				tx,
				common.ProtocolVersionVanRossem-1,
			)
			require.NoError(t, err)
		},
	)

	t.Run(
		"same script as a collateral return is not malformed",
		func(t *testing.T) {
			t.Parallel()
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxCollateralReturn: &babbage.BabbageTransactionOutput{
						TxOutScriptRef: &common.ScriptRef{
							Type:   common.ScriptRefTypePlutusV2,
							Script: uplc110V2,
						},
					},
				},
			}

			err := common.ValidatePlutusScriptsWellFormed(
				tx,
				common.ProtocolVersionVanRossem-1,
			)
			require.NoError(t, err)
		},
	)

	t.Run(
		"UPLC 1.1.0 reference script at van Rossem is not malformed",
		func(t *testing.T) {
			t.Parallel()
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxOutputs: []babbage.BabbageTransactionOutput{
						{
							TxOutScriptRef: &common.ScriptRef{
								Type:   common.ScriptRefTypePlutusV2,
								Script: uplc110V2,
							},
						},
					},
				},
			}

			err := common.ValidatePlutusScriptsWellFormed(
				tx,
				common.ProtocolVersionVanRossem,
			)
			require.NoError(t, err)
		},
	)

	t.Run(
		"genuinely malformed reference script is still rejected",
		func(t *testing.T) {
			t.Parallel()
			// Unsupported UPLC program version 1.2.0: a decode-time defect
			// independent of the van Rossem gate, so it must still be
			// caught by the well-formedness check.
			badScript := common.PlutusV2Script(encodePlutusContextTestScript(
				t,
				lang.LanguageVersion{1, 2, 0},
				3,
				nil,
			))
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxOutputs: []babbage.BabbageTransactionOutput{
						{
							TxOutScriptRef: &common.ScriptRef{
								Type:   common.ScriptRefTypePlutusV2,
								Script: badScript,
							},
						},
					},
				},
			}

			err := common.ValidatePlutusScriptsWellFormed(
				tx,
				common.ProtocolVersionVanRossem-1,
			)
			require.Error(t, err)
			require.ErrorIs(t, err, common.ErrMalformedReferenceScripts)
		},
	)
}
