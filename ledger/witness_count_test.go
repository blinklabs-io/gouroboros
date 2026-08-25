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

package ledger

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlockTransactionsRejectsMismatchedWitnessCounts(t *testing.T) {
	tests := []struct {
		name           string
		valid          common.Block
		missingWitness common.Block
		extraWitness   common.Block
	}{
		{
			name: "Shelley",
			valid: &ShelleyBlock{
				TransactionBodies:      []ShelleyTransactionBody{{}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{{}},
			},
			missingWitness: &ShelleyBlock{
				TransactionBodies: []ShelleyTransactionBody{{}, {}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{
					{},
				},
			},
			extraWitness: &ShelleyBlock{
				TransactionBodies: []ShelleyTransactionBody{{}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{
					{}, {},
				},
			},
		},
		{
			name: "Allegra",
			valid: &AllegraBlock{
				TransactionBodies:      []AllegraTransactionBody{{}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{{}},
			},
			missingWitness: &AllegraBlock{
				TransactionBodies: []AllegraTransactionBody{{}, {}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{
					{},
				},
			},
			extraWitness: &AllegraBlock{
				TransactionBodies: []AllegraTransactionBody{{}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{
					{}, {},
				},
			},
		},
		{
			name: "Mary",
			valid: &MaryBlock{
				TransactionBodies:      []MaryTransactionBody{{}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{{}},
			},
			missingWitness: &MaryBlock{
				TransactionBodies: []MaryTransactionBody{{}, {}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{
					{},
				},
			},
			extraWitness: &MaryBlock{
				TransactionBodies: []MaryTransactionBody{{}},
				TransactionWitnessSets: []ShelleyTransactionWitnessSet{
					{}, {},
				},
			},
		},
		{
			name: "Alonzo",
			valid: &AlonzoBlock{
				TransactionBodies:      []AlonzoTransactionBody{{}},
				TransactionWitnessSets: []AlonzoTransactionWitnessSet{{}},
			},
			missingWitness: &AlonzoBlock{
				TransactionBodies: []AlonzoTransactionBody{{}, {}},
				TransactionWitnessSets: []AlonzoTransactionWitnessSet{
					{},
				},
			},
			extraWitness: &AlonzoBlock{
				TransactionBodies: []AlonzoTransactionBody{{}},
				TransactionWitnessSets: []AlonzoTransactionWitnessSet{
					{}, {},
				},
			},
		},
		{
			name: "Babbage",
			valid: &BabbageBlock{
				TransactionBodies:      []BabbageTransactionBody{{}},
				TransactionWitnessSets: []BabbageTransactionWitnessSet{{}},
			},
			missingWitness: &BabbageBlock{
				TransactionBodies: []BabbageTransactionBody{{}, {}},
				TransactionWitnessSets: []BabbageTransactionWitnessSet{
					{},
				},
			},
			extraWitness: &BabbageBlock{
				TransactionBodies: []BabbageTransactionBody{{}},
				TransactionWitnessSets: []BabbageTransactionWitnessSet{
					{}, {},
				},
			},
		},
		{
			name: "Conway",
			valid: &ConwayBlock{
				TransactionBodies:      []ConwayTransactionBody{{}},
				TransactionWitnessSets: []ConwayTransactionWitnessSet{{}},
			},
			missingWitness: &ConwayBlock{
				TransactionBodies: []ConwayTransactionBody{{}, {}},
				TransactionWitnessSets: []ConwayTransactionWitnessSet{
					{},
				},
			},
			extraWitness: &ConwayBlock{
				TransactionBodies: []ConwayTransactionBody{{}},
				TransactionWitnessSets: []ConwayTransactionWitnessSet{
					{}, {},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Len(t, test.valid.Transactions(), 1)

			for _, mismatch := range []struct {
				name  string
				block common.Block
			}{
				{name: "missing witness", block: test.missingWitness},
				{name: "extra witness", block: test.extraWitness},
			} {
				t.Run(mismatch.name, func(t *testing.T) {
					var transactions []common.Transaction
					assert.NotPanics(t, func() {
						transactions = mismatch.block.Transactions()
					})
					assert.NotNil(t, transactions)
					assert.Empty(t, transactions)
				})
			}
		})
	}
}

func TestBlockConstructorsRejectMismatchedWitnessCounts(
	t *testing.T,
) {
	type blockConstructor func(
		[]byte,
		...common.VerifyConfig,
	) (common.Block, error)
	tests := []struct {
		name         string
		constructor  blockConstructor
		minRawLength int
	}{
		{
			name: "Shelley",
			constructor: func(
				data []byte,
				config ...common.VerifyConfig,
			) (common.Block, error) {
				return NewShelleyBlockFromCbor(data, config...)
			},
			minRawLength: 4,
		},
		{
			name: "Allegra",
			constructor: func(
				data []byte,
				config ...common.VerifyConfig,
			) (common.Block, error) {
				return NewAllegraBlockFromCbor(data, config...)
			},
			minRawLength: 4,
		},
		{
			name: "Mary",
			constructor: func(
				data []byte,
				config ...common.VerifyConfig,
			) (common.Block, error) {
				return NewMaryBlockFromCbor(data, config...)
			},
			minRawLength: 4,
		},
		{
			name: "Alonzo",
			constructor: func(
				data []byte,
				config ...common.VerifyConfig,
			) (common.Block, error) {
				return NewAlonzoBlockFromCbor(data, config...)
			},
			minRawLength: 5,
		},
		{
			name: "Babbage",
			constructor: func(
				data []byte,
				config ...common.VerifyConfig,
			) (common.Block, error) {
				return NewBabbageBlockFromCbor(data, config...)
			},
			minRawLength: 5,
		},
		{
			name: "Conway",
			constructor: func(
				data []byte,
				config ...common.VerifyConfig,
			) (common.Block, error) {
				return NewConwayBlockFromCbor(data, config...)
			},
			minRawLength: 5,
		},
	}
	skipConfig := common.VerifyConfig{SkipBodyHashValidation: true}

	// The count check lives in ExtractAndSetTransactionCbor, which every
	// decode runs, so a mismatch is rejected identically whether or not
	// body-hash validation is enabled. Both configurations are exercised to
	// keep that shared rejection path visible; the positive case can only use
	// the skipping config, because these synthetic blocks carry no header or
	// real body hash.
	configs := []struct {
		name   string
		config []common.VerifyConfig
	}{
		{name: "default config", config: nil},
		{name: "skip body hash", config: []common.VerifyConfig{skipConfig}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("matching counts", func(t *testing.T) {
				blockCbor := blockCborWithTransactionCounts(
					t,
					2,
					2,
					test.minRawLength,
				)
				block, err := test.constructor(blockCbor, skipConfig)
				require.NoError(t, err)
				require.Len(t, block.Transactions(), 2)
			})

			for _, mismatch := range []struct {
				name         string
				bodyCount    int
				witnessCount int
			}{
				{name: "missing witness", bodyCount: 2, witnessCount: 1},
				{name: "extra witness", bodyCount: 1, witnessCount: 2},
			} {
				t.Run(mismatch.name, func(t *testing.T) {
					for _, cfg := range configs {
						t.Run(cfg.name, func(t *testing.T) {
							blockCbor := blockCborWithTransactionCounts(
								t,
								mismatch.bodyCount,
								mismatch.witnessCount,
								test.minRawLength,
							)
							block, err := test.constructor(
								blockCbor,
								cfg.config...,
							)
							require.Error(t, err)
							assert.Nil(t, block)
							assert.ErrorContains(
								t,
								err,
								"transaction body and witness set counts do not match",
							)
						})
					}
				})
			}
		})
	}
}

func blockCborWithTransactionCounts(
	t *testing.T,
	bodyCount int,
	witnessCount int,
	minRawLength int,
) []byte {
	t.Helper()

	bodyItems := make([]cbor.RawMessage, bodyCount)
	for i := range bodyItems {
		bodyItems[i] = cbor.RawMessage{0xa0}
	}
	witnessItems := make([]cbor.RawMessage, witnessCount)
	for i := range witnessItems {
		witnessItems[i] = cbor.RawMessage{0xa0}
	}
	txBodies, err := cbor.Encode(bodyItems)
	require.NoError(t, err)
	txWitnesses, err := cbor.Encode(witnessItems)
	require.NoError(t, err)

	components := []cbor.RawMessage{
		{0xf6},
		txBodies,
		txWitnesses,
		{0xa0},
	}
	if minRawLength == 5 {
		components = append(components, cbor.RawMessage{0x80})
	}
	blockCbor, err := cbor.Encode(components)
	require.NoError(t, err)
	return blockCbor
}
