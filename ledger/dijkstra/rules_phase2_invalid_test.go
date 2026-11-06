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
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// productionRule returns the entry of UtxoValidationRules registered for id,
// so a test exercises the phase wrapper rather than the bare validator.
func productionRule(
	t *testing.T,
	id common.UtxoValidationRuleId,
) common.UtxoValidationRuleFunc {
	t.Helper()
	return UtxoValidationRules[dijkstraValidationRuleIndex(t, id)]
}

func TestDijkstraBatchChecksRunForPhase2InvalidTransactions(t *testing.T) {
	t.Parallel()
	for _, id := range []common.UtxoValidationRuleId{
		common.UtxoValidationRuleCurrentTreasuryValue,
		common.UtxoValidationRuleRefScriptSizePerTx,
	} {
		require.Equalf(
			t,
			dijkstraUtxoValidationAlways,
			dijkstraUtxoValidationRulePhases[id],
			"rule %q runs before the Phase2Valid branch",
			id,
		)
	}
}

func TestDijkstraRefScriptSizePerTxAppliesToPhase2InvalidTransactions(
	t *testing.T,
) {
	t.Parallel()
	rule := productionRule(t, common.UtxoValidationRuleRefScriptSizePerTx)
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 10}

	t.Run("top-level reference input", func(t *testing.T) {
		t.Parallel()
		input, utxo := dijkstraRefScriptInput(t, 0x01, 0, 100)
		ls := dijkstraRefScriptLedgerState(t, utxo)
		for _, isValid := range []bool{true, false} {
			tx := dijkstraTxWithReferenceInputs(input)
			tx.TxIsValid = isValid
			err := rule(tx, 0, ls, pp)
			var target common.RefScriptSizePerTxTooLargeError
			require.ErrorAsf(t, err, &target, "isValid=%v", isValid)
			require.Equal(t, uint64(100), target.TxSize)
			require.Equal(t, uint64(10), target.MaxSize)
		}
	})

	t.Run("sum over top-level and sub-transactions", func(t *testing.T) {
		t.Parallel()
		inputA, utxoA := dijkstraRefScriptInput(t, 0x01, 0, 60)
		inputB, utxoB := dijkstraRefScriptInput(t, 0x02, 0, 60)
		ls := dijkstraRefScriptLedgerState(t, utxoA, utxoB)
		tx := dijkstraTxWithReferenceInputs(inputA)
		tx.TxIsValid = false
		tx.Body.TxSubTransactions = cbor.NewSetType(
			[]DijkstraSubTransaction{{
				Body: DijkstraSubTransactionBody{
					TxReferenceInputs: cbor.NewSetType(
						[]shelley.ShelleyTransactionInput{inputB},
						false,
					),
				},
			}},
			false,
		)
		limit := &DijkstraProtocolParameters{MaxRefScriptSizePerTx: 119}
		var target common.RefScriptSizePerTxTooLargeError
		require.ErrorAs(t, rule(tx, 0, ls, limit), &target)
		require.Equal(t, uint64(120), target.TxSize)
		limit.MaxRefScriptSizePerTx = 120
		require.NoError(t, rule(tx, 0, ls, limit))
	})
}

func TestDijkstraCurrentTreasuryValueAppliesToPhase2InvalidTransactions(
	t *testing.T,
) {
	t.Parallel()
	rule := productionRule(t, common.UtxoValidationRuleCurrentTreasuryValue)
	ls := mockledger.NewLedgerStateBuilder().WithTreasuryAmount(1000).Build()

	t.Run("top-level mismatch", func(t *testing.T) {
		t.Parallel()
		for _, isValid := range []bool{true, false} {
			tx := &DijkstraTransaction{
				Body: decodeDijkstraTreasuryBody(
					t,
					dijkstraTreasuryValue(999),
				),
				TxIsValid: isValid,
			}
			var target common.CurrentTreasuryValueMismatchError
			require.ErrorAsf(
				t,
				rule(tx, 0, ls, &DijkstraProtocolParameters{}),
				&target,
				"isValid=%v",
				isValid,
			)
			require.Equal(t, big.NewInt(999), target.Supplied)
			require.Equal(t, uint64(1000), target.Expected)
		}
	})

	t.Run("sub-transaction mismatch", func(t *testing.T) {
		t.Parallel()
		body := decodeDijkstraTreasuryBody(t, nil)
		body.TxSubTransactions = cbor.NewSetType(
			[]DijkstraSubTransaction{
				decodeDijkstraTreasurySubTransaction(
					t,
					dijkstraTreasuryValue(999),
				),
			},
			true,
		)
		tx := &DijkstraTransaction{Body: body, TxIsValid: false}
		var target common.CurrentTreasuryValueMismatchError
		require.ErrorAs(
			t,
			rule(tx, 0, ls, &DijkstraProtocolParameters{}),
			&target,
		)
		require.Equal(t, big.NewInt(999), target.Supplied)
	})

	t.Run("matching and absent values pass", func(t *testing.T) {
		t.Parallel()
		for _, value := range []*uint64{dijkstraTreasuryValue(1000), nil} {
			tx := &DijkstraTransaction{
				Body:      decodeDijkstraTreasuryBody(t, value),
				TxIsValid: false,
			}
			require.NoError(
				t,
				rule(tx, 0, ls, &DijkstraProtocolParameters{}),
			)
		}
	})
}
