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
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

const optionTimeout = 50 * time.Millisecond

// requireTimesOut waits for the protocol error a state timer raises.
func requireTimesOut(t *testing.T, errs <-chan error, what string) {
	t.Helper()
	select {
	case err := <-errs:
		require.ErrorContains(t, err, "timeout waiting on transition", what)
	case <-time.After(referenceWireWait):
		t.Fatalf("the Server did not time out %s", what)
	}
}

// holdOneID has the peer announce id in reply to a blocking request, so the
// Server holds one unacknowledged ID: a non-blocking request and a message
// request both need one.
func holdOneID(t *testing.T, h *serverHarness, v2 bool, id []byte) {
	t.Helper()
	if !v2 {
		h.peer.send(true, []byte{0x81, 0x06})
	}
	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	if v2 {
		h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
		h.peer.send(false, cat([]byte{0x82, 0x02, 0x9f}, idAndSize(id), []byte{0xff}))
	} else {
		h.peer.expect(false, []byte{0x84, 0x00, 0xf5, 0x00, 0x03})
		h.peer.send(true, cat([]byte{0x82, 0x01, 0x9f}, idAndSize(id), []byte{0xff}))
	}
	require.Equal(t, 1, await(t, h.idReplies, h.errs, "the id reply"))
}

// TestServerTimesItsWaitsWithTheOptions checks that the timeout options bound
// the waits they name on the side that waits. The Server sends the requests
// and waits for the replies, so the engine arms those states' timers on the
// Server only.
func TestServerTimesItsWaitsWithTheOptions(t *testing.T) {
	id := referenceID(0xd4)
	tests := []struct {
		name    string
		version uint16
		option  MessageSubmissionOptionFunc
		wait    func(*testing.T, *serverHarness)
	}{
		{
			name:    "V2 blocking id request",
			version: protocol.ProtocolVersionDMQNtN2,
			option:  WithMessageIdsBlockingTimeout(optionTimeout),
			wait: func(t *testing.T, h *serverHarness) {
				require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
				h.peer.expect(true, []byte{0x84, 0x01, 0xf5, 0x00, 0x03})
			},
		},
		{
			name:    "V2 non-blocking id request",
			version: protocol.ProtocolVersionDMQNtN2,
			option:  WithMessageIdsNonblockingTimeout(optionTimeout),
			wait: func(t *testing.T, h *serverHarness) {
				holdOneID(t, h, true, id)
				require.NoError(t, h.server.RequestMessageIdsNonBlocking(0, 3))
				h.peer.expect(true, []byte{0x84, 0x01, 0xf4, 0x00, 0x03})
			},
		},
		{
			name:    "V2 message request",
			version: protocol.ProtocolVersionDMQNtN2,
			option:  WithMessagesTimeout(optionTimeout),
			wait: func(t *testing.T, h *serverHarness) {
				holdOneID(t, h, true, id)
				require.NoError(t, h.server.RequestMessages([][]byte{id}))
				h.peer.expect(true, cat([]byte{0x82, 0x04, 0x9f}, byteString32(id), []byte{0xff}))
			},
		},
		{
			name:    "V1 non-blocking id request",
			version: protocol.ProtocolVersionDMQNtN1,
			option:  WithMessageIdsNonblockingTimeout(optionTimeout),
			wait: func(t *testing.T, h *serverHarness) {
				holdOneID(t, h, false, id)
				require.NoError(t, h.server.RequestMessageIdsNonBlocking(0, 3))
				h.peer.expect(false, []byte{0x84, 0x00, 0xf4, 0x00, 0x03})
			},
		},
		{
			name:    "V1 message request",
			version: protocol.ProtocolVersionDMQNtN1,
			option:  WithMessagesTimeout(optionTimeout),
			wait: func(t *testing.T, h *serverHarness) {
				holdOneID(t, h, false, id)
				require.NoError(t, h.server.RequestMessages([][]byte{id}))
				h.peer.expect(false, cat([]byte{0x82, 0x02, 0x9f}, byteString32(id), []byte{0xff}))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newServerHarnessWith(t, tc.version, tc.option)
			tc.wait(t, h)
			requireTimesOut(t, h.errs, tc.name)
		})
	}
}

// TestV1BlockingIDRequestIgnoresTheOption checks the one wait the blocking
// option leaves alone. tx-submission's outbound answers a V1 blocking
// request only once it has an ID, and dmq-node's limit there is
// waitForever (DMQ/Protocol/SigSubmission/Codec.hs timeLimitsSigSubmission),
// so a caller's bound for V2 must not drop a V1 peer that has nothing new.
func TestV1BlockingIDRequestIgnoresTheOption(t *testing.T) {
	h := newServerHarnessWith(
		t,
		protocol.ProtocolVersionDMQNtN1,
		WithMessageIdsBlockingTimeout(optionTimeout),
	)
	h.peer.send(true, []byte{0x81, 0x06})
	require.NoError(t, h.server.RequestMessageIdsBlocking(0, 3))
	h.peer.expect(false, []byte{0x84, 0x00, 0xf5, 0x00, 0x03})
	select {
	case err := <-h.errs:
		t.Fatalf("the V1 blocking wait was timed by the option: %v", err)
	case <-time.After(10 * optionTimeout):
	}
}

// TestV1ClientTimesItsIdleWait checks the one wait of a V1 Client: in Idle,
// after MsgInit, for the peer's next request. WithIdleTimeout bounds it.
func TestV1ClientTimesItsIdleWait(t *testing.T) {
	h := newClientHarnessWith(
		t,
		protocol.ProtocolVersionDMQNtN1,
		WithIdleTimeout(optionTimeout),
	)
	require.NoError(t, h.client.Init())
	h.peer.expect(true, []byte{0x81, 0x06})
	select {
	case err := <-h.errs:
		require.ErrorContains(t, err, "timeout waiting on transition")
	case <-time.After(referenceWireWait):
		t.Fatal("the Client did not time out its wait in Idle")
	}
}
