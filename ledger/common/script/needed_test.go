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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/common/script"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// noUtxoLedgerState satisfies the LedgerState argument for a transaction with no
// inputs. Every method is nil on purpose, so a call this test did not intend
// panics rather than returning a zero value.
type noUtxoLedgerState struct {
	common.LedgerState
}

// ScriptPurposeVoting.ScriptHash returns Blake2b224(Voter.Hash) with no check on
// Voter.Type, so a key-hash voter yields its key hash typed as a script hash.
// These cases give the voter the same 28 bytes as an available PlutusV1 script,
// which is the shape that turns the missing type check into a false entry in
// Needed.
func TestNewTxScriptViewVotingProcedureVoterTypes(t *testing.T) {
	v1 := common.PlutusV1Script([]byte{0x01, 0x02})
	scriptHash := v1.Hash()
	var voterHash [28]byte
	copy(voterHash[:], scriptHash.Bytes())

	for _, tc := range []struct {
		name       string
		voterType  uint8
		wantNeeded bool
	}{
		{"drep script hash", common.VoterTypeDRepScriptHash, true},
		{
			"committee hot script hash",
			common.VoterTypeConstitutionalCommitteeHotScriptHash,
			true,
		},
		{"drep key hash", common.VoterTypeDRepKeyHash, false},
		{
			"committee hot key hash",
			common.VoterTypeConstitutionalCommitteeHotKeyHash,
			false,
		},
		{
			"staking pool key hash",
			common.VoterTypeStakingPoolKeyHash,
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			voter := common.Voter{Type: tc.voterType, Hash: voterHash}
			tx := &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxVotingProcedures: common.VotingProcedures{}.AddOrReplace(
						voter,
						common.GovActionId{},
						common.VotingProcedure{},
					),
				},
				WitnessSet: conway.ConwayTransactionWitnessSet{
					WsPlutusV1Scripts: cbor.NewSetType(
						[]common.PlutusV1Script{v1},
						false,
					),
				},
			}
			view, err := script.NewTxScriptView(tx, noUtxoLedgerState{})
			require.NoError(t, err)
			require.Contains(
				t,
				view.Available,
				scriptHash,
				"the witness script is reachable regardless of voter type",
			)
			if tc.wantNeeded {
				require.Contains(
					t,
					view.Needed,
					scriptHash,
					"a script voter requires the script it names",
				)
				return
			}
			require.NotContains(
				t,
				view.Needed,
				scriptHash,
				"a key-hash voter must not enter a key hash as a needed script",
			)
		})
	}
}

// A view assembled field-by-field rather than through NewTxScriptView has no
// cached concatenation, so AllResolvedInputs must still build one.
func TestTxScriptViewAllResolvedInputsWithoutCache(t *testing.T) {
	consumed := common.Utxo{}
	reference := common.Utxo{}
	view := script.TxScriptView{
		ResolvedInputs:          []common.Utxo{consumed},
		ResolvedReferenceInputs: []common.Utxo{reference},
	}
	require.Len(t, view.AllResolvedInputs(), 2)
	require.Empty(t, script.TxScriptView{}.AllResolvedInputs())
}
