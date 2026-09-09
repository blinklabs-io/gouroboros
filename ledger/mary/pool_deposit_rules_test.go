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

package mary_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/ledgertest"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// TestUtxoValidateValueNotConservedUtxoPoolDeposits runs the shared pool
// deposit cases against Mary's own registered rule. Mary carries its own copy
// of the rule body, so reverting its call site to the registration on record,
// or dropping its per-transaction dedup, fails here.
func TestUtxoValidateValueNotConservedUtxoPoolDeposits(t *testing.T) {
	ledgertest.RunPoolDepositRuleCases(t, ledgertest.PoolDepositRuleFixture{
		Era:         "mary",
		Rules:       mary.UtxoValidationRules,
		Descriptors: mary.UtxoValidationRuleDescriptors,
		Pparams: &mary.MaryProtocolParameters{
			PoolDeposit: uint(ledgertest.PoolDepositAmount),
		},
		NewTx: func(
			outputAmount uint64,
			certs []common.CertificateWrapper,
		) common.Transaction {
			return &mary.MaryTransaction{
				Body: mary.MaryTransactionBody{
					TxFee: ledgertest.PoolDepositTxFee,
					TxInputs: shelley.NewShelleyTransactionInputSet(
						ledgertest.PoolDepositInputs(),
					),
					TxOutputs: []mary.MaryTransactionOutput{
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
