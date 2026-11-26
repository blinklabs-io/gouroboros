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

package messagesubmission_test

import (
	"context"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/messagesubmission"
	"github.com/stretchr/testify/require"
)

// TestV2ClientServesRunAfterMsgDone ends a V2 run with the requesting
// side's MsgDone and starts the next run with a new request for IDs. In V2
// the Client is the responder, and dmq-node runs its node-to-node
// responders under the inbound governor, which restarts a responder that
// ended cleanly; dmq-node's V2 inbound sends MsgDone when its hot peer is
// demoted and requests again on the same connection when it is promoted.
func TestV2ClientServesRunAfterMsgDone(t *testing.T) {
	t.Parallel()
	version := messagesubmission.MessageSubmissionV2MinVersion
	h := restartwindow.New(t, messagesubmission.ProtocolID)
	requests := make(chan struct{}, 4)
	cfg := messagesubmission.NewConfig(
		messagesubmission.WithRequestMessageIdsFunc(func(
			messagesubmission.CallbackContext, bool, uint16, uint16,
		) {
			requests <- struct{}{}
		}),
	)
	opts := protocol.ProtocolOptions{
		Muxer:   h.Server,
		Logger:  h.Logger("unused hold"),
		Mode:    protocol.ProtocolModeNodeToNode,
		Version: version,
	}
	client := messagesubmission.NewClient(opts, &cfg)
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	h.Start()

	for run := range 2 {
		h.Send(messagesubmission.NewMsgRequestMessageIds(version, true, 0, 1))
		select {
		case <-requests:
		case <-time.After(restartwindow.Timeout):
			t.Fatalf("run %d: the client never handled the request", run)
		}
		require.NoError(t, client.ReplyNoMessageIds())
		require.Equal(
			t,
			uint(messagesubmission.MessageTypeV2ReplyNoMessageIds),
			h.RecvType("ReplyNoMessageIds"),
		)
		h.Send(messagesubmission.NewMsgDone(version))
	}
}

// TestV2ClientIdleTimeoutAfterMsgDone checks that the V2 Client does not
// time out waiting for the next run after MsgDone. dmq-node's requester
// starts it only when the peer is promoted again, which may be any time
// later, and the reference waits forever in StIdle. Within a run the Idle
// timeout still applies; that case shows the wait below is long enough to
// see the timer fire.
func TestV2ClientIdleTimeoutAfterMsgDone(t *testing.T) {
	t.Parallel()
	const idleTimeout = 50 * time.Millisecond
	for _, tc := range []struct {
		name        string
		sendDone    bool
		wantTimeout bool
	}{
		{name: "after MsgDone", sendDone: true, wantTimeout: false},
		{name: "within a run", sendDone: false, wantTimeout: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			version := messagesubmission.MessageSubmissionV2MinVersion
			h := restartwindow.New(t, messagesubmission.ProtocolID)
			requests := make(chan struct{}, 4)
			errs := make(chan error, 4)
			cfg := messagesubmission.NewConfig(
				messagesubmission.WithIdleTimeout(idleTimeout),
				messagesubmission.WithRequestMessageIdsFunc(func(
					messagesubmission.CallbackContext, bool, uint16, uint16,
				) {
					requests <- struct{}{}
				}),
			)
			opts := protocol.ProtocolOptions{
				Muxer:     h.Server,
				Logger:    h.Logger("unused hold"),
				ErrorChan: errs,
				Mode:      protocol.ProtocolModeNodeToNode,
				Version:   version,
			}
			client := messagesubmission.NewClient(opts, &cfg)
			client.Start()
			t.Cleanup(func() { _ = client.Stop(context.Background()) })
			h.Start()

			h.Send(messagesubmission.NewMsgRequestMessageIds(version, true, 0, 1))
			select {
			case <-requests:
			case <-time.After(restartwindow.Timeout):
				t.Fatal("the client never handled the request")
			}
			require.NoError(t, client.ReplyNoMessageIds())
			require.Equal(
				t,
				uint(messagesubmission.MessageTypeV2ReplyNoMessageIds),
				h.RecvType("ReplyNoMessageIds"),
			)
			if tc.sendDone {
				h.Send(messagesubmission.NewMsgDone(version))
			}
			select {
			case err := <-errs:
				require.True(t, tc.wantTimeout, "unexpected error: %v", err)
				require.ErrorContains(t, err, "timeout")
				return
			case <-time.After(40 * idleTimeout):
				require.False(t, tc.wantTimeout, "the Idle timeout never fired")
			}
			h.Send(messagesubmission.NewMsgRequestMessageIds(version, true, 0, 1))
			select {
			case <-requests:
			case <-time.After(restartwindow.Timeout):
				t.Fatal("the client never handled the next run's request")
			}
		})
	}
}
