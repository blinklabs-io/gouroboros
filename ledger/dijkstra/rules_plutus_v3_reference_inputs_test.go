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

	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/stretchr/testify/require"
)

type overlapPurpose int

const (
	overlapPurposeNone overlapPurpose = iota
	overlapPurposeRewarding
	overlapPurposeSpending
	overlapPurposeGuarding
)

// dijkstraOverlappingInputTx builds a transaction whose only input is also a
// reference input, with the given script executing for the given purpose.
func dijkstraOverlappingInputTx(
	t *testing.T,
	purpose overlapPurpose,
	script common.Script,
) (*DijkstraTransaction, common.LedgerState) {
	t.Helper()
	redeemer := common.RedeemerValue{
		ExUnits: common.ExUnits{Steps: 10_000_000, Memory: 10_000_000},
	}
	input := shelley.NewShelleyTransactionInput(
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		0,
	)
	utxo := common.Utxo{
		Id: input,
		Output: babbage.BabbageTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{Amount: 2_000_000},
		},
	}
	tx := &DijkstraTransaction{TxIsValid: true}
	switch purpose {
	case overlapPurposeRewarding:
		tx, _ = testDijkstraWithdrawalTx(t, 0, script)
		tx.WitnessSet.WsRedeemers = DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagReward, Index: 0}: redeemer,
			},
		}
	case overlapPurposeSpending:
		input, utxo = dijkstraScriptLockedInput(t, script, 0)
		tx.WitnessSet = testDijkstraWitnessSet(t, script)
		tx.WitnessSet.WsRedeemers = DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: redeemer,
			},
		}
	case overlapPurposeGuarding:
		tx.Body.TxGuards = &DijkstraGuards{
			Credentials: []common.Credential{
				dijkstraGuardCredentialForScript(script),
			},
		}
		tx.WitnessSet = testDijkstraWitnessSet(t, script)
		tx.WitnessSet.WsRedeemers = DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding, Index: 0}: redeemer,
			},
		}
	}
	tx.Body.TxInputs = conway.NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)
	tx.Body.TxReferenceInputs = dijkstraReferenceInputSet(input)
	return tx, mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
}

// TestUtxoValidatePlutusScriptsV3ReferenceInputOverlap pins the cardano-ledger
// behavior (Dijkstra TxInfo.hs, PlutusV3 toPlutusTxInfo): the overlap is a
// context-translation error that only a Plutus V3 execution can raise.
func TestUtxoValidatePlutusScriptsV3ReferenceInputOverlap(t *testing.T) {
	v3 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV3, false)
	v1 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV1, false)
	v2 := dijkstraGuardTestPlutus(t, lang.LanguageVersionV2, false)
	for _, tc := range []struct {
		name       string
		purpose    overlapPurpose
		script     common.Script
		major      uint
		wantReject bool
	}{
		{
			name:       "ordinary V3 rewarding PV11",
			purpose:    overlapPurposeRewarding,
			script:     v3,
			major:      common.ProtocolVersionVanRossem,
			wantReject: true,
		},
		{
			name:       "ordinary V3 rewarding PV12",
			purpose:    overlapPurposeRewarding,
			script:     v3,
			major:      common.ProtocolVersionDijkstra,
			wantReject: true,
		},
		{
			name:       "ordinary V3 spending PV11",
			purpose:    overlapPurposeSpending,
			script:     v3,
			major:      common.ProtocolVersionVanRossem,
			wantReject: true,
		},
		{
			name:       "guarding V3 PV11",
			purpose:    overlapPurposeGuarding,
			script:     v3,
			major:      common.ProtocolVersionVanRossem,
			wantReject: true,
		},
		{
			name:       "guarding V3 PV12",
			purpose:    overlapPurposeGuarding,
			script:     v3,
			major:      common.ProtocolVersionDijkstra,
			wantReject: true,
		},
		{
			name:    "no Plutus PV11",
			purpose: overlapPurposeNone,
			major:   common.ProtocolVersionVanRossem,
		},
		{
			name:    "V1 rewarding PV11",
			purpose: overlapPurposeRewarding,
			script:  v1,
			major:   common.ProtocolVersionVanRossem,
		},
		{
			name:    "V2 rewarding PV11",
			purpose: overlapPurposeRewarding,
			script:  v2,
			major:   common.ProtocolVersionVanRossem,
		},
		{
			name:    "V1 rewarding PV12",
			purpose: overlapPurposeRewarding,
			script:  v1,
			major:   common.ProtocolVersionDijkstra,
		},
		{
			name:    "V2 rewarding PV12",
			purpose: overlapPurposeRewarding,
			script:  v2,
			major:   common.ProtocolVersionDijkstra,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx, state := dijkstraOverlappingInputTx(t, tc.purpose, tc.script)
			pp := dijkstraGuardTestPParams()
			pp.ProtocolVersion.Major = tc.major
			err := UtxoValidatePlutusScripts(tx, 0, state, pp)
			if tc.wantReject {
				var construction conway.ScriptContextConstructionError
				require.ErrorAs(t, err, &construction)
				require.ErrorContains(t, err, "is also a regular input")
				return
			}
			require.NoError(t, err)
		})
	}
}
