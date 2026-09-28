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

package blockfetch

import (
	"testing"

	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// A Byron epoch boundary puts an EBB and the first main block in one slot, so
// two distinct blocks legitimately share the slot of a range endpoint.

type recordStep struct {
	point    pcommon.Point
	prevHash []byte
}

func recordAll(req *rangeRequest, steps []recordStep) error {
	for _, s := range steps {
		if err := req.recordBlock(s.point, s.prevHash); err != nil {
			return err
		}
	}
	return nil
}

func TestRecordBlockSameSlotEBBAndMainBlock(t *testing.T) {
	t.Parallel()
	genesis := []byte("genesis")
	ebb := pcommon.NewPoint(0, []byte("ebb"))
	main0 := pcommon.NewPoint(0, []byte("main0"))
	main5 := pcommon.NewPoint(5, []byte("main5"))
	later := pcommon.NewPoint(9, []byte("later"))

	t.Run("start EBB end main block same slot", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: ebb, end: main0}
		require.NoError(t, recordAll(req, []recordStep{
			{ebb, genesis},
			{main0, ebb.Hash},
		}))
		require.True(t, pointsEqual(req.lastPoint, req.end))
	})

	t.Run("start EBB end later slot", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: ebb, end: main5}
		require.NoError(t, recordAll(req, []recordStep{
			{ebb, genesis},
			{main0, ebb.Hash},
			{main5, main0.Hash},
		}))
		require.True(t, pointsEqual(req.lastPoint, req.end))
	})

	t.Run("start main block same slot as EBB", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: main0, end: main5}
		require.NoError(t, recordAll(req, []recordStep{
			{main0, ebb.Hash},
			{main5, main0.Hash},
		}))
	})

	t.Run("first block at start slot must be the start point", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: main0, end: main5}
		err := recordAll(req, []recordStep{{ebb, genesis}})
		require.ErrorContains(t, err, "does not match requested range start")
	})

	t.Run("same slot block must chain from previous block", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: ebb, end: main5}
		err := recordAll(req, []recordStep{
			{ebb, genesis},
			{main0, []byte("unrelated")},
		})
		require.ErrorContains(t, err, "does not follow previous range point")
	})

	t.Run("block after end at end slot is rejected", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: ebb, end: ebb}
		err := recordAll(req, []recordStep{
			{ebb, genesis},
			{main0, ebb.Hash},
		})
		require.ErrorContains(t, err, "beyond requested range end")
	})

	t.Run("slot past end is rejected", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: ebb, end: main5}
		err := recordAll(req, []recordStep{
			{ebb, genesis},
			{main0, ebb.Hash},
			{main5, main0.Hash},
			{later, main5.Hash},
		})
		require.ErrorContains(t, err, "outside requested range")
	})

	t.Run("slot before start is rejected", func(t *testing.T) {
		t.Parallel()
		req := &rangeRequest{start: main5, end: later}
		err := recordAll(req, []recordStep{{main0, ebb.Hash}})
		require.ErrorContains(t, err, "outside requested range")
	})
}
