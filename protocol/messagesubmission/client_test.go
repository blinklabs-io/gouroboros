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

package messagesubmission

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleRequestMessageIdsRejectsInvalidCountsWithoutMutatingPending(t *testing.T) {
	tests := []struct {
		name         string
		limit        int
		pending      [][]byte
		isBlocking   bool
		ackCount     uint16
		requestCount uint16
	}{
		{
			name:         "acknowledges more IDs than are pending",
			limit:        10,
			pending:      [][]byte{[]byte("id-1"), []byte("id-2")},
			isBlocking:   true,
			ackCount:     3,
			requestCount: 1,
		},
		{
			name:         "blocking request asks for zero IDs",
			limit:        10,
			isBlocking:   true,
			requestCount: 0,
		},
		{
			name:         "nonblocking request asks for zero IDs",
			limit:        10,
			pending:      [][]byte{[]byte("id-1")},
			requestCount: 0,
		},
		{
			name:         "request exceeds remaining unacknowledged window",
			limit:        3,
			pending:      [][]byte{[]byte("id-1"), []byte("id-2")},
			ackCount:     1,
			requestCount: 3,
		},
		{
			name:         "blocking request leaves IDs unacknowledged",
			limit:        10,
			pending:      [][]byte{[]byte("id-1")},
			isBlocking:   true,
			requestCount: 1,
		},
		{
			name:         "nonblocking request leaves no IDs unacknowledged",
			limit:        10,
			pending:      [][]byte{[]byte("id-1")},
			ackCount:     1,
			requestCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(WithMaxUnacknowledgedMessageIDs(tt.limit))
			client := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
			client.pendingMessageIDs = cloneIDs(tt.pending)
			before := client.GetPendingMessageIDs()

			err := client.handleRequestMessageIds(
				NewMsgRequestMessageIds(tt.isBlocking, tt.ackCount, tt.requestCount),
			)

			assert.Equal(t, before, client.GetPendingMessageIDs())
			require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
		})
	}
}

func TestHandleRequestMessageIdsAcceptsWindowBoundary(t *testing.T) {
	var callbackBlocking bool
	var callbackAckCount, callbackRequestCount uint16
	cfg := NewConfig(
		WithMaxUnacknowledgedMessageIDs(3),
		WithRequestMessageIdsFunc(func(
			_ CallbackContext,
			blocking bool,
			ackCount, requestCount uint16,
		) {
			callbackBlocking = blocking
			callbackAckCount = ackCount
			callbackRequestCount = requestCount
		}),
	)
	client := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
	client.pendingMessageIDs = [][]byte{[]byte("id-1"), []byte("id-2")}

	err := client.handleRequestMessageIds(NewMsgRequestMessageIds(false, 1, 2))

	require.NoError(t, err)
	assert.False(t, callbackBlocking)
	assert.Equal(t, uint16(1), callbackAckCount)
	assert.Equal(t, uint16(2), callbackRequestCount)
	assert.Equal(t, [][]byte{[]byte("id-2")}, client.GetPendingMessageIDs())
}

func TestAppendPendingMessageIDsPreservesUnacknowledgedIDs(t *testing.T) {
	client := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), nil)
	client.pendingMessageIDs = [][]byte{[]byte("id-1")}
	newID := []byte("id-2")

	client.appendPendingMessageIDs([]pcommon.MessageIDAndSize{{MessageID: newID}})
	newID[0] = 'x'

	assert.Equal(
		t,
		[][]byte{[]byte("id-1"), []byte("id-2")},
		client.GetPendingMessageIDs(),
	)
}

func cloneIDs(ids [][]byte) [][]byte {
	ret := make([][]byte, len(ids))
	for i, id := range ids {
		ret[i] = append([]byte(nil), id...)
	}
	return ret
}
