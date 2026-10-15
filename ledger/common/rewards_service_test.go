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

package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRewardDistributionComparesAllocations(t *testing.T) {
	operatorHash := AddrKeyHash{1}
	delegatorHash := AddrKeyHash{2}
	otherHash := AddrKeyHash{3}
	operator := keyRewardCredential(operatorHash)
	delegator := keyRewardCredential(delegatorHash)
	other := keyRewardCredential(otherHash)
	expected := &RewardCalculationResult{
		PoolRewards: map[PoolKeyHash]PoolRewards{
			{9}: {
				OperatorRewardAccount: operator,
				OperatorRewards:       40,
				DelegatorRewards: map[RewardCredential]uint64{
					delegator: 60,
				},
				TotalRewards: 100,
			},
		},
	}
	service := &RewardService{}

	tests := []struct {
		name   string
		actual map[RewardCredential]uint64
		valid  bool
	}{
		{
			name: "valid independent of insertion order",
			actual: map[RewardCredential]uint64{
				delegator: 60,
				operator:  40,
			},
			valid: true,
		},
		{
			name: "missing recipient",
			actual: map[RewardCredential]uint64{
				operator: 40,
			},
		},
		{
			name: "additional recipient",
			actual: map[RewardCredential]uint64{
				operator:  40,
				delegator: 60,
				other:     1,
			},
		},
		{
			name: "substituted recipient with equal total",
			actual: map[RewardCredential]uint64{
				operator: 40,
				other:    60,
			},
		},
		{
			name: "mismatched amounts with equal total",
			actual: map[RewardCredential]uint64{
				operator:  60,
				delegator: 40,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := service.ValidateRewardDistribution(expected, test.actual)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.EqualError(
					t,
					err,
					"reward distribution does not match calculated allocations",
				)
			}
		})
	}
}

func TestValidateRewardDistributionAggregatesSharedRecipients(t *testing.T) {
	recipientHash := AddrKeyHash{1}
	recipient := keyRewardCredential(recipientHash)
	expected := &RewardCalculationResult{
		PoolRewards: map[PoolKeyHash]PoolRewards{
			{8}: {
				OperatorRewardAccount: recipient,
				OperatorRewards:       40,
				DelegatorRewards: map[RewardCredential]uint64{
					recipient: 10,
				},
			},
			{9}: {
				OperatorRewardAccount: recipient,
				OperatorRewards:       50,
			},
		},
	}

	require.NoError(
		t,
		(&RewardService{}).ValidateRewardDistribution(
			expected,
			map[RewardCredential]uint64{recipient: 100},
		),
	)
}

func TestValidateRewardDistributionOmitsZeroAllocations(t *testing.T) {
	recipientHash := AddrKeyHash{1}
	recipient := keyRewardCredential(recipientHash)
	expected := &RewardCalculationResult{
		PoolRewards: map[PoolKeyHash]PoolRewards{
			{9}: {
				OperatorRewardAccount: recipient,
				DelegatorRewards: map[RewardCredential]uint64{
					recipient: 0,
				},
			},
		},
	}

	require.NoError(
		t,
		(&RewardService{}).ValidateRewardDistribution(
			expected,
			map[RewardCredential]uint64{},
		),
	)
}

func TestValidateRewardDistributionDistinguishesCredentialKinds(t *testing.T) {
	hash := CredentialHash{1}
	script := RewardCredential{
		CredType:   CredentialTypeScriptHash,
		Credential: hash,
	}
	key := RewardCredential{
		CredType:   CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	service := &RewardService{}
	tests := []struct {
		name    string
		rewards PoolRewards
	}{
		{
			name: "operator",
			rewards: PoolRewards{
				OperatorRewardAccount: script,
				OperatorRewards:       40,
			},
		},
		{
			name: "delegator",
			rewards: PoolRewards{
				DelegatorRewards: map[RewardCredential]uint64{
					script: 40,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			expected := &RewardCalculationResult{
				PoolRewards: map[PoolKeyHash]PoolRewards{{9}: test.rewards},
			}
			require.NoError(
				t,
				service.ValidateRewardDistribution(
					expected,
					map[RewardCredential]uint64{script: 40},
				),
			)
			require.EqualError(
				t,
				service.ValidateRewardDistribution(
					expected,
					map[RewardCredential]uint64{key: 40},
				),
				"reward distribution does not match calculated allocations",
			)
		})
	}
}
