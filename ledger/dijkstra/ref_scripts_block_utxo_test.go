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
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// dijkstraAbsenceLedgerState reports an unknown input by wrapping
// common.ErrUtxoNotFound and any input in failing with its mapped error.
func dijkstraAbsenceLedgerState(
	failing map[string]error,
	utxos ...common.Utxo,
) common.LedgerState {
	byInput := make(map[string]common.Utxo, len(utxos))
	for _, utxo := range utxos {
		byInput[utxo.Id.String()] = utxo
	}
	return mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(input common.TransactionInput) (common.Utxo, error) {
			if err, ok := failing[input.String()]; ok {
				return common.Utxo{}, err
			}
			utxo, ok := byInput[input.String()]
			if !ok {
				return common.Utxo{}, fmt.Errorf(
					"%w: %s",
					common.ErrUtxoNotFound,
					input,
				)
			}
			return utxo, nil
		}).
		Build()
}

func dijkstraTxWithInputsAndFee(
	fee uint64,
	inputs []shelley.ShelleyTransactionInput,
	referenceInputs []shelley.ShelleyTransactionInput,
) *DijkstraTransaction {
	tx := dijkstraTxWithReferenceInputs(referenceInputs...)
	tx.Body.TxInputs = conway.NewConwayTransactionInputSet(inputs)
	tx.Body.TxFee = fee
	return tx
}

func dijkstraPParamsAtVersion(
	limit uint32,
	major uint,
) *DijkstraProtocolParameters {
	pp := &DijkstraProtocolParameters{MaxRefScriptSizePerBlock: limit}
	pp.ProtocolVersion.Major = major
	return pp
}

// The block rule only adds outputs to the UTxO from protocol version 11, so a
// spent input keeps resolving, a double spend counts twice, and an unknown
// input counts zero.
func TestValidateRefScriptSizePerBlockAccumulatesWithoutRemoval(t *testing.T) {
	t.Parallel()
	inputX, utxoX := dijkstraRefScriptInput(t, 0x01, 0, 100)
	missing, _ := dijkstraRefScriptInput(t, 0x02, 0, 100)
	ls := dijkstraAbsenceLedgerState(nil, utxoX)
	spendThenReference := dijkstraBlockWithTransactions(
		dijkstraTxWithInputsAndFee(
			1,
			[]shelley.ShelleyTransactionInput{inputX},
			nil,
		),
		dijkstraTxWithInputsAndFee(
			2,
			nil,
			[]shelley.ShelleyTransactionInput{inputX},
		),
	)
	doubleSpend := dijkstraBlockWithTransactions(
		dijkstraTxWithInputsAndFee(
			1,
			[]shelley.ShelleyTransactionInput{inputX},
			nil,
		),
		dijkstraTxWithInputsAndFee(
			2,
			[]shelley.ShelleyTransactionInput{inputX},
			nil,
		),
	)
	missingInput := dijkstraBlockWithTransactions(
		dijkstraTxWithInputsAndFee(
			1,
			nil,
			[]shelley.ShelleyTransactionInput{missing},
		),
		dijkstraTxWithInputsAndFee(
			2,
			nil,
			[]shelley.ShelleyTransactionInput{inputX},
		),
	)
	for _, major := range []uint{
		common.ProtocolVersionVanRossem,
		common.ProtocolVersionVanRossem - 1,
	} {
		for name, block := range map[string]*DijkstraBlock{
			"spend then reference": spendThenReference,
			"double spend":         doubleSpend,
		} {
			require.NoError(
				t,
				ValidateRefScriptSizePerBlock(
					block,
					dijkstraPParamsAtVersion(200, major),
					ls,
				),
				"%s pv%d at limit",
				name,
				major,
			)
			err := ValidateRefScriptSizePerBlock(
				block,
				dijkstraPParamsAtVersion(199, major),
				ls,
			)
			require.ErrorAs(
				t,
				err,
				&common.RefScriptSizePerBlockTooLargeError{},
				"%s pv%d over limit",
				name,
				major,
			)
		}
		require.NoError(
			t,
			ValidateRefScriptSizePerBlock(
				missingInput,
				dijkstraPParamsAtVersion(100, major),
				ls,
			),
			"missing input pv%d",
			major,
		)
	}
}

// A lookup failure other than absence is not a missing input: it must reach
// the caller rather than count as zero.
func TestValidateRefScriptSizePerBlockPropagatesLookupFailure(t *testing.T) {
	t.Parallel()
	failing, _ := dijkstraRefScriptInput(t, 0x02, 0, 100)
	lookupErr := errors.New("storage unavailable")
	ls := dijkstraAbsenceLedgerState(
		map[string]error{failing.String(): lookupErr},
	)
	block := dijkstraBlockWithTransactions(
		dijkstraTxWithReferenceInputs(failing),
	)
	for _, major := range []uint{
		common.ProtocolVersionVanRossem,
		common.ProtocolVersionVanRossem - 1,
	} {
		err := ValidateRefScriptSizePerBlock(
			block,
			dijkstraPParamsAtVersion(1000, major),
			ls,
		)
		require.ErrorIs(t, err, lookupErr, "pv%d", major)
	}
}
