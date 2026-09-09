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

package dijkstra_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/ledgertest"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
)

// TestUtxoValidateValueNotConservedUtxoPoolDeposits runs the shared pool
// deposit cases against Dijkstra's own registered rule. Dijkstra delegates the
// rule body to Conway, so this pins the delegation and the protocol parameter
// conversion, which is the part Dijkstra owns.
func TestUtxoValidateValueNotConservedUtxoPoolDeposits(t *testing.T) {
	ledgertest.RunPoolDepositRuleCases(t, ledgertest.PoolDepositRuleFixture{
		Era:         "dijkstra",
		Rules:       dijkstra.UtxoValidationRules,
		Descriptors: dijkstra.UtxoValidationRuleDescriptors,
		Pparams: &dijkstra.DijkstraProtocolParameters{
			ConwayProtocolParameters: conway.ConwayProtocolParameters{
				PoolDeposit: uint(ledgertest.PoolDepositAmount),
			},
		},
		NewTx: func(
			outputAmount uint64,
			certs []common.CertificateWrapper,
		) common.Transaction {
			return &dijkstra.DijkstraTransaction{
				Body: dijkstra.DijkstraTransactionBody{
					TxFee: ledgertest.PoolDepositTxFee,
					TxInputs: conway.NewConwayTransactionInputSet(
						ledgertest.PoolDepositInputs(),
					),
					TxOutputs: []dijkstra.DijkstraTransactionOutput{
						{
							Output: &babbage.BabbageTransactionOutput{
								OutputAmount: mary.MaryTransactionOutputValue{
									Amount: outputAmount,
								},
							},
						},
					},
					TxCertificates: certs,
				},
			}
		},
	})
}
