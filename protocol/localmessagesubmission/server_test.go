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

package localmessagesubmission

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

type submissionTestSession struct {
	client *Client
	result chan pcommon.RejectReason
}

func newSubmissionTestSession(
	t *testing.T,
	serverCfg *Config,
) submissionTestSession {
	t.Helper()
	serverConn, clientConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	clientErrors := make(chan error, 1)
	serverErrors := make(chan error, 1)
	result := make(chan pcommon.RejectReason, 1)
	clientCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithAcceptMessageFunc(func(CallbackContext) {
			result <- nil
		}),
		WithRejectMessageFunc(func(
			_ CallbackContext,
			reason pcommon.RejectReason,
		) {
			result <- reason
		}),
	)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: clientErrors,
	}, &clientCfg)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     serverMuxer,
		ErrorChan: serverErrors,
	}, serverCfg)
	client.Protocol.EnsureRegistered()
	server.Protocol.EnsureRegistered()
	clientMuxer.Start()
	serverMuxer.Start()
	client.Start()
	server.Start()
	t.Cleanup(func() {
		_ = client.Stop()
		server.Protocol.Stop()
		clientMuxer.Stop()
		serverMuxer.Stop()
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	return submissionTestSession{client: client, result: result}
}

func waitSubmissionResult(
	t *testing.T,
	result <-chan pcommon.RejectReason,
) pcommon.RejectReason {
	t.Helper()
	select {
	case reason := <-result:
		return reason
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for submission result")
		return nil
	}
}

func replayTestMessage() pcommon.DmqMessage {
	return pcommon.DmqMessage{
		MessageID: []byte("authenticated-message-id"),
		Payload: pcommon.DmqMessagePayload{
			ExpiresAt: uint32(time.Now().Add(time.Minute).Unix()),
		},
	}
}

func TestServerRejectsSequentialDuplicateSubmission(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	serverCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithSubmitMessageFunc(func(
			CallbackContext,
			*pcommon.DmqMessage,
		) pcommon.RejectReason {
			calls.Add(1)
			return nil
		}),
	)
	session := newSubmissionTestSession(t, &serverCfg)
	msg := replayTestMessage()

	require.NoError(t, session.client.SubmitMessage(&msg))
	require.Nil(t, waitSubmissionResult(t, session.result))
	require.NoError(t, session.client.SubmitMessage(&msg))
	reason := waitSubmissionResult(t, session.result)
	require.IsType(t, pcommon.AlreadyReceivedReason{}, reason)
	require.EqualValues(t, 1, calls.Load())
}

func TestServerReservesRejectedSubmission(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	serverCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithSubmitMessageFunc(func(
			CallbackContext,
			*pcommon.DmqMessage,
		) pcommon.RejectReason {
			calls.Add(1)
			return pcommon.OtherReason{Message: "application rejected message"}
		}),
	)
	session := newSubmissionTestSession(t, &serverCfg)
	msg := replayTestMessage()

	require.NoError(t, session.client.SubmitMessage(&msg))
	require.IsType(
		t,
		pcommon.OtherReason{},
		waitSubmissionResult(t, session.result),
	)
	require.NoError(t, session.client.SubmitMessage(&msg))
	require.IsType(
		t,
		pcommon.AlreadyReceivedReason{},
		waitSubmissionResult(t, session.result),
	)
	require.EqualValues(t, 1, calls.Load())
}

func TestServerRejectsInvalidReplayCapacity(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	serverCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithMaxReplayEntries(-1),
		WithSubmitMessageFunc(func(
			CallbackContext,
			*pcommon.DmqMessage,
		) pcommon.RejectReason {
			calls.Add(1)
			return nil
		}),
	)
	session := newSubmissionTestSession(t, &serverCfg)
	msg := replayTestMessage()

	require.NoError(t, session.client.SubmitMessage(&msg))
	require.IsType(
		t,
		pcommon.OtherReason{},
		waitSubmissionResult(t, session.result),
	)
	require.Zero(t, calls.Load())
}

func TestServerRejectsConcurrentDuplicateSubmissions(t *testing.T) {
	t.Parallel()
	const sessionCount = 8
	var calls atomic.Int32
	serverCfg := NewConfig(
		WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
		WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
		WithSubmitMessageFunc(func(
			CallbackContext,
			*pcommon.DmqMessage,
		) pcommon.RejectReason {
			calls.Add(1)
			return nil
		}),
	)
	sessions := make([]submissionTestSession, sessionCount)
	for i := range sessions {
		sessions[i] = newSubmissionTestSession(t, &serverCfg)
	}
	msg := replayTestMessage()
	start := make(chan struct{})
	errCh := make(chan error, sessionCount)
	var wg sync.WaitGroup
	for i := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errCh <- sessions[i].client.SubmitMessage(&msg)
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	accepted := 0
	alreadyReceived := 0
	for i := range sessions {
		reason := waitSubmissionResult(t, sessions[i].result)
		switch reason.(type) {
		case nil:
			accepted++
		case pcommon.AlreadyReceivedReason:
			alreadyReceived++
		default:
			require.Failf(t, "unexpected rejection", "%T", reason)
		}
	}
	require.Equal(t, 1, accepted)
	require.Equal(t, sessionCount-1, alreadyReceived)
	require.EqualValues(t, 1, calls.Load())
}

