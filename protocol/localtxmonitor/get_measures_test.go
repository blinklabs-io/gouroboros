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

package localtxmonitor_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/test/restartwindow"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/stretchr/testify/require"
)

// TestServerAnswersGetMeasures sends the wire bytes of MsgAcquire and then
// MsgGetMeasures ([11]) and requires MsgReplyGetMeasures ([12, ...]). The
// reference node decodes MsgGetMeasures in StAcquired and answers it.
func TestServerAnswersGetMeasures(t *testing.T) {
	t.Parallel()
	h := restartwindow.New(t, localtxmonitor.ProtocolId)
	server := localtxmonitor.NewServer(
		protocol.ProtocolOptions{
			Muxer: h.Server,
			Mode:  protocol.ProtocolModeNodeToClient,
		},
		&localtxmonitor.Config{
			GetMempoolFunc: func(
				localtxmonitor.CallbackContext,
			) (uint64, uint32, []localtxmonitor.TxAndEraId, error) {
				return 1, 1000, nil, nil
			},
		},
	)
	server.Start()
	t.Cleanup(server.Stop)
	h.Start()

	h.Send([]uint{1})
	require.Equal(t, uint(2), h.RecvType("Acquired"))
	h.Send([]uint{11})
	require.Equal(t, uint(12), h.RecvType("ReplyGetMeasures"))
}
