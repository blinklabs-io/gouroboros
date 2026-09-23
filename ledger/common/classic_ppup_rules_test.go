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
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type classicPPUPTestTransaction struct {
	common.Transaction
	Epoch   uint64
	Updates map[common.Blake2b224]common.ProtocolParameterUpdate
	Witness common.TransactionWitnessSet
}

func TestClassicPPUPThroughVerifyTransaction(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	delegate := common.Blake2b224Hash(publicKey)
	state := classicPPUPTestState{
		delegates: []common.Blake2b224{delegate},
		epoch:     7,
		cutoff:    40,
	}
	makeTransaction := func(epoch uint64, proposer common.Blake2b224, includeWitness bool) *shelley.ShelleyTransaction {
		update := shelley.ShelleyProtocolParameterUpdate{}
		tx := &shelley.ShelleyTransaction{
			Body: shelley.ShelleyTransactionBody{
				Update: &shelley.ShelleyTransactionPparamUpdate{
					ProtocolParamUpdates: map[common.Blake2b224]shelley.ShelleyProtocolParameterUpdate{
						proposer: update,
					},
					Epoch: epoch,
				},
			},
		}
		if includeWitness {
			hash := tx.Hash()
			tx.WitnessSet.VkeyWitnesses = []common.VkeyWitness{{
				Vkey:      publicKey,
				Signature: ed25519.Sign(privateKey, hash[:]),
			}}
		}
		return tx
	}
	rules := []common.UtxoValidationRuleFunc{
		common.UtxoValidateSignatures,
		shelley.UtxoValidateProtocolParameterUpdates,
	}
	pp := &shelley.ShelleyProtocolParameters{ProtocolMajor: 8}
	require.NoError(t, common.VerifyTransaction(makeTransaction(7, delegate, true), 39, state, pp, rules))
	wrongEpochErr := common.VerifyTransaction(makeTransaction(8, delegate, true), 39, state, pp, rules)
	var epochErr common.ProtocolParameterUpdateEpochError
	require.ErrorAs(t, wrongEpochErr, &epochErr)
	unknown := common.Blake2b224Hash(bytes.Repeat([]byte{4}, 32))
	unknownErr := common.VerifyTransaction(makeTransaction(7, unknown, true), 39, state, pp, rules)
	var delegateErr common.ProtocolParameterUpdateDelegateError
	require.ErrorAs(t, unknownErr, &delegateErr)
	missingWitnessErr := common.VerifyTransaction(makeTransaction(7, delegate, false), 39, state, pp, rules)
	var witnessErr common.ProtocolParameterUpdateWitnessError
	require.ErrorAs(t, missingWitnessErr, &witnessErr)
}

func (tx classicPPUPTestTransaction) ProtocolParameterUpdates() (
	uint64,
	map[common.Blake2b224]common.ProtocolParameterUpdate,
) {
	return tx.Epoch, tx.Updates
}

func (tx classicPPUPTestTransaction) Witnesses() common.TransactionWitnessSet {
	return tx.Witness
}

type classicPPUPTestWitnessSet struct {
	common.TransactionWitnessSet
	vkeys []common.VkeyWitness
}

func (w classicPPUPTestWitnessSet) Vkey() []common.VkeyWitness { return w.vkeys }

type classicPPUPTestState struct {
	common.LedgerState
	delegates []common.Blake2b224
	epoch     uint64
	cutoff    uint64
}

func (s classicPPUPTestState) GenesisDelegateKeyHashes() ([]common.Blake2b224, error) {
	return s.delegates, nil
}

func (classicPPUPTestState) GenesisUpdateQuorum() (uint, error) { return 1, nil }

func (s classicPPUPTestState) ProtocolParameterUpdateWindow(
	uint64,
) (uint64, uint64, error) {
	return s.epoch, s.cutoff, nil
}

func classicPPUPValidator(
	t *testing.T,
	descriptors []common.UtxoValidationRuleDescriptor,
) common.UtxoValidationRuleFunc {
	t.Helper()
	for _, descriptor := range descriptors {
		if descriptor.Id == common.UtxoValidationRuleProtocolParameterUpdates {
			return descriptor.Validator
		}
	}
	t.Fatal("classic protocol parameter update rule is not registered")
	return nil
}

