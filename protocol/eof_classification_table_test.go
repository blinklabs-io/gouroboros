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

package protocol_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/leiosvotes"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagenotification"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagesubmission"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/messagesubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/perasvotes"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

type protocolInstance interface {
	ProtocolInstance() *protocol.Protocol
}

// closeClassificationProtocols builds the client and server of every
// mini-protocol package, each with the StateMap its constructor installs.
func closeClassificationProtocols(t *testing.T) map[string]*protocol.Protocol {
	t.Helper()
	n2n := protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode}
	n2c := protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToClient}
	hsCfg := handshake.NewConfig()
	servers := map[string]protocolInstance{
		"blockfetch":  blockfetch.NewServer(n2n, nil),
		"chainsync":   chainsync.NewServer(n2n, nil),
		"keepalive":   keepalive.NewServer(n2n, nil),
		"leiosfetch":  leiosfetch.NewServer(n2n, nil),
		"leiosnotify": leiosnotify.NewServer(n2n, nil),
		"leiosvotes":  leiosvotes.NewServer(n2n, nil),
		"localmessagenotification": localmessagenotification.NewServer(
			n2c,
			nil,
		),
		"localmessagesubmission": localmessagesubmission.NewServer(n2c, nil),
		"localstatequery":        localstatequery.NewServer(n2c, nil),
		"localtxmonitor":         localtxmonitor.NewServer(n2c, nil),
		"localtxsubmission":      localtxsubmission.NewServer(n2c, nil),
		"messagesubmission":      messagesubmission.NewServer(n2n, nil),
		"peersharing":            peersharing.NewServer(n2n, nil),
		"perasvotes":             perasvotes.NewServer(n2n, nil),
		"txsubmission":           txsubmission.NewServer(n2n, nil),
	}
	ret := map[string]*protocol.Protocol{
		"blockfetch/client":  blockfetch.NewClient(n2n, nil).ProtocolInstance(),
		"chainsync/client":   chainsync.NewClient(n2n, nil).ProtocolInstance(),
		"handshake/client":   handshake.NewClient(n2n, &hsCfg).Protocol,
		"handshake/server":   handshake.NewServer(n2n, &hsCfg).Protocol,
		"keepalive/client":   keepalive.NewClient(n2n, nil).Protocol,
		"leiosfetch/client":  leiosfetch.NewClient(n2n, nil).Protocol,
		"leiosnotify/client": leiosnotify.NewClient(n2n, nil).Protocol,
		"leiosvotes/client":  leiosvotes.NewClient(n2n, nil).Protocol,
		"localmessagenotification/client": localmessagenotification.NewClient(
			n2c, nil,
		).Protocol,
		"localmessagesubmission/client": localmessagesubmission.NewClient(
			n2c, nil,
		).Protocol,
		"localstatequery/client": localstatequery.NewClient(
			n2c,
			nil,
		).Protocol,
		"localtxmonitor/client": localtxmonitor.NewClient(n2c, nil).Protocol,
		"localtxsubmission/client": localtxsubmission.NewClient(
			n2c,
			nil,
		).Protocol,
		"messagesubmission/client": messagesubmission.NewClient(
			n2n,
			nil,
		).Protocol,
		"peersharing/client":  peersharing.NewClient(n2n, nil).Protocol,
		"perasvotes/client":   perasvotes.NewClient(n2n, nil).Protocol,
		"txsubmission/client": txsubmission.NewClient(n2n, nil).Protocol,
	}
	for name, s := range servers {
		ret[name+"/server"] = s.ProtocolInstance()
	}
	return ret
}

// TestCloseClassificationMatchesAgency checks IsInTerminalOrIdleState in
// every state of every mini-protocol's StateMap, for both roles: a peer
// close is clean in a Done state (AgencyNone, which must have no
// transitions) and in the initial state with no pipelined request
// unanswered, and an error in every other state, which covers every state
// where the peer holds agency after a request. Once the protocol is stopped
// every state is clean.
func TestCloseClassificationMatchesAgency(t *testing.T) {
	t.Parallel()
	for name, p := range closeClassificationProtocols(t) {
		require.NotNil(t, p, name)
		stateMap, initial := p.ClassificationConfigForTest()
		_, ok := stateMap[initial]
		require.True(
			t,
			ok,
			"%s: initial state %s not in StateMap",
			name,
			initial,
		)
		for state, entry := range stateMap {
			terminal := entry.Agency == protocol.AgencyNone
			if terminal {
				require.Empty(
					t,
					entry.Transitions,
					"%s: AgencyNone state %s must be terminal",
					name,
					state,
				)
			}
			p.SetCurrentStateForTest(state)
			p.SetPendingPipelinedRequestsForTest(0)
			require.Equal(
				t,
				terminal || state == initial,
				p.IsInTerminalOrIdleState(),
				"%s in %s, nothing pipelined",
				name,
				state,
			)
			// A pipelined request is unanswered: only Done is clean.
			p.SetPendingPipelinedRequestsForTest(1)
			require.Equal(
				t,
				terminal,
				p.IsInTerminalOrIdleState(),
				"%s in %s, a pipelined request unanswered",
				name,
				state,
			)
		}
		require.NoError(t, p.Stop(context.Background()))
		for state := range stateMap {
			p.SetCurrentStateForTest(state)
			require.True(
				t,
				p.IsInTerminalOrIdleState(),
				"%s stopped in %s",
				name,
				state,
			)
		}
	}
}
