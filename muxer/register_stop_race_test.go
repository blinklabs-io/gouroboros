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

package muxer

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestRegisterProtocolRacingStopDoesNotLeak runs a complete Stop, including
// the read loop's exit sweep of every receiver, after RegisterProtocol has
// passed its first shutdown check and before it inserts the new receiver.
// The registration must be refused, and the muxer must still finish
// shutting down: a receiver the sweep never saw would leave its forward
// goroutine running, and ErrorChan would never close.
func TestRegisterProtocolRacingStopDoesNotLeak(t *testing.T) {
	t.Parallel()

	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() {
		_ = localConn.Close()
		_ = remoteConn.Close()
	})
	m := New(localConn)
	_, _, _ = m.RegisterProtocol(1, ProtocolRoleInitiator)

	m.registerHook = func() {
		m.Stop()
		require.Eventually(t, func() bool {
			m.protocolReceiversMutex.Lock()
			defer m.protocolReceiversMutex.Unlock()
			return len(m.protocolReceivers[1]) == 0
		}, 2*time.Second, time.Millisecond, "read loop exit sweep did not run")
	}
	sendChan, recvChan, doneChan := m.RegisterProtocol(2, ProtocolRoleInitiator)
	m.registerHook = nil

	select {
	case _, ok := <-m.ErrorChan():
		for ok {
			_, ok = <-m.ErrorChan()
		}
	case <-time.After(2 * time.Second):
		t.Fatal("muxer did not finish shutting down: a receiver outlived Stop")
	}
	require.Nil(t, sendChan, "registration during shutdown must be refused")
	require.Nil(t, recvChan, "registration during shutdown must be refused")
	require.Nil(t, doneChan, "registration during shutdown must be refused")
}