func TestClassicProtocolParameterUpdateRuleRegistrationAndEpochs(t *testing.T) {
	vkey := bytes.Repeat([]byte{7}, 32)
	delegate := common.Blake2b224Hash(vkey)
	state := classicPPUPTestState{delegates: []common.Blake2b224{delegate}, epoch: 10, cutoff: 50}
	witness := classicPPUPTestWitnessSet{vkeys: []common.VkeyWitness{{Vkey: vkey}}}
	update := common.ProtocolParameterUpdate(shelley.ShelleyProtocolParameterUpdate{})
	newTx := func(epoch uint64, key common.Blake2b224, withWitness bool) classicPPUPTestTransaction {
		tx := classicPPUPTestTransaction{
			Epoch:   epoch,
			Updates: map[common.Blake2b224]common.ProtocolParameterUpdate{key: update},
		}
		if withWitness {
			tx.Witness = witness
		}
		return tx
	}
	allEraDescriptors := []struct {
		name        string
		descriptors func() []common.UtxoValidationRuleDescriptor
	}{
		{"Shelley", shelley.UtxoValidationRuleDescriptors},
		{"Allegra", allegra.UtxoValidationRuleDescriptors},
		{"Mary", mary.UtxoValidationRuleDescriptors},
		{"Alonzo", alonzo.UtxoValidationRuleDescriptors},
		{"Babbage", babbage.UtxoValidationRuleDescriptors},
	}
	for _, era := range allEraDescriptors {
		t.Run(era.name, func(t *testing.T) {
			validate := classicPPUPValidator(t, era.descriptors())
			require.NoError(t, validate(newTx(10, delegate, true), 49, state, &shelley.ShelleyProtocolParameters{}))
		})
	}
	validate := classicPPUPValidator(t, shelley.UtxoValidationRuleDescriptors())
	t.Run("unknown genesis key", func(t *testing.T) {
		unknown := common.Blake2b224Hash(bytes.Repeat([]byte{8}, 32))
		var updateErr common.ProtocolParameterUpdateDelegateError
		require.ErrorAs(t, validate(newTx(10, unknown, true), 49, state, &shelley.ShelleyProtocolParameters{}), &updateErr)
		require.Equal(t, unknown, updateErr.Delegate)
	})
	t.Run("missing proposing delegate witness", func(t *testing.T) {
		var updateErr common.ProtocolParameterUpdateWitnessError
		require.ErrorAs(t, validate(newTx(10, delegate, false), 49, state, &shelley.ShelleyProtocolParameters{}), &updateErr)
		require.Equal(t, delegate, updateErr.Delegate)
	})
	t.Run("wrong current epoch", func(t *testing.T) {
		var updateErr common.ProtocolParameterUpdateEpochError
		require.ErrorAs(t, validate(newTx(11, delegate, true), 49, state, &shelley.ShelleyProtocolParameters{}), &updateErr)
		require.Equal(t, uint64(10), updateErr.Expected)
		require.False(t, updateErr.ForNextEpoch)
	})
	t.Run("wrong epoch after no-return slot", func(t *testing.T) {
		var updateErr common.ProtocolParameterUpdateEpochError
		require.ErrorAs(t, validate(newTx(10, delegate, true), 50, state, &shelley.ShelleyProtocolParameters{}), &updateErr)
		require.Equal(t, uint64(11), updateErr.Expected)
		require.True(t, updateErr.ForNextEpoch)
	})
	t.Run("next epoch after no-return slot", func(t *testing.T) {
		require.NoError(t, validate(newTx(11, delegate, true), 50, state, &shelley.ShelleyProtocolParameters{}))
	})
	t.Run("reject invalid protocol version jump", func(t *testing.T) {
		proposed := common.ProtocolParametersProtocolVersion{Major: 12}
		versionUpdate := shelley.ShelleyProtocolParameterUpdate{ProtocolVersion: &proposed}
		tx := newTx(10, delegate, true)
		tx.Updates[delegate] = versionUpdate
		params := &shelley.ShelleyProtocolParameters{ProtocolMajor: 10, ProtocolMinor: 0}
		var updateErr common.ProtocolParameterUpdateVersionError
		require.ErrorAs(t, validate(tx, 49, state, params), &updateErr)
	})
	t.Run("accept next major protocol version", func(t *testing.T) {
		proposed := common.ProtocolParametersProtocolVersion{Major: 11}
		versionUpdate := shelley.ShelleyProtocolParameterUpdate{ProtocolVersion: &proposed}
		tx := newTx(10, delegate, true)
		tx.Updates[delegate] = versionUpdate
		params := &shelley.ShelleyProtocolParameters{ProtocolMajor: 10, ProtocolMinor: 0}
		require.NoError(t, validate(tx, 49, state, params))
	})
}

