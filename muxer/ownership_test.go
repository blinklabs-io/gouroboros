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
	"go.uber.org/goleak"
)

func TestUnregisterRemovesBothProtocolDirections(t *testing.T) {
	defer goleak.VerifyNone(t)
	local, remote := net.Pipe()
	m := New(local)
	t.Cleanup(func() {
		_ = remote.Close()
		_ = local.Close()
	})
	const protocolID = uint16(0x42)
	_, _, _ = m.RegisterProtocol(protocolID, ProtocolRoleInitiator)

	m.UnregisterProtocol(protocolID, ProtocolRoleInitiator)

	m.protocolReceiversMutex.Lock()
	_, receiverExists := m.protocolReceivers[protocolID]
	_, senderExists := m.protocolSenders[protocolID]
	m.protocolReceiversMutex.Unlock()
	require.False(t, receiverExists)
	require.False(t, senderExists)
	m.Stop()
	require.Eventually(t, func() bool {
		select {
		case <-m.doneChan:
			return true
		default:
			return false
		}
	}, time.Second, time.Millisecond)
}
