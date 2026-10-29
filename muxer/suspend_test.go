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

func newSuspendPair(t *testing.T) (*Muxer, *Muxer) {
	t.Helper()
	localConn, remoteConn := net.Pipe()
	local := New(localConn)
	remote := New(remoteConn)
	t.Cleanup(func() {
		local.Stop()
		remote.Stop()
		_ = localConn.Close()
		_ = remoteConn.Close()
	})
	local.Start()
	remote.Start()
	return local, remote
}

// TestSuspendProtocolKeepsIngressForNextRegistration sends a segment while a
// role is suspended and requires the next registration of the role to receive
// it, and the carried data to be handed over once.
func TestSuspendProtocolKeepsIngressForNextRegistration(t *testing.T) {
	t.Parallel()
	local, remote := newSuspendPair(t)
	const protocolId, sentinelId = 7, 8

	_, _, _ = local.RegisterProtocol(protocolId, ProtocolRoleResponder)
	_, sentinelRecv, _ := local.RegisterProtocol(
		sentinelId,
		ProtocolRoleResponder,
	)
	protoSend, _, _ := remote.RegisterProtocol(
		protocolId,
		ProtocolRoleInitiator,
	)
	sentinelSend, _, _ := remote.RegisterProtocol(
		sentinelId,
		ProtocolRoleInitiator,
	)

	require.True(
		t,
		local.SuspendProtocol(
			protocolId,
			ProtocolRoleResponder,
			[]byte("carry"),
		),
	)
	protoSend <- NewSegment(protocolId, []byte("during"), false)
	sentinelSend <- NewSegment(sentinelId, []byte("s"), false)
	select {
	case <-sentinelRecv:
	case <-time.After(5 * time.Second):
		t.Fatal("sentinel never routed")
	}

	_, recv, _ := local.RegisterProtocol(protocolId, ProtocolRoleResponder)
	require.NotNil(t, recv)
	require.Equal(
		t,
		[]byte("carry"),
		local.TakeCarriedIngress(protocolId, ProtocolRoleResponder),
	)
	require.Nil(t, local.TakeCarriedIngress(protocolId, ProtocolRoleResponder))
	select {
	case seg := <-recv:
		require.Equal(t, []byte("during"), seg.Payload)
	case <-time.After(5 * time.Second):
		t.Fatal("segment sent while the role was suspended was dropped")
	}
}

// TestSuspendProtocolUnregisteredRole requires a role that is not registered
// to be left alone, so the caller can fall back to UnregisterProtocol.
func TestSuspendProtocolUnregisteredRole(t *testing.T) {
	t.Parallel()
	local, _ := newSuspendPair(t)
	require.False(t, local.SuspendProtocol(9, ProtocolRoleResponder, nil))
}

// TestUnregisterAfterSuspendDropsIngress requires a suspended role that is
// unregistered instead of resumed to behave like any unregistered role: later
// segments are dropped without failing the connection.
func TestUnregisterAfterSuspendDropsIngress(t *testing.T) {
	t.Parallel()
	local, remote := newSuspendPair(t)
	const protocolId, sentinelId = 7, 8

	_, _, _ = local.RegisterProtocol(protocolId, ProtocolRoleResponder)
	_, sentinelRecv, _ := local.RegisterProtocol(
		sentinelId,
		ProtocolRoleResponder,
	)
	protoSend, _, _ := remote.RegisterProtocol(
		protocolId,
		ProtocolRoleInitiator,
	)
	sentinelSend, _, _ := remote.RegisterProtocol(
		sentinelId,
		ProtocolRoleInitiator,
	)
	require.True(
		t,
		local.SuspendProtocol(protocolId, ProtocolRoleResponder, nil),
	)
	local.UnregisterProtocol(protocolId, ProtocolRoleResponder)
	protoSend <- NewSegment(protocolId, []byte("late"), false)
	sentinelSend <- NewSegment(sentinelId, []byte("s"), false)
	select {
	case <-sentinelRecv:
	case err := <-local.ErrorChan():
		t.Fatalf("connection failed: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("sentinel never routed")
	}
}
