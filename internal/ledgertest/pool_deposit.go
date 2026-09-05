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

package ledgertest

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// Fixed values shared by every era's pool deposit rule cases. The input is
// large enough to fund two pool deposits plus the fee, so a case may budget
// one deposit more than the rule should charge without underflowing.
const (
	PoolDepositTxFee   uint64 = 123_456
	PoolDepositAmount  uint64 = 500_000_000
	poolDepositInput   uint64 = 3_000_000_000
	poolDepositTxId           = "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
	poolDepositRetired uint64 = 197
)

// poolDepositEpochLedgerState adds the optional common.EpochState capability to
// a ledger state. common.PoolRegistrationDepositDue needs the current epoch to
// decide whether a pending retirement has already taken effect.
type poolDepositEpochLedgerState struct {
	common.LedgerState
	epoch uint64
}

func (s poolDepositEpochLedgerState) EpochForSlot(uint64) (uint64, error) {
	return s.epoch, nil
}

var _ common.EpochState = poolDepositEpochLedgerState{}

// PoolDepositInputs returns the transaction inputs every era's fixture spends.
// Conway and Dijkstra wrap them in a ConwayTransactionInputSet; the earlier
// eras use shelley.NewShelleyTransactionInputSet.
func PoolDepositInputs() []shelley.ShelleyTransactionInput {
	return []shelley.ShelleyTransactionInput{
		shelley.NewShelleyTransactionInput(poolDepositTxId, 0),
	}
}

// PoolDepositRuleFixture supplies the era-specific pieces of the pool deposit
// rule cases: the era's registered rules, its protocol parameters, and a
// builder for a transaction of its own type.
type PoolDepositRuleFixture struct {
	// Era names the era under test, for failure messages.
	Era string
	// Rules and Descriptors are the era's own authoritative rule list. The
	// validator is resolved from them by id rather than named directly, so a
	// rule dropped from an era's production wiring fails here.
	Rules       []common.UtxoValidationRuleFunc
	Descriptors func() []common.UtxoValidationRuleDescriptor
	// Pparams must carry PoolDeposit set to PoolDepositAmount.
	Pparams common.ProtocolParameters
	// NewTx builds a transaction of the era's own type spending
	// PoolDepositInputs, paying PoolDepositTxFee, with a single output of
	// outputAmount and the given certificates.
	NewTx func(
		outputAmount uint64,
		certs []common.CertificateWrapper,
	) common.Transaction
}

