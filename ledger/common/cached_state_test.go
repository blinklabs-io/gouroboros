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
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// allCapabilityState implements every optional ledger-state capability
// declared in state.go, so a rule that asserts one against it succeeds when it
// is passed directly. VerifyTransaction substitutes a cache wrapper that
// implements none of them.
type allCapabilityState struct {
	common.LedgerState
}

func (allCapabilityState) EpochForSlot(uint64) (uint64, error) { return 0, nil }

func (allCapabilityState) StakeCredentialDeposit(
	common.Credential,
) (*uint64, error) {
	return nil, nil
}

func (allCapabilityState) CommitteeStateAvailable() (bool, error) {
	return true, nil
}

func (allCapabilityState) CommitteeCredentialMember(
	common.Credential,
) (*common.CommitteeMember, error) {
	return nil, nil
}

func (allCapabilityState) CommitteeHotCredentialMember(
	common.Credential,
) (*common.CommitteeMember, error) {
	return nil, nil
}

func (allCapabilityState) DRepDelegation(
	common.Credential,
) (*common.Drep, error) {
	return nil, nil
}

func (allCapabilityState) GenesisDelegateKeyHashes(
	uint64,
) (
	[]common.Blake2b224,
	error,
) {
	return nil, nil
}

func (allCapabilityState) GenesisDelegateForGenesisKey(
	common.Blake2b224,
	uint64,
) (common.Blake2b224, bool, error) {
	return common.Blake2b224{}, false, nil
}

func (allCapabilityState) GenesisUpdateQuorum() (uint, error) { return 0, nil }

func (allCapabilityState) GovPurposeRoots() (*common.GovPurposeRoots, error) {
	return &common.GovPurposeRoots{}, nil
}

func (allCapabilityState) Tip() (pcommon.Tip, error) { return pcommon.Tip{}, nil }

var (
	_ common.EpochState                  = allCapabilityState{}
	_ common.StakeCredentialDepositState = allCapabilityState{}
	_ common.CommitteeCredentialState    = allCapabilityState{}
	_ common.DRepDelegationState         = allCapabilityState{}
	_ common.GenesisDelegationState      = allCapabilityState{}
	_ common.GovPurposeRootsState        = allCapabilityState{}
	_ common.TipState                    = allCapabilityState{}
)

// capabilityProbes asserts each optional capability against a ledger state,
// once on the state as the rule receives it and once after unwrapping.
var capabilityProbes = map[string]func(common.LedgerState) (direct, unwrapped bool){
	"EpochState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.EpochState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.EpochState)
		return direct, unwrapped
	},
	"StakeCredentialDepositState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.StakeCredentialDepositState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.StakeCredentialDepositState)
		return direct, unwrapped
	},
	"CommitteeCredentialState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.CommitteeCredentialState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.CommitteeCredentialState)
		return direct, unwrapped
	},
	"DRepDelegationState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.DRepDelegationState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.DRepDelegationState)
		return direct, unwrapped
	},
	"GenesisDelegationState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.GenesisDelegationState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.GenesisDelegationState)
		return direct, unwrapped
	},
	"GovPurposeRootsState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.GovPurposeRootsState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.GovPurposeRootsState)
		return direct, unwrapped
	},
	"TipState": func(ls common.LedgerState) (bool, bool) {
		_, direct := ls.(common.TipState)
		_, unwrapped := common.UnwrapLedgerState(ls).(common.TipState)
		return direct, unwrapped
	},
}

