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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// legacyModeConservationTx builds a batch whose top level spends a UTxO of
// 2,000,000 and pays out topOut, with one sub-transaction paying out subOut
// from no inputs. The batch balances when topOut+subOut is 2,000,000. The top
// level needs a Plutus script, which puts the transaction in legacy mode, only
// when legacy is true.
func legacyModeConservationTx(
	t *testing.T,
	legacy bool,
	topOut, subOut uint64,
) (*DijkstraTransaction, common.LedgerState) {
	t.Helper()
	v3 := common.PlutusV3Script{0x31}
	input, utxo := dijkstraScriptLockedInput(t, v3, 981)
	tx := dijkstraSubUtxoTopLevelTx(
		[]shelley.ShelleyTransactionInput{input},
		[]DijkstraTransactionOutput{dijkstraSubUtxoOutput(t, topOut)},
	)
	tx.Body.TxSubTransactions = cbor.NewSetType(
		[]DijkstraSubTransaction{{
			Body: DijkstraSubTransactionBody{
				TxOutputs: []DijkstraTransactionOutput{
					dijkstraSubUtxoOutput(t, subOut),
				},
			},
		}},
		true,
	)
	if legacy {
		tx.WitnessSet.WsPlutusV3Scripts = cbor.NewSetType(
			[]common.PlutusV3Script{v3},
			true,
		)
	}
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	return tx, ls
}

func TestDijkstraLegacyModeValueConservation(t *testing.T) {
	t.Parallel()
	pp := dijkstraGuardTestPParams()

	t.Run("legacy mode rejects an unbalanced top level", func(t *testing.T) {
		t.Parallel()
		tx, ls := legacyModeConservationTx(t, true, 1_000_000, 1_000_000)
		require.NoError(t, UtxoValidateValueNotConservedUtxo(tx, 0, ls, pp))
		err := UtxoValidateValueNotConservedLegacyModeUtxo(tx, 0, ls, pp)
		var target ValueNotConservedInLegacyModeError
		require.ErrorAs(t, err, &target)
		require.Equal(t, int64(2_000_000), target.Consumed.Int64())
		require.Equal(t, int64(1_000_000), target.Produced.Int64())
	})

	t.Run("legacy mode accepts a balanced top level", func(t *testing.T) {
		t.Parallel()
		tx, ls := legacyModeConservationTx(t, true, 2_000_000, 0)
		require.NoError(
			t,
			UtxoValidateValueNotConservedLegacyModeUtxo(tx, 0, ls, pp),
		)
	})

	t.Run("outside legacy mode the top level may rely on sub-transactions", func(t *testing.T) {
		t.Parallel()
		tx, ls := legacyModeConservationTx(t, false, 1_000_000, 1_000_000)
		require.NoError(
			t,
			UtxoValidateValueNotConservedLegacyModeUtxo(tx, 0, ls, pp),
		)
	})

	t.Run("a transaction without sub-transactions is left to the batch rule", func(t *testing.T) {
		t.Parallel()
		tx, ls := legacyModeConservationTx(t, true, 1_000_000, 0)
		tx.Body.TxSubTransactions = cbor.SetType[DijkstraSubTransaction]{}
		require.NoError(
			t,
			UtxoValidateValueNotConservedLegacyModeUtxo(tx, 0, ls, pp),
		)
	})
}

func TestDijkstraLegacyModeValueConservationRegistration(t *testing.T) {
	t.Parallel()
	rule, legacyIdx := dijkstraValidationRule(
		t,
		"ledger/dijkstra.UtxoValidateValueNotConservedLegacyModeUtxo",
	)
	_, batchIdx := dijkstraValidationRule(
		t,
		"ledger/dijkstra.UtxoValidateValueNotConservedUtxo",
	)
	require.Less(t, batchIdx, legacyIdx)
	require.Equal(
		t,
		dijkstraUtxoValidationPhase2Valid,
		dijkstraUtxoValidationRulePhases[common.UtxoValidationRuleValueNotConservedLegacyMode],
	)

	tx, ls := legacyModeConservationTx(t, true, 1_000_000, 1_000_000)
	pp := dijkstraGuardTestPParams()
	var target ValueNotConservedInLegacyModeError
	require.ErrorAs(t, rule(tx, 0, ls, pp), &target)

	tx.TxIsValid = false
	require.NoError(t, rule(tx, 0, ls, pp))
}

// The top level is checked against the pool state the sub-transactions left,
// so a pool a sub-transaction registered owes the top level no deposit.
func TestDijkstraLegacyModeValueConservationPoolRegisteredBySubTransaction(
	t *testing.T,
) {
	t.Parallel()
	const poolDeposit = 500_000
	pp := dijkstraGuardTestPParams()
	pp.PoolDeposit = poolDeposit
	operator := common.PoolKeyHash{0x07}
	registration := common.CertificateWrapper{
		Type: uint(common.CertificateTypePoolRegistration),
		Certificate: &common.PoolRegistrationCertificate{
			CertType: uint(common.CertificateTypePoolRegistration),
			Operator: operator,
		},
	}

	build := func(t *testing.T, subRegisters bool) (*DijkstraTransaction, common.LedgerState) {
		t.Helper()
		tx, ls := legacyModeConservationTx(t, true, 2_000_000-poolDeposit, 0)
		tx.Body.TxCertificates = []common.CertificateWrapper{registration}
		if subRegisters {
			subTxs := tx.Body.TxSubTransactions.Items()
			subTxs[0].Body.TxCertificates = []common.CertificateWrapper{
				registration,
			}
			tx.Body.TxSubTransactions = cbor.NewSetType(subTxs, true)
		}
		return tx, ls
	}

	t.Run("deposit owed when no sub-transaction registers the pool", func(t *testing.T) {
		t.Parallel()
		tx, ls := build(t, false)
		require.NoError(
			t,
			UtxoValidateValueNotConservedLegacyModeUtxo(tx, 0, ls, pp),
		)
	})

	t.Run("no deposit owed when a sub-transaction registers the pool", func(t *testing.T) {
		t.Parallel()
		tx, ls := build(t, true)
		var target ValueNotConservedInLegacyModeError
		require.ErrorAs(
			t,
			UtxoValidateValueNotConservedLegacyModeUtxo(tx, 0, ls, pp),
			&target,
		)
		require.Equal(t, int64(2_000_000), target.Consumed.Int64())
		require.Equal(
			t,
			int64(2_000_000-poolDeposit),
			target.Produced.Int64(),
		)
	})
}