// RunPoolDepositRuleCases drives an era's registered value-not-conserved rule
// and pins how far the produced value moves for pool registration
// certificates.
//
// The unit tests in ledger/common prove the common.PoolRegistrationDepositDue
// predicate. These cases prove what an era's rule does with it. Each case
// declares how many deposits the rule must add, budgets exactly that many and
// requires conservation, then budgets one deposit either side and requires the
// consumed/produced gap to be exactly one deposit. Bounding the rule from both
// sides makes "the produced value moves by exactly one deposit" an assertion
// rather than a restatement of the implementation.
//
// The rule is duplicated per era and each copy is independently revertible, so
// every era that carries it runs these cases from its own package. Allegra
// delegates to Shelley and Dijkstra to Conway; those rows pin the delegation
// itself, which is the only part those eras own.
//
// The two-certificate cases exercise the per-transaction dedup. POOL applies
// certificates in order, and cardano-ledger's shelleyTotalDepositsTxBody
// counts a Set of not-yet-registered pool ids, so a second certificate for a
// pool the first one just registered incurs nothing.
func RunPoolDepositRuleCases(t *testing.T, f PoolDepositRuleFixture) {
	t.Helper()
	rule := poolDepositRule(t, f)
	exactAmount := poolDepositInput - PoolDepositTxFee
	operator := common.PoolKeyHash(
		common.NewBlake2b224(
			bytes.Repeat([]byte{0x42}, common.Blake2b224Size),
		),
	)
	onRecord := &common.PoolRegistrationCertificate{Operator: operator}
	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(poolDepositTxId, 0),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: poolDepositInput,
			},
		},
	}
	// newTx budgets budgetedDeposits pool deposits out of the output and
	// carries the given number of registration certificates, all for the same
	// operator.
	newTx := func(
		budgetedDeposits uint64,
		registrations int,
	) common.Transaction {
		certs := make([]common.CertificateWrapper, 0, registrations)
		for range registrations {
			certs = append(certs, common.CertificateWrapper{
				Type: uint(common.CertificateTypePoolRegistration),
				Certificate: &common.PoolRegistrationCertificate{
					Operator: operator,
				},
			})
		}
		return f.NewTx(exactAmount-budgetedDeposits*PoolDepositAmount, certs)
	}
	newLedgerState := func(
		reg *common.PoolRegistrationCertificate,
		retirement *uint64,
	) common.LedgerState {
		return poolDepositEpochLedgerState{
			LedgerState: mockledger.NewLedgerStateBuilder().
				WithUtxos(utxos).
				WithPoolCurrentState(
					func(common.PoolKeyHash) (*common.PoolRegistrationCertificate, *uint64, error) {
						return reg, retirement, nil
					},
				).Build(),
			epoch: poolDepositRetired,
		}
	}
	retiresAt := func(epoch uint64) *uint64 { return &epoch }

	for _, tc := range []struct {
		name string
		// reg and retire are what PoolCurrentState reports for the operator.
		reg    *common.PoolRegistrationCertificate
		retire *uint64
		// registrations is how many registration certificates for that
		// operator the transaction carries.
		registrations int
		// wantDeposits is how many pool deposits the rule must add to the
		// produced value.
		wantDeposits uint64
	}{
		{
			name:          "unregistered pool",
			registrations: 1,
			wantDeposits:  1,
		},
		{
			name:          "live pool is a parameter update",
			reg:           onRecord,
			registrations: 1,
			wantDeposits:  0,
		},
		{
			name:          "retirement still pending is a parameter update",
			reg:           onRecord,
			retire:        retiresAt(poolDepositRetired + 1),
			registrations: 1,
			wantDeposits:  0,
		},
		{
			name:          "retirement has taken effect",
			reg:           onRecord,
			retire:        retiresAt(poolDepositRetired),
			registrations: 1,
			wantDeposits:  1,
		},
		{
			name:          "two registrations for an unregistered pool",
			registrations: 2,
			wantDeposits:  1,
		},
		{
			name:          "two registrations for a retired pool",
			reg:           onRecord,
			retire:        retiresAt(poolDepositRetired),
			registrations: 2,
			wantDeposits:  1,
		},
		{
			name:          "two registrations for a live pool",
			reg:           onRecord,
			registrations: 2,
			wantDeposits:  0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ls := newLedgerState(tc.reg, tc.retire)
			// Budgeting exactly the expected deposits must conserve value.
			require.NoError(
				t,
				rule(newTx(tc.wantDeposits, tc.registrations), 0, ls, f.Pparams),
				"%s: budgeting %d pool deposit(s) should conserve value",
				f.Era,
				tc.wantDeposits,
			)
			// Budgeting one deposit either side must miss by exactly one
			// deposit, which bounds the rule above and below.
			budgets := []uint64{tc.wantDeposits + 1}
			if tc.wantDeposits > 0 {
				budgets = append(budgets, tc.wantDeposits-1)
			}
			for _, budget := range budgets {
				err := rule(
					newTx(budget, tc.registrations),
					0,
					ls,
					f.Pparams,
				)
				var notConserved shelley.ValueNotConservedUtxoError
				require.ErrorAs(
					t,
					err,
					&notConserved,
					"%s: budgeting %d pool deposit(s) should not conserve value",
					f.Era,
					budget,
				)
				// Consumed minus produced isolates the deposit difference.
				// The budget is one deposit either side of wantDeposits, so
				// the gap is one deposit, signed by which side it is on.
				want := new(big.Int).SetUint64(PoolDepositAmount)
				if budget < tc.wantDeposits {
					want.Neg(want)
				}
				gap := new(big.Int).Sub(
					notConserved.Consumed,
					notConserved.Produced,
				)
				require.Equal(
					t,
					want.String(),
					gap.String(),
					"%s: budgeting %d pool deposit(s): the rule added the wrong number of deposits",
					f.Era,
					budget,
				)
			}
		})
	}
}

// poolDepositRule returns the era's value-not-conserved validator as it is
// actually registered. Resolving it through the descriptors rather than naming
// the function keeps the cases on the code path a consumer runs, so an
// unregistered or misidentified rule fails here.
func poolDepositRule(
	t *testing.T,
	f PoolDepositRuleFixture,
) common.UtxoValidationRuleFunc {
	t.Helper()
	descriptors := f.Descriptors()
	require.Len(t, f.Rules, len(descriptors))
	for idx, descriptor := range descriptors {
		if descriptor.Id == common.UtxoValidationRuleValueNotConserved {
			return f.Rules[idx]
		}
	}
	t.Fatalf(
		"%s validation rule %q is not registered",
		f.Era,
		common.UtxoValidationRuleValueNotConserved,
	)
	return nil
}