func TestSubmissionReplayStateRemainsBoundedUntilExpiration(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	state := newSubmissionReplayState()

	reserved, err := state.reserve([]byte("first"), 101, now, 1, nil)
	require.NoError(t, err)
	require.True(t, reserved)

	reserved, err = state.reserve([]byte("first"), 101, now, 1, nil)
	require.NoError(t, err)
	require.False(t, reserved)

	reserved, err = state.reserve(
		[]byte("second"),
		102,
		time.Unix(101, 0),
		1,
		nil,
	)
	require.ErrorIs(t, err, errReplayCacheCapacityExceeded)
	require.False(t, reserved)

	reserved, err = state.reserve(
		[]byte("second"),
		103,
		time.Unix(102, 0),
		1,
		nil,
	)
	require.NoError(t, err)
	require.True(t, reserved)
}

func TestSubmissionReplayStateRejectsAfterCapacityReduction(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	state := newSubmissionReplayState()
	reserved, err := state.reserve([]byte("first"), 200, now, 2, nil)
	require.NoError(t, err)
	require.True(t, reserved)
	reserved, err = state.reserve([]byte("second"), 200, now, 2, nil)
	require.NoError(t, err)
	require.True(t, reserved)

	reserved, err = state.reserve([]byte("third"), 200, now, 1, nil)
	require.ErrorIs(t, err, errReplayCacheCapacityExceeded)
	require.False(t, reserved)
}

func TestSubmissionReplayStateRejectsInvalidCapacity(t *testing.T) {
	t.Parallel()
	reserved, err := newSubmissionReplayState().reserve(
		[]byte("first"),
		200,
		time.Unix(100, 0),
		-1,
		nil,
	)
	require.ErrorIs(t, err, errInvalidReplayCacheCapacity)
	require.False(t, reserved)
}

func TestSubmissionReplayStateCommitsOnlyReservedIDs(t *testing.T) {
	t.Parallel()
	now := time.Unix(100, 0)
	state := newSubmissionReplayState()
	commits := 0
	commit := func() error {
		commits++
		return nil
	}

	reserved, err := state.reserve([]byte("first"), 200, now, 2, commit)
	require.NoError(t, err)
	require.True(t, reserved)
	reserved, err = state.reserve([]byte("first"), 200, now, 2, commit)
	require.NoError(t, err)
	require.False(t, reserved)
	require.Equal(t, 1, commits)

	commitErr := errors.New("authentication state changed")
	reserved, err = state.reserve(
		[]byte("second"),
		200,
		now,
		2,
		func() error { return commitErr },
	)
	require.ErrorIs(t, err, commitErr)
	require.False(t, reserved)

	reserved, err = state.reserve([]byte("second"), 200, now, 2, nil)
	require.NoError(t, err)
	require.True(t, reserved)
}

func TestServerRejectsExpiredMessageWithExpiredReason(t *testing.T) {
	for _, test := range []struct {
		name      string
		expiresAt uint32
		wantType  uint8
	}{
		{
			name:      "expired",
			expiresAt: 1,
			wantType:  2,
		},
		{
			name:      "too far in future remains invalid",
			expiresAt: uint32(time.Now().Add(time.Hour).Unix()),
			wantType:  0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverConn, clientConn := net.Pipe()
			clientMuxer := muxer.New(clientConn)
			serverMuxer := muxer.New(serverConn)
			clientErrors := make(chan error, 1)
			serverErrors := make(chan error, 1)
			rejectReasons := make(chan pcommon.RejectReason, 1)
			clientCfg := NewConfig(
				WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
				WithTTLValidator(pcommon.NewNoOpTTLValidator(nil)),
				WithRejectMessageFunc(func(
					_ CallbackContext,
					reason pcommon.RejectReason,
				) {
					rejectReasons <- reason
				}),
			)
			serverCfg := NewConfig(
				WithAuthenticator(pcommon.NewNoOpAuthenticator(nil)),
				WithSubmitMessageFunc(func(
					CallbackContext,
					*pcommon.DmqMessage,
				) pcommon.RejectReason {
					return nil
				}),
			)
			client := NewClient(protocol.ProtocolOptions{
				Muxer:     clientMuxer,
				ErrorChan: clientErrors,
			}, &clientCfg)
			server := NewServer(protocol.ProtocolOptions{
				Muxer:     serverMuxer,
				ErrorChan: serverErrors,
			}, &serverCfg)
			client.Protocol.EnsureRegistered()
			server.Protocol.EnsureRegistered()
			clientMuxer.Start()
			serverMuxer.Start()
			client.Start()
			server.Start()
			t.Cleanup(func() {
				_ = client.Stop()
				server.Protocol.Stop()
				clientMuxer.Stop()
				serverMuxer.Stop()
				_ = clientConn.Close()
				_ = serverConn.Close()
			})

			require.NoError(t, client.SubmitMessage(&pcommon.DmqMessage{
				Payload: pcommon.DmqMessagePayload{ExpiresAt: test.expiresAt},
			}))
			select {
			case reason := <-rejectReasons:
				require.Equal(t, test.wantType, reason.RejectReasonType())
			case <-time.After(5 * time.Second):
				require.FailNow(t, "timed out waiting for rejection reason")
			}
		})
	}
}
