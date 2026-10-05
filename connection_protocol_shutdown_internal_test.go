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

package ouroboros

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagenotification"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagesubmission"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestConnectionShutdownStopsConstructedDMQProtocols(t *testing.T) {
	defer goleak.VerifyNone(t)
	for range 10 {
		conn, err := New()
		require.NoError(t, err)
		protoOptions := protocol.ProtocolOptions{}
		conn.localMessageSubmission = localmessagesubmission.New(protoOptions, nil)
		conn.localMessageNotification = localmessagenotification.New(protoOptions, nil)
		conn.protocolsReady = true

		protocolDone := []<-chan struct{}{
			conn.localMessageSubmission.Client.DoneChan(),
			conn.localMessageSubmission.Server.DoneChan(),
			conn.localMessageNotification.Client.DoneChan(),
			conn.localMessageNotification.Server.DoneChan(),
		}
		conn.shutdown()

		for _, done := range protocolDone {
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("constructed protocol did not complete connection shutdown")
			}
		}
	}
}

func TestConnectionRefusesProtocolSetupAfterShutdownBegins(t *testing.T) {
	conn, err := New()
	require.NoError(t, err)
	conn.doneChan = make(chan any)
	close(conn.doneChan)

	require.ErrorContains(
		t,
		conn.lockProtocolSetup(),
		"connection shutting down",
	)
	require.True(
		t,
		conn.protocolMu.TryLock(),
		"protocol setup left its lock held",
	)
	conn.protocolMu.Unlock()
}
