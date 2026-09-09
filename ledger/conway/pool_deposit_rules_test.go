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

package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/ledgertest"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
)

// TestUtxoValidateValueNotConservedUtxoPoolDeposits runs the shared pool
// deposit cases against Conway's own registered rule. Conway carries its own
// copy of the rule body, the one Dijkstra delegates to, so reverting its call
// site to the registration on record, or dropping its per-transaction dedup,
// fails here.
func TestUtxoValidateValueNotConservedUtxoPoolDeposits(t *testing.T) {
	ledgertest.RunPoolDepositRuleCases(t, ledgertest.PoolDepositRuleFixture{
		Era:         "conway",
		Rules:       conway.UtxoValidationRules,
		Descriptors: conway.UtxoValidationRuleDescriptors,
		Pparams: &conway.ConwayProtocolParameters{
			PoolDeposit: uint(ledgertest.PoolDepositAmount),
		},
		NewTx: func(
			outputAmount uint64,
			certs []common.CertificateWrapper,
		) common.Transaction {
			return &conway.ConwayTransaction{
				Body: conway.ConwayTransactionBody{
					TxFee: ledgertest.PoolDepositTxFee,
					TxInputs: conway.NewConwayTransactionInputSet(
						ledgertest.PoolDepositInputs(),
					),
					TxOutputs: []babbage.BabbageTransactionOutput{
						{OutputAmount: mary.MaryTransactionOutputValue{
							Amount: outputAmount,
						}},
					},
					TxCertificates: certs,
				},
			}
		},
	})
}
