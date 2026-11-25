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
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// Dijkstra's ppuWellFormed (cardano-ledger 1210d37fe6bd
// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/PParams.hs 1150-1172), which
// GOV applies to every ParameterChange proposal, adds two conditions to
// Conway's: a set eMax is not zero, and a set pledge leverage cap is at
// least 1. Removing the cap is allowed.
func TestDijkstraParameterUpdateWellFormedEMaxAndPledgeLeverage(t *testing.T) {
	zero, one := uint(0), uint(1)
	ratio := func(n, d int64) *cbor.Rat { return &cbor.Rat{Rat: big.NewRat(n, d)} }
	pp := &DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: common.ProtocolVersionDijkstra,
			},
		},
	}
	for _, tc := range []struct {
		name   string
		update DijkstraProtocolParameterUpdate
		ok     bool
	}{
		{name: "eMax zero", update: DijkstraProtocolParameterUpdate{MaxEpoch: &zero}},
		{name: "eMax one", update: DijkstraProtocolParameterUpdate{MaxEpoch: &one}, ok: true},
		{name: "leverage one half", update: DijkstraProtocolParameterUpdate{MaxPledgeLeverage: ratio(1, 2)}},
		{name: "leverage one", update: DijkstraProtocolParameterUpdate{MaxPledgeLeverage: ratio(1, 1)}, ok: true},
		{name: "leverage three", update: DijkstraProtocolParameterUpdate{MaxPledgeLeverage: ratio(3, 1)}, ok: true},
		{name: "leverage cap removed", update: DijkstraProtocolParameterUpdate{MaxPledgeLeverageSet: true}, ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &DijkstraTransaction{
				TxIsValid: true,
				Body: DijkstraTransactionBody{
					TxSubTransactions: cbor.NewSetType([]DijkstraSubTransaction{{
						Body: DijkstraSubTransactionBody{
							TxProposalProcedures: []DijkstraProposalProcedure{{
								PPGovAction: DijkstraGovAction{
									Action: &DijkstraParameterChangeGovAction{
										Type:        uint(common.GovActionTypeParameterChange),
										ParamUpdate: tc.update,
									},
								},
							}},
						},
					}}, true),
				},
			}
			err := UtxoValidateProposalProcedures(
				tx, 0, mockledger.NewLedgerStateBuilder().Build(), pp,
			)
			if tc.ok {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}
