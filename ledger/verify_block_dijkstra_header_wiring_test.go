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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/require"
)

// VerifyBlock applies the Dijkstra header version floor: a signed header whose
// highest supported major version is below the one in force is rejected.
func TestVerifyBlockDijkstraHeaderProtocolVersionFloor(t *testing.T) {
	block := buildDijkstraLimitsTestBlock(t, nil)
	tests := []struct {
		name      string
		major     uint
		wantError bool
	}{
		{name: "at the floor", major: dijkstra.MinProtocolVersionDijkstra},
		{
			name:      "below the floor",
			major:     dijkstra.MinProtocolVersionDijkstra + 1,
			wantError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pp := &dijkstra.DijkstraProtocolParameters{}
			pp.ProtocolVersion.Major = test.major
			valid, _, _, _, err := ledger.VerifyBlock(
				block,
				blockLimitsTestEta0Hex,
				blockLimitsTestSlotsPerKesPeriod,
				common.VerifyConfig{
					SkipBodyHashValidation:    true,
					SkipTransactionValidation: true,
					SkipStakePoolValidation:   true,
					ProtocolParameters:        pp,
				},
			)
			if !test.wantError {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.False(t, valid)
			var tooLow *dijkstra.HeaderProtVerTooLowError
			require.ErrorAs(t, err, &tooLow)
		})
	}
}
