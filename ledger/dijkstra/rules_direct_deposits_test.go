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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const (
	dijkstraDepositFunding = uint64(1_200_000)
	dijkstraDepositAmount  = uint64(500_000_000)
)

// dijkstraDepositInput resolves to a UTxO holding exactly amount, so a
// conservation failure's consumed total is the funding under test.
func dijkstraDepositInput(amount uint64) (
	shelley.ShelleyTransactionInput,
	common.Utxo,
) {
	input := shelley.NewShelleyTransactionInput(
		"1111111111111111111111111111111111111111111111111111111111111111",
		0,
	)
	return input, common.Utxo{
		Id: input,
		Output: babbage.BabbageTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{Amount: amount},
		},
	}
}

func dijkstraDepositAccount(fill byte) cbor.ByteString {
	// A reward address: header byte plus a 28-byte staking key hash.
	raw := append(
		[]byte{0xe0},
		bytes.Repeat([]byte{fill}, common.AddressHashSize)...)
	return cbor.NewByteString(raw)
}

// TestDijkstraDirectDepositsCountAsProducedValue pins that a direct deposit
// (body key 25) is produced value: a 500 ADA deposit funded by a 1.2 ADA input
// is rejected with the same consumed and produced totals as the same 500 ADA
// paid to an output, which is how the rules already treat an ordinary value
// imbalance.
func TestDijkstraDirectDepositsCountAsProducedValue(t *testing.T) {
	input, utxo := dijkstraDepositInput(dijkstraDepositFunding)
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleValueNotConserved)

	depositTx := dijkstraSubUtxoTopLevelTx(
		[]shelley.ShelleyTransactionInput{input},
		nil,
	)
	depositTx.Body.TxDirectDeposits = map[cbor.ByteString]uint64{
		dijkstraDepositAccount(0x11): dijkstraDepositAmount,
	}
	var depositErr shelley.ValueNotConservedUtxoError
	require.ErrorAs(t, rule(depositTx, 0, ls, pp), &depositErr)
	require.NotNil(t, depositErr.Consumed)
	require.NotNil(t, depositErr.Produced)
	require.Equal(
		t,
		new(big.Int).SetUint64(dijkstraDepositFunding),
		depositErr.Consumed,
	)
	require.Equal(
		t,
		new(big.Int).SetUint64(dijkstraDepositAmount),
		depositErr.Produced,
	)

	outputTx := dijkstraSubUtxoTopLevelTx(
		[]shelley.ShelleyTransactionInput{input},
		[]DijkstraTransactionOutput{
			dijkstraSubUtxoOutput(t, dijkstraDepositAmount),
		},
	)
	var outputErr shelley.ValueNotConservedUtxoError
	require.ErrorAs(t, rule(outputTx, 0, ls, pp), &outputErr)
	require.Equal(t, outputErr, depositErr)
}

// TestDijkstraDirectDepositsConserveWhenFunded keeps the deposit rejection
// conditional: a deposit matched by its input is accepted, and the deposit
// does not double-count against the proposal deposits and treasury donations
// the batch view already folds.
func TestDijkstraDirectDepositsConserveWhenFunded(t *testing.T) {
	input, utxo := dijkstraDepositInput(dijkstraDepositAmount)
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleValueNotConserved)

	tx := dijkstraSubUtxoTopLevelTx(
		[]shelley.ShelleyTransactionInput{input},
		nil,
	)
	tx.Body.TxDirectDeposits = map[cbor.ByteString]uint64{
		dijkstraDepositAccount(0x11): dijkstraDepositAmount,
	}
	require.NoError(t, rule(tx, 0, ls, pp))

	withDonation := dijkstraSubUtxoTopLevelTx(
		[]shelley.ShelleyTransactionInput{input},
		nil,
	)
	withDonation.Body.TxDirectDeposits = map[cbor.ByteString]uint64{
		dijkstraDepositAccount(0x11): dijkstraDepositAmount - 1,
	}
	withDonation.Body.TxDonation = 1
	require.NoError(t, rule(withDonation, 0, ls, pp))
}

// TestDijkstraDirectDepositsFoldAcrossTransactionLevels pins that a deposit
// hidden in a sub-transaction is counted exactly like the same deposit at the
// top level, the way cardano-ledger's localProducedValue applies to every
// body in the batch.
func TestDijkstraDirectDepositsFoldAcrossTransactionLevels(t *testing.T) {
	input, utxo := dijkstraDepositInput(dijkstraDepositFunding)
	ls := mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{utxo}).
		Build()
	pp := &DijkstraProtocolParameters{}
	rule := dijkstraRule(t, common.UtxoValidationRuleValueNotConserved)

	deposits := map[cbor.ByteString]uint64{
		dijkstraDepositAccount(0x11): dijkstraDepositAmount,
	}
	topTx := dijkstraSubUtxoTopLevelTx(
		[]shelley.ShelleyTransactionInput{input},
		nil,
	)
	topTx.Body.TxDirectDeposits = deposits
	var topErr shelley.ValueNotConservedUtxoError
	require.ErrorAs(t, rule(topTx, 0, ls, pp), &topErr)
	require.NotNil(t, topErr.Produced)
	require.Equal(
		t,
		new(big.Int).SetUint64(dijkstraDepositAmount),
		topErr.Produced,
	)

	subTx := dijkstraSingleSubTx(DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{TxDirectDeposits: deposits},
	})
	subTx.Body.TxInputs = conway.NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input},
	)
	var subErr shelley.ValueNotConservedUtxoError
	require.ErrorAs(t, rule(subTx, 0, ls, pp), &subErr)
	require.NotNil(t, subErr.Produced)
	require.Equal(t, topErr, subErr)
}
