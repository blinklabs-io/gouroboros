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

package localstatequery

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// Each result map must grow only with entries that decode, and each genesis
// rational must be bounded before it decodes into a slice. Sized from the
// declared count, a reply of one- or two-byte entries reserves a whole slot
// per entry before the first entry fails.
func TestResultCollectionsGrowAsEntriesDecode(t *testing.T) {
	decodeInto := func(newResult func() any) func([]byte) error {
		return func(data []byte) error {
			_, err := cbor.Decode(data, newResult())
			return err
		}
	}
	entries := twoByteEntries()
	poolState := func(mapField int) []byte {
		out := []byte{0x84}
		for field := range 4 {
			if field == mapField {
				out = append(out, entries...)
				continue
			}
			out = append(out, 0xa0)
		}
		return out
	}
	// genesis encodes a legacy genesis record whose active slots coefficient
	// and pledge influence are asc and a0.
	genesis := func(asc, a0 []byte) []byte {
		return concat(
			[]byte{
				0x8f, 0x83, 0x19, 0x07, 0xe1, 0x18, 0xfe, 0x00,
				0x18, 0x2a, 0x00,
			},
			asc,
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x92},
			[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			a0,
			[]byte{
				0x82, 0x03, 0x0a, 0x82, 0x03, 0x0a, 0x82, 0x03, 0x0a,
				0x81, 0x00, 0x00, 0x00, 0x00, 0x00,
				0xa0, 0x80, 0x82, 0x80, 0xa0,
			},
		)
	}
	pair := []byte{0x82, 0x01, 0x14}
	tests := []struct {
		name   string
		data   []byte
		decode func([]byte) error
	}{
		{
			name: "stake distribution",
			data: concat([]byte{0x81}, entries),
			decode: decodeInto(func() any {
				return new(StakeDistributionResult)
			}),
		},
		{
			name:   "pool distribution",
			data:   concat([]byte{0x81}, entries),
			decode: decodeInto(func() any { return new(PoolDistrResult) }),
		},
		{
			name: "stake distribution 2",
			data: concat([]byte{0x81, 0x82}, entries, []byte{0x00}),
			decode: decodeInto(func() any {
				return new(StakeDistribution2Result)
			}),
		},
		{
			name: "proposed protocol parameter updates",
			data: entries,
			decode: decodeInto(func() any {
				return new(ProposedProtocolParamsUpdatesResult)
			}),
		},
		{
			name: "stake pool parameters",
			data: concat([]byte{0x81}, entries),
			decode: decodeInto(func() any {
				return new(StakePoolParamsResult)
			}),
		},
		{
			name: "reward info pools",
			data: concat(
				[]byte{0x81, 0x82, 0x84, 0x00, 0x82, 0x01, 0x02, 0x00, 0x00},
				entries,
			),
			decode: decodeInto(func() any {
				return new(RewardInfoPoolsResult)
			}),
		},
		{
			name:   "pool state pstate",
			data:   poolState(0),
			decode: decodeInto(func() any { return new(PoolStateResult) }),
		},
		{
			name:   "pool state fstate",
			data:   poolState(1),
			decode: decodeInto(func() any { return new(PoolStateResult) }),
		},
		{
			name:   "pool state retiring",
			data:   poolState(2),
			decode: decodeInto(func() any { return new(PoolStateResult) }),
		},
		{
			name:   "pool state deposits",
			data:   poolState(3),
			decode: decodeInto(func() any { return new(PoolStateResult) }),
		},
		{
			name: "filtered delegations",
			data: concat([]byte{0x81, 0x82}, entries, []byte{0xa0}),
			decode: decodeInto(func() any {
				return new(FilteredDelegationsAndRewardAccountsResult)
			}),
		},
		{
			name: "filtered rewards",
			data: concat([]byte{0x81, 0x82, 0xa0}, entries),
			decode: decodeInto(func() any {
				return new(FilteredDelegationsAndRewardAccountsResult)
			}),
		},
		{
			name: "stake deleg deposits",
			data: concat([]byte{0x81}, entries),
			decode: decodeInto(func() any {
				return new(StakeDelegDepositsResult)
			}),
		},
		{
			name: "filtered vote delegatees",
			data: concat([]byte{0x81}, entries),
			decode: decodeInto(func() any {
				return new(FilteredVoteDelegateesResult)
			}),
		},
		{
			name: "SPO stake distribution",
			data: concat([]byte{0x81}, entries),
			decode: decodeInto(func() any {
				return new(SPOStakeDistrResult)
			}),
		},
		{
			name: "genesis active slots coefficient",
			data: genesis(oneByteEntries(), pair),
			decode: decodeInto(func() any {
				return new(GenesisConfigResult)
			}),
		},
		{
			name: "genesis legacy rational",
			data: genesis(pair, oneByteEntries()),
			decode: decodeInto(func() any {
				return new(GenesisConfigResult)
			}),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.decode(tc.data))
			result := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					if err := tc.decode(tc.data); err == nil {
						b.Fatal("expected decode error")
					}
				}
			})
			t.Logf(
				"%d entries: %d bytes/op",
				governanceAllocEntries,
				result.AllocedBytesPerOp(),
			)
			// The input is at most 2 bytes per entry; anything near the
			// declared count times a slot means the collection was sized
			// from the header.
			require.Less(
				t,
				result.AllocedBytesPerOp(),
				int64(governanceAllocEntries*8),
			)
		})
	}
}