// TestVerifyTransactionUnwrapRestoresEveryOptionalCapability records the
// contract a rule must follow: inside VerifyTransaction a direct assertion
// fails for every optional capability and UnwrapLedgerState restores all of
// them. The direct-assertion half is what makes this test non-vacuous - it
// fails if the wrapper ever satisfies a capability by accident, in which case
// the rules no longer need to unwrap.
func TestVerifyTransactionUnwrapRestoresEveryOptionalCapability(t *testing.T) {
	t.Parallel()
	state := allCapabilityState{
		LedgerState: mockledger.NewLedgerStateBuilder().Build(),
	}
	for name, probe := range capabilityProbes {
		direct, unwrapped := probe(state)
		require.True(t, direct, "%s: unwrapped state must implement it", name)
		require.True(t, unwrapped, "%s: UnwrapLedgerState is identity here", name)
	}

	observed := map[string][2]bool{}
	err := common.VerifyTransaction(
		nil,
		0,
		state,
		nil,
		[]common.UtxoValidationRuleFunc{
			func(
				_ common.Transaction,
				_ uint64,
				ls common.LedgerState,
				_ common.ProtocolParameters,
			) error {
				for name, probe := range capabilityProbes {
					direct, unwrapped := probe(ls)
					observed[name] = [2]bool{direct, unwrapped}
				}
				return nil
			},
		},
	)
	require.NoError(t, err)
	require.Len(t, observed, len(capabilityProbes))
	for name, got := range observed {
		require.False(
			t,
			got[0],
			"%s: cache wrapper must not satisfy an optional capability", name,
		)
		require.True(
			t,
			got[1],
			"%s: UnwrapLedgerState must restore the capability", name,
		)
	}
}

// TestVerifyTransactionUnwrapLeavesForeignStateAlone pins UnwrapLedgerState to
// the wrapper this package creates. A caller-supplied state is returned
// unchanged even when it carries an UnderlyingLedgerState method of its own.
func TestVerifyTransactionUnwrapLeavesForeignStateAlone(t *testing.T) {
	t.Parallel()
	inner := mockledger.NewLedgerStateBuilder().Build()
	state := foreignWrapper{LedgerState: mockledger.NewLedgerStateBuilder().Build(), inner: inner}
	require.Equal(
		t,
		common.LedgerState(state),
		common.UnwrapLedgerState(state),
	)
}

type foreignWrapper struct {
	common.LedgerState
	inner common.LedgerState
}

func (w foreignWrapper) UnderlyingLedgerState() common.LedgerState {
	return w.inner
}

// TestVerifyTransactionCacheIsSafeForConcurrentRules covers a rule that
// resolves inputs from more than one goroutine. VerifyTransaction replaces the
// caller's state with the cache wrapper, so the wrapper has to keep whatever
// concurrency guarantee that state had. Without the lock this fails under
// -race, and fatals on "concurrent map writes" without it.
func TestVerifyTransactionCacheIsSafeForConcurrentRules(t *testing.T) {
	t.Parallel()
	const inputCount = 32
	inputs := make([]common.TransactionInput, 0, inputCount)
	for i := range inputCount {
		input, err := mockledger.NewTransactionInputBuilder().
			WithTxId([]byte{byte(i + 1)}).
			WithIndex(uint32(i)).
			Build()
		require.NoError(t, err)
		inputs = append(inputs, input)
	}
	output, err := mockledger.NewTransactionOutputBuilder().
		WithAddress("addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd").
		WithLovelace(2_000_000).
		Build()
	require.NoError(t, err)
	state := mockledger.NewLedgerStateBuilder().
		WithUtxoById(func(got common.TransactionInput) (common.Utxo, error) {
			time.Sleep(time.Microsecond)
			return common.Utxo{Id: got, Output: output}, nil
		}).
		Build()

	err = common.VerifyTransaction(
		nil,
		0,
		state,
		nil,
		[]common.UtxoValidationRuleFunc{
			func(
				_ common.Transaction,
				_ uint64,
				ls common.LedgerState,
				_ common.ProtocolParameters,
			) error {
				var wg sync.WaitGroup
				// Each input is resolved twice so both the miss and
				// the hit path run concurrently.
				for range 2 {
					for _, input := range inputs {
						wg.Add(1)
						go func(in common.TransactionInput) {
							defer wg.Done()
							_, _ = ls.UtxoById(in)
						}(input)
					}
				}
				wg.Wait()
				return nil
			},
		},
	)
	require.NoError(t, err)
}
