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
	"testing"
)

// TestHardForkProtocolVersionCanFollowWrapsMinor covers a hard-fork
// initiation's following rule at the edge of the 32-bit minor version: the
// next minor is computed in 32 bits, so (major, 0) follows
// (major, 4294967295).
func TestHardForkProtocolVersionCanFollowWrapsMinor(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		curMajor, curMinor, newMajor uint
		newMinor                     uint
		want                         bool
	}{
		{"minor increment", 10, 0, 10, 1, true},
		{"major increment", 10, 3, 11, 0, true},
		{"minor wraps at maxBound", 10, math.MaxUint32, 10, 0, true},
		{"same version", 10, 1, 10, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := protocolVersionCanFollow(
				tc.curMajor, tc.curMinor, tc.newMajor, tc.newMinor,
			)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
