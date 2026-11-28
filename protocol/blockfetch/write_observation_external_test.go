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

package blockfetch_test

import (
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestRangeWriteObservationSharedPeerNoBlocks(t *testing.T) {
	defer goleak.VerifyNone(t)
	recorder, err := muxer.NewWriteRecorder(1)
	require.NoError(t, err)
	h := newPipelineHarness(t, []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryHandshakeNtNResponse,
		requestRangeInput(), batchOutput(blockfetch.NewMsgNoBlocks()),
	}, blockfetch.WithWriteRecorder(recorder))
	defer h.close(t)
	id, err := h.client.RequestRange(context.Background(), blockfetch.RangeRequest{})
	require.NoError(t, err)
	result := h.nextDone(t)
	require.Equal(t, id, result.requestId)
	require.ErrorIs(t, result.err, blockfetch.ErrNoBlocks)
	require.True(t, h.client.ProtocolInstance().WaitSendQueueDrained(time.Second))
	record := recorder.Snapshot()[0]
	require.NotZero(t, h.client.WriteObservationSession())
	require.Equal(t, h.client.WriteObservationSession(), record.Metadata.Session)
	require.Equal(t, id, record.Metadata.RequestID)
	require.False(t, record.WriteStartedAt.IsZero())
	require.False(t, record.WriteCompletedAt.IsZero())
	require.True(t, record.FailedAt.IsZero())
	require.Zero(t, recorder.Lost())
	require.Zero(t, recorder.Rejected())
}
