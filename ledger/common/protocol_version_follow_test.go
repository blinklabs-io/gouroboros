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
	"math"
	"testing"
)

// TestProtocolVersionCanFollowWrapsMinor covers the ledger's following rule
// at the edge of its 32-bit minor version: the next minor is computed in 32
// bits, so (major, 0) follows (major, 4294967295).
func TestProtocolVersionCanFollowWrapsMinor(t *testing.T) {
	v := func(major, minor uint) ProtocolParametersProtocolVersion {
		return ProtocolParametersProtocolVersion{Major: major, Minor: minor}
	}
	for _, tc := range []struct {
		name     string
		cur, new ProtocolParametersProtocolVersion
		want     bool
	}{
		{"minor increment", v(7, 0), v(7, 1), true},
		{"major increment", v(7, 3), v(8, 0), true},
		{"minor wraps at maxBound", v(7, math.MaxUint32), v(7, 0), true},
		{"major from maxBound minor", v(7, math.MaxUint32), v(8, 0), true},
		{"same version", v(7, 1), v(7, 1), false},
		{"minor skip", v(7, 1), v(7, 3), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := protocolVersionCanFollow(tc.cur, tc.new); got != tc.want {
				t.Errorf("%v -> %v: got %v, want %v", tc.cur, tc.new, got, tc.want)
			}
		})
	}
}
