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
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type classicPPUPValidityTransaction struct {
	classicPPUPTestTransaction
	valid bool
}

func (tx classicPPUPValidityTransaction) IsValid() bool { return tx.valid }

// Alonzo and Babbage run the PPUP rule only on the phase-2-valid branch of
// UTXOS (alonzoEvalScriptsTxValid, babbageEvalScriptsTxValid), so an IsValid
// false transaction's update is neither checked nor recorded. UTXOW still
// requires the delegate witness of every proposing key that is in the
// genesis delegation map (witsVKeyNeededGenDelegs in Shelley/UTxO.hs).
func TestClassicPPUPSkippedForPhase2InvalidTransaction(t *testing.T) {
	vkey := bytes.Repeat([]byte{7}, 32)
	delegate := common.Blake2b224Hash(vkey)
	genesisKey := common.Blake2b224Hash(bytes.Repeat([]byte{6}, 32))
	outsider := common.Blake2b224Hash(bytes.Repeat([]byte{8}, 32))
	state := classicPPUPTestState{
		delegates:   []common.Blake2b224{delegate},
		genesisKeys: map[common.Blake2b224]common.Blake2b224{genesisKey: delegate},
		epoch:       10,
		cutoff:      50,
	}
	witness := classicPPUPTestWitnessSet{
		vkeys: []common.VkeyWitness{{Vkey: vkey}},
	}
	newTx := func(
		key common.Blake2b224,
		epoch uint64,
		withWitness bool,
		valid bool,
	) classicPPUPValidityTransaction {
		tx := classicPPUPValidityTransaction{
			classicPPUPTestTransaction: classicPPUPTestTransaction{
				Epoch: epoch,
				Updates: map[common.Blake2b224]common.ProtocolParameterUpdate{
					key: shelley.ShelleyProtocolParameterUpdate{},
				},
				Witness: classicPPUPTestWitnessSet{},
			},
			valid: valid,
		}
		if withWitness {
			tx.Witness = witness
		}
		return tx
	}
	pp := &shelley.ShelleyProtocolParameters{ProtocolMajor: 6}
	for _, era := range []struct {
		name        string
		descriptors func() []common.UtxoValidationRuleDescriptor
	}{
		{"Alonzo", alonzo.UtxoValidationRuleDescriptors},
		{"Babbage", babbage.UtxoValidationRuleDescriptors},
	} {
		t.Run(era.name, func(t *testing.T) {
			validate := classicPPUPValidator(t, era.descriptors())
			require.NoError(
				t,
				validate(newTx(genesisKey, 11, true, false), 49, state, pp),
				"wrong target epoch is not checked",
			)
			require.NoError(
				t,
				validate(newTx(outsider, 10, false, false), 49, state, pp),
				"a non-genesis key needs no witness and is not checked",
			)
			var witnessErr common.ProtocolParameterUpdateWitnessError
			require.ErrorAs(
				t,
				validate(newTx(genesisKey, 10, false, false), 49, state, pp),
				&witnessErr,
			)
			var epochErr common.ProtocolParameterUpdateEpochError
			require.ErrorAs(
				t,
				validate(newTx(genesisKey, 11, true, true), 49, state, pp),
				&epochErr,
			)
		})
	}
}
