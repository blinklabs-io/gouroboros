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

package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newValidatedTestItem(t *testing.T, seq uint64) *BlockItem {
	t.Helper()
	item := newDecodedTestItem(t, seq)
	item.SetValidation(true, "vrf", nil, time.Millisecond)
	return item
}

// Cancellation while ApplyFunc commits a block lets that block finish but
// stops the ordered chain before the next buffered block: the next block is
// neither applied nor taken from the buffer, so nextSequence stays on it.
func TestApplyStageCancellationStopsBufferedChain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var applied []uint64
	stage := newTestApplyStage(func(item *BlockItem) error {
		applied = append(applied, item.SequenceNumber())
		if item.SequenceNumber() == 0 {
			cancel()
		}
		return nil
	}, 0)

	for _, seq := range []uint64{1, 2} {
		processed, err := stage.ProcessWithStatus(ctx, newValidatedTestItem(t, seq))
		require.NoError(t, err)
		require.Empty(t, processed)
	}
	require.Equal(t, 2, stage.PendingCount())

	processed, err := stage.ProcessWithStatus(ctx, newValidatedTestItem(t, 0))
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, processed, 1)
	require.Equal(t, uint64(0), processed[0].SequenceNumber())
	require.True(t, processed[0].IsApplied())
	require.Equal(t, []uint64{0}, applied)
	require.Equal(t, 2, stage.PendingCount())
	stage.mu.Lock()
	nextSequence := stage.nextSequence
	stage.mu.Unlock()
	require.Equal(t, uint64(1), nextSequence)
}
