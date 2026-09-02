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

package ledger_test

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

func dijkstraBlockLimitWitnesses(
	exUnits common.ExUnits,
) dijkstra.DijkstraTransactionWitnessSet {
	return dijkstra.DijkstraTransactionWitnessSet{
		WsRedeemers: dijkstra.DijkstraRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagGuarding, Index: 0}: {
					Data: common.Datum{
						Data: data.NewInteger(big.NewInt(0)),
					},
					ExUnits: exUnits,
				},
			},
		},
	}
}

func dijkstraBlockLimitExUnitsTx(
	topLevel common.ExUnits,
	subtransaction common.ExUnits,
) dijkstra.DijkstraTransaction {
	return dijkstra.DijkstraTransaction{
		Body: dijkstra.DijkstraTransactionBody{
			TxSubTransactions: cbor.NewSetType(
				[]dijkstra.DijkstraSubTransaction{{
					WitnessSet: dijkstraBlockLimitWitnesses(subtransaction),
				}},
				true,
			),
		},
		WitnessSet: dijkstraBlockLimitWitnesses(topLevel),
		TxIsValid:  true,
	}
}

func buildDijkstraLimitsTestBlock(
	t *testing.T,
	txs []dijkstra.DijkstraTransaction,
) ledger.Block {
	t.Helper()
	headerCborBytes, err := hex.DecodeString(blockLimitsTestHeaderHex)
	require.NoError(t, err)
	header, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeDijkstra,
		headerCborBytes,
	)
	require.NoError(t, err)
	dijkstraHeader, ok := header.(*dijkstra.DijkstraBlockHeader)
	require.True(t, ok)

	crafted := &dijkstra.DijkstraBlock{
		BlockHeader: dijkstraHeader,
		BlockBody: dijkstra.DijkstraBlockBody{
			Transactions: txs,
		},
	}
	blockCbor, err := cbor.Encode(crafted)
	require.NoError(t, err)
	decoded, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeDijkstra,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return decoded
}

func TestVerifyBlockDijkstraExUnitsIncludesEveryTransactionLevel(
	t *testing.T,
) {
	block := buildDijkstraLimitsTestBlock(
		t,
		[]dijkstra.DijkstraTransaction{
			dijkstraBlockLimitExUnitsTx(
				common.ExUnits{Memory: 5, Steps: 7},
				common.ExUnits{Memory: 11, Steps: 13},
			),
			dijkstraBlockLimitExUnitsTx(
				common.ExUnits{Memory: 17, Steps: 19},
				common.ExUnits{Memory: 23, Steps: 29},
			),
		},
	)
	wantTotal := common.ExUnits{Memory: 56, Steps: 68}
	tests := []struct {
		name      string
		max       common.ExUnits
		wantError bool
	}{
		{name: "below limit", max: common.ExUnits{Memory: 57, Steps: 69}},
		{name: "at limit", max: wantTotal},
		{
			name:      "over limit",
			max:       common.ExUnits{Memory: 55, Steps: 67},
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &dijkstra.DijkstraProtocolParameters{
				ConwayProtocolParameters: conway.ConwayProtocolParameters{
					MaxBlockExUnits: test.max,
				},
			}
			valid, _, _, _, err := ledger.VerifyBlock(
				block,
				blockLimitsTestEta0Hex,
				blockLimitsTestSlotsPerKesPeriod,
				common.VerifyConfig{
					SkipBodyHashValidation:    true,
					SkipTransactionValidation: true,
					SkipStakePoolValidation:   true,
					ProtocolParameters:        pp,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.False(t, valid)
			var target common.BlockExUnitsTooBigError
			require.ErrorAs(t, err, &target)
			require.Equal(t, wantTotal, target.TotalExUnits)
		})
	}
}

func dijkstraBlockLimitScript(t *testing.T) common.PlutusV4Script {
	t.Helper()
	// lang.LanguageVersionV4 identifies the ledger language, not the UPLC
	// program version, which must be 1.0.0 or 1.1.0.
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{
		Version: [3]uint32{1, 1, 0},
		Term:    &syn.Error{},
	})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return common.PlutusV4Script(wrapper)
}
