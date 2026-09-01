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

package conway

import (
	"math"
	"strconv"
	"testing"
)

func TestGovernanceProposalIndexFits(t *testing.T) {
	tests := []struct {
		name string
		idx  int
		want bool
	}{
		{name: "negative", idx: -1, want: false},
		{name: "zero", idx: 0, want: true},
		{
			name: "largest native int",
			idx:  int(^uint(0) >> 1),
			want: strconv.IntSize == 32,
		},
	}

	// MaxUint32 and MaxUint32+1 are representable as int only on 64-bit
	// targets. Keep these cases runtime-gated so this test itself compiles on
	// 32-bit targets while exercising both sides of the production boundary on
	// 64-bit targets.
	if strconv.IntSize == 64 {
		maxUint32 := uint64(math.MaxUint32)
		tests = append(tests,
			struct {
				name string
				idx  int
				want bool
			}{"MaxUint32", int(maxUint32), true},
			struct {
				name string
				idx  int
				want bool
			}{"MaxUint32 plus one", int(maxUint32 + 1), false},
		)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := governanceProposalIndexFits(test.idx); got != test.want {
				t.Fatalf("governanceProposalIndexFits(%d) = %t, want %t", test.idx, got, test.want)
			}
		})
	}
}
