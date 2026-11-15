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
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// A phase-2-invalid transaction does not change ledger state, so the LEDGER
// rule skips value conservation and the cross-level duplicate-input check
// (sub-transactions leave the UTxO untouched, so nothing is spent twice).

func TestDijkstraValueConservationSkippedForPhase2InvalidTransactions(
	t *testing.T,
) {
	t.Parallel()
	input, utxo := dijkstraSubUtxoInput(0)
	inputs := []shelley.ShelleyTransactionInput{input}
	outputs := []DijkstraTransactionOutput{
		dijkstraSubUtxoOutput(t, dijkstraSubUtxoOverAmount),
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := productionRule(t, common.UtxoValidationRuleValueNotConserved)

	for name, build := range map[string]func() *DijkstraTransaction{
		"top-level": func() *DijkstraTransaction {
			return dijkstraSubUtxoTopLevelTx(inputs, outputs)
		},
		"sub-transaction": func() *DijkstraTransaction {
			return dijkstraSubUtxoSubTx(inputs, outputs)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tx := build()
			tx.TxIsValid = true
			var target shelley.ValueNotConservedUtxoError
			require.ErrorAs(t, rule(tx, 0, ls, pp), &target)

			tx = build()
			tx.TxIsValid = false
			require.NoError(t, rule(tx, 0, ls, pp))
		})
	}
}

func TestDijkstraDuplicateInputsSkippedForPhase2InvalidTransactions(
	t *testing.T,
) {
	t.Parallel()
	input, utxo := dijkstraSubUtxoInput(0)
	inputs := []shelley.ShelleyTransactionInput{input}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := productionRule(t, common.UtxoValidationRuleNoDuplicateInputs)

	build := func(isValid bool) *DijkstraTransaction {
		tx := dijkstraSubUtxoSubTx(inputs, nil)
		tx.Body.TxInputs = conway.NewConwayTransactionInputSet(inputs)
		tx.TxIsValid = isValid
		return tx
	}
	var target shelley.DuplicateInputError
	require.ErrorAs(t, rule(build(true), 0, ls, pp), &target)
	require.NoError(t, rule(build(false), 0, ls, pp))
}

// A treasury donation at a level whose needed scripts include Plutus V1 or V2
// fails script-context translation in cardano-ledger, and that failure is
// raised for phase-2-invalid transactions too, so it must not be skipped with
// value conservation.
func TestDijkstraDonationScriptCheckRunsForPhase2InvalidTransactions(
	t *testing.T,
) {
	t.Parallel()
	input, utxo := dijkstraSubUtxoInput(0)
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	v1Script := common.PlutusV1Script{0x41, 0}
	mint := common.NewMultiAsset[common.MultiAssetTypeMint](
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{
			v1Script.Hash(): {
				cbor.NewByteString([]byte("asset")): big.NewInt(1),
			},
		},
	)
	newTx := func(isValid bool) *DijkstraTransaction {
		return &DijkstraTransaction{
			Body: DijkstraTransactionBody{
				TxInputs: conway.NewConwayTransactionInputSet(
					[]shelley.ShelleyTransactionInput{input},
				),
				TxMint:     &mint,
				TxDonation: dijkstraSubUtxoInputAmount,
			},
			WitnessSet: DijkstraTransactionWitnessSet{
				WsPlutusV1Scripts: cbor.NewSetType(
					[]common.PlutusV1Script{v1Script}, false,
				),
			},
			TxIsValid: isValid,
		}
	}
	rejected := func(isValid bool) bool {
		for _, rule := range UtxoValidationRules {
			var target conway.TreasuryDonationWithPlutusV1V2Error
			if errors.As(rule(newTx(isValid), 0, ls, pp), &target) {
				return true
			}
		}
		return false
	}
	require.True(t, rejected(true))
	require.True(t, rejected(false))
}
