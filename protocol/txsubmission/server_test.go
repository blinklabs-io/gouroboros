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

package txsubmission

import (
	"net"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestReplyHandlersSynchronizeProtocolAccessDuringRestart(t *testing.T) {
	const iterations = 10_000

	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
			RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
		},
	}, nil)
	server.requestTxIdsResultChan = make(
		chan requestTxIdsResult,
		iterations,
	)
	server.requestTxsResultChan = make(chan []TxBody, iterations)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			server.initProtocol()
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			server.handleReplyTxIds(NewMsgReplyTxIds(nil))
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			server.handleReplyTxs(NewMsgReplyTxs(nil))
		}
	}()

	close(start)
	wg.Wait()

	require.Len(t, server.requestTxIdsResultChan, iterations)
	require.Len(t, server.requestTxsResultChan, iterations)
}
