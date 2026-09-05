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

package allegra_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/ledgertest"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// TestUtxoValidateValueNotConservedUtxoPoolDeposits runs the shared pool
// deposit cases against Allegra's own registered rule. Allegra delegates the
// rule body to Shelley, so this pins the delegation and the protocol parameter
// conversion, which is the part Allegra owns.
func TestUtxoValidateValueNotConservedUtxoPoolDeposits(t *testing.T) {
	ledgertest.RunPoolDepositRuleCases(t, ledgertest.PoolDepositRuleFixture{
		Era:         "allegra",
		Rules:       allegra.UtxoValidationRules,
		Descriptors: allegra.UtxoValidationRuleDescriptors,
		Pparams: &allegra.AllegraProtocolParameters{
			PoolDeposit: uint(ledgertest.PoolDepositAmount),
		},
		NewTx: func(
			outputAmount uint64,
			certs []common.CertificateWrapper,
		) common.Transaction {
			return &allegra.AllegraTransaction{
				Body: allegra.AllegraTransactionBody{
					TxFee: ledgertest.PoolDepositTxFee,
					TxInputs: shelley.NewShelleyTransactionInputSet(
						ledgertest.PoolDepositInputs(),
					),
					TxOutputs: []shelley.ShelleyTransactionOutput{
						{OutputAmount: outputAmount},
					},
					TxCertificates: certs,
				},
			}
		},
	})
}
