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
	operator := AddrKeyHash{1}
	delegator := AddrKeyHash{2}
	other := AddrKeyHash{3}
	expected := &RewardCalculationResult{
		PoolRewards: map[PoolKeyHash]PoolRewards{
			{9}: {
				OperatorRewardAccount: operator,
				OperatorRewards:       40,
				DelegatorRewards: map[AddrKeyHash]uint64{
					delegator: 60,
				},
				TotalRewards: 100,
			},
		},
	}
	service := &RewardService{}

	tests := []struct {
		name   string
		actual map[AddrKeyHash]uint64
		valid  bool
	}{
		{
			name: "valid independent of insertion order",
			actual: map[AddrKeyHash]uint64{
				delegator: 60,
				operator:  40,
			},
			valid: true,
		},
		{
			name: "missing recipient",
			actual: map[AddrKeyHash]uint64{
				operator: 40,
			},
		},
		{
			name: "additional recipient",
			actual: map[AddrKeyHash]uint64{
				operator:  40,
				delegator: 60,
				other:     1,
			},
		},
		{
			name: "substituted recipient with equal total",
			actual: map[AddrKeyHash]uint64{
				operator: 40,
				other:    60,
			},
		},
		{
			name: "mismatched amounts with equal total",
			actual: map[AddrKeyHash]uint64{
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
	recipient := AddrKeyHash{1}
	expected := &RewardCalculationResult{
		PoolRewards: map[PoolKeyHash]PoolRewards{
			{8}: {
				OperatorRewardAccount: recipient,
				OperatorRewards:       40,
				DelegatorRewards: map[AddrKeyHash]uint64{
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
			map[AddrKeyHash]uint64{recipient: 100},
		),
	)
}

func TestValidateRewardDistributionOmitsZeroAllocations(t *testing.T) {
	recipient := AddrKeyHash{1}
	expected := &RewardCalculationResult{
		PoolRewards: map[PoolKeyHash]PoolRewards{
			{9}: {
				OperatorRewardAccount: recipient,
				DelegatorRewards: map[AddrKeyHash]uint64{
					recipient: 0,
				},
			},
		},
	}

	require.NoError(
		t,
		(&RewardService{}).ValidateRewardDistribution(
			expected,
			map[AddrKeyHash]uint64{},
		),
	)
}
