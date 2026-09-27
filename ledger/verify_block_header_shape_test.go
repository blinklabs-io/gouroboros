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

package ledger

import (
	"fmt"
	"math"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/stretchr/testify/require"
)

// TestDetermineBlockTypeHeaderShapeBoundaries walks the protocol majors at
// each edge of the Babbage-shaped (10-field) and Leios-extended (12-field)
// header bodies. Only Dijkstra's CDDL has the 12-field shape, whose major is
// the issuer's highest supported version: Dijkstra's BBODY rule requires it
// to be at least the ledger's current major (HeaderProtVerTooLow) and bounds
// it only by its uint .size 4 encoding. So a 12-field body is Dijkstra from
// Dijkstra's first major up to MaxUint32, including the majors a producer
// announces ahead of the next hard fork, and a pre-Dijkstra major is no valid
// block of any era rather than a Mary through Conway block.
func TestDetermineBlockTypeHeaderShapeBoundaries(t *testing.T) {
	testCases := []struct {
		fields    int
		major     uint64
		blockType uint
		errText   string
	}{
		{
			fields:    10,
			major:     babbage.MinProtocolVersionBabbage,
			blockType: BlockTypeBabbage,
		},
		{
			fields:    10,
			major:     babbage.MaxProtocolVersionBabbage,
			blockType: BlockTypeBabbage,
		},
		{
			fields:    10,
			major:     conway.MinProtocolVersionConway,
			blockType: BlockTypeConway,
		},
		{
			fields:    10,
			major:     conway.MaxProtocolVersionConway,
			blockType: BlockTypeConway,
		},
		{
			fields:    10,
			major:     dijkstra.MinProtocolVersionDijkstra,
			blockType: BlockTypeDijkstra,
		},
		{
			fields:    10,
			major:     dijkstra.MaxProtocolVersionDijkstra,
			blockType: BlockTypeDijkstra,
		},
		{
			fields:  10,
			major:   dijkstra.MaxProtocolVersionDijkstra + 1,
			errText: "unknown proto major",
		},
		{
			fields:    12,
			major:     dijkstra.MinProtocolVersionDijkstra,
			blockType: BlockTypeDijkstra,
		},
		{
			fields:    12,
			major:     dijkstra.MaxProtocolVersionDijkstra,
			blockType: BlockTypeDijkstra,
		},
		{
			fields:    12,
			major:     dijkstra.MaxProtocolVersionDijkstra + 1,
			blockType: BlockTypeDijkstra,
		},
		{
			fields:    12,
			major:     math.MaxUint32,
			blockType: BlockTypeDijkstra,
		},
		{
			fields:  12,
			major:   math.MaxUint32 + 1,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   dijkstra.MinProtocolVersionDijkstra - 1,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   conway.MaxProtocolVersionConway,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   conway.MinProtocolVersionConway,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   babbage.MaxProtocolVersionBabbage,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   babbage.MinProtocolVersionBabbage,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   alonzo.MinProtocolVersionAlonzo,
			errText: "unknown proto major",
		},
		{
			fields:  12,
			major:   mary.MinProtocolVersionMary,
			errText: "unknown proto major",
		},
		{
			fields:  11,
			major:   dijkstra.MinProtocolVersionDijkstra,
			errText: "unknown header body length 11",
		},
		{
			fields:  13,
			major:   dijkstra.MinProtocolVersionDijkstra,
			errText: "unknown header body length 13",
		},
	}
	for _, tc := range testCases {
		t.Run(
			fmt.Sprintf("%d-field major %d", tc.fields, tc.major),
			func(t *testing.T) {
				t.Parallel()
				body := make([]any, tc.fields)
				body[9] = []any{tc.major, uint64(0)}
				headerCbor, err := cbor.Encode([]any{body, []byte{}})
				require.NoError(t, err)
				blockType, err := DetermineBlockType(headerCbor)
				if tc.errText != "" {
					require.ErrorContainsf(t, err, tc.errText,
						"classified as block type %d", blockType)
					return
				}
				require.NoError(t, err)
				require.Equal(t, tc.blockType, blockType)
			},
		)
	}
}