func TestClassicCostModelUpdateProtocolVersionBoundary(t *testing.T) {
	vkey := bytes.Repeat([]byte{9}, 32)
	delegate := common.Blake2b224Hash(vkey)
	state := classicPPUPTestState{
		delegates: []common.Blake2b224{delegate},
		epoch:     3,
		cutoff:    100,
	}
	witness := classicPPUPTestWitnessSet{vkeys: []common.VkeyWitness{{Vkey: vkey}}}
	for _, era := range []struct {
		name        string
		descriptors func() []common.UtxoValidationRuleDescriptor
		params      func(uint) common.ProtocolParameters
		newUpdate   func(map[uint][]int64) common.ProtocolParameterUpdate
		valid       map[uint][]int64
		invalid     []map[uint][]int64
	}{
		{
			name:        "Alonzo",
			descriptors: alonzo.UtxoValidationRuleDescriptors,
			params: func(version uint) common.ProtocolParameters {
				return &alonzo.AlonzoProtocolParameters{ProtocolMajor: version}
			},
			newUpdate: func(models map[uint][]int64) common.ProtocolParameterUpdate {
				return alonzo.AlonzoProtocolParameterUpdate{CostModels: models}
			},
			valid: map[uint][]int64{alonzo.PlutusV1Key: make([]int64, 166)},
			invalid: []map[uint][]int64{
				{alonzo.PlutusV1Key: make([]int64, 165)},
				{1: make([]int64, 175)},
				{alonzo.PlutusV1Key: make([]int64, 167)},
			},
		},
		{
			name:        "Babbage",
			descriptors: babbage.UtxoValidationRuleDescriptors,
			params: func(version uint) common.ProtocolParameters {
				return &babbage.BabbageProtocolParameters{ProtocolMajor: version}
			},
			newUpdate: func(models map[uint][]int64) common.ProtocolParameterUpdate {
				return babbage.BabbageProtocolParameterUpdate{CostModels: models}
			},
			valid: map[uint][]int64{
				alonzo.PlutusV1Key: make([]int64, 166),
				alonzo.PlutusV2Key: make([]int64, 175),
			},
			invalid: []map[uint][]int64{
				{alonzo.PlutusV1Key: make([]int64, 165)},
				{alonzo.PlutusV2Key: make([]int64, 174)},
				{alonzo.PlutusV2Key: make([]int64, 176)},
				{2: make([]int64, 187)},
			},
		},
	} {
		t.Run(era.name, func(t *testing.T) {
			validate := classicPPUPValidator(t, era.descriptors())
			validateModels := func(models map[uint][]int64, version uint) error {
				tx := classicPPUPTestTransaction{
					Epoch: 3,
					Updates: map[common.Blake2b224]common.ProtocolParameterUpdate{
						delegate: era.newUpdate(models),
					},
					Witness: witness,
				}
				return validate(tx, 10, state, era.params(version))
			}
			require.NoError(t, validateModels(era.valid, 8))
			for _, models := range era.invalid {
				var modelErr common.ProtocolParameterUpdateCostModelError
				require.ErrorAs(t, validateModels(models, 8), &modelErr)
			}
			require.NoError(t, validateModels(map[uint][]int64{99: nil}, 9))
		})
	}
}
