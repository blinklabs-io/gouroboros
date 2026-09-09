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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// epochAware adds the optional EpochState capability to a ledger state, which
// the mock does not provide on its own.
type epochAware struct {
	common.LedgerState
	epoch uint64
}

func (e epochAware) EpochForSlot(uint64) (uint64, error) { return e.epoch, nil }

// TestPoolRegistrationDepositDueAfterRetirement is the blinklabs-io/dingo#3908
// regression.
//
// A pool whose retirement has taken effect is not currently registered, so
// registering it again incurs the deposit. PoolCurrentState still returns the
// old registration certificate, and reading only that treats the
// re-registration as a parameter update and charges nothing — leaving the
// produced value exactly one deposit short of what the chain charged.
func TestPoolRegistrationDepositDueAfterRetirement(t *testing.T) {
	operator := common.PoolKeyHash(common.NewBlake2b224([]byte{0x42}))
	reg := &common.PoolRegistrationCertificate{Operator: operator}

	build := func(r *common.PoolRegistrationCertificate, retire *uint64) common.LedgerState {
		return mockledger.NewLedgerStateBuilder().
			WithPoolCurrentState(
				func(common.PoolKeyHash) (*common.PoolRegistrationCertificate, *uint64, error) {
					return r, retire, nil
				},
			).Build()
	}
	epoch := func(n uint64) *uint64 { return &n }

	for _, tc := range []struct {
		name    string
		reg     *common.PoolRegistrationCertificate
		retire  *uint64
		current uint64
		want    bool
	}{
		{
			name: "never registered: deposit due",
			reg:  nil, retire: nil, current: 197, want: true,
		},
		{
			name: "registered, no retirement: update, no deposit",
			reg:  reg, retire: nil, current: 197, want: false,
		},
		{
			name: "retirement still pending: update, no deposit",
			reg:  reg, retire: epoch(197), current: 196, want: false,
		},
		{
			name: "retirement has taken effect: deposit due",
			reg:  reg, retire: epoch(197), current: 197, want: true,
		},
		{
			name: "long retired: deposit due",
			reg:  reg, retire: epoch(197), current: 240, want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ls := epochAware{
				LedgerState: build(tc.reg, tc.retire),
				epoch:       tc.current,
			}
			got, err := common.PoolRegistrationDepositDue(ls, 0, operator)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("deposit due = %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("without EpochState the retirement bound is not evaluated", func(t *testing.T) {
		// Documented degradation: the registration on record is taken at face
		// value, which is the behaviour that existed before this helper.
		got, err := common.PoolRegistrationDepositDue(
			build(reg, epoch(197)), 0, operator,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Error("without the epoch capability the bound cannot be evaluated")
		}
	})
}
