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

package chainsync_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/stretchr/testify/require"
)

// TestNtCStateMapBoundsPendingReceiveBytes checks every node-to-client
// ChainSync state carries a nonzero pending receive budget, and that the
// budget is not also a per-message size limit, which the reference
// node-to-client policy leaves unbounded.
func TestNtCStateMapBoundsPendingReceiveBytes(t *testing.T) {
	t.Parallel()

	require.Positive(t, chainsync.PendingReceiveBytesNtC)
	require.NotEmpty(t, chainsync.StateMapNtC)
	for state, entry := range chainsync.StateMapNtC {
		require.Equalf(
			t,
			chainsync.PendingReceiveBytesNtC,
			entry.PendingReceiveByteBudget,
			"state %s pending receive budget",
			state,
		)
		require.Zerof(
			t,
			entry.PendingMessageByteLimit,
			"state %s message size limit must stay unbounded",
			state,
		)
	}
}

// TestNtNStateMapKeepsMessageLimitWithoutReceiveBudget checks the
// node-to-node map is unchanged: its pending-message limit already bounds
// both message size and queued bytes.
func TestNtNStateMapKeepsMessageLimitWithoutReceiveBudget(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, chainsync.StateMapNtN)
	for state, entry := range chainsync.StateMapNtN {
		require.Equalf(
			t,
			chainsync.MaxPendingMessageBytes,
			entry.PendingMessageByteLimit,
			"state %s",
			state,
		)
		require.Zerof(t, entry.PendingReceiveByteBudget, "state %s", state)
	}
}
