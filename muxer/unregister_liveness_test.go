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
	"bytes"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeUnregisterTestSegment(conn net.Conn, segment *Segment) error {
	buf := bytes.NewBuffer(nil)
	if err := binary.Write(buf, binary.BigEndian, segment.SegmentHeader); err != nil {
		return err
	}
	if _, err := buf.Write(segment.Payload); err != nil {
		return err
	}
	data := buf.Bytes()
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		data = data[n:]
	}
	return nil
}

func TestUnregisterProtocolWakesBlockedDelivery(t *testing.T) {
	const protocolId = 10

	localConn, remoteConn := net.Pipe()
	m := New(localConn)
	_, recvChan, _ := m.RegisterProtocol(protocolId, ProtocolRoleResponder)
	m.Start()
	t.Cleanup(func() {
		m.Stop()
		_ = remoteConn.Close()
		_ = localConn.Close()
	})

	segment := NewSegment(protocolId, []byte{0x81, 0x02}, false)
	require.NotNil(t, segment)
	for range cap(recvChan) {
		require.NoError(t, writeUnregisterTestSegment(remoteConn, segment))
	}
	require.Eventually(t, func() bool {
		return len(recvChan) == cap(recvChan)
	}, time.Second, time.Millisecond)

	m.protocolReceiversMutex.Lock()
	receiver := m.protocolReceivers[protocolId][ProtocolRoleResponder]
	m.protocolReceiversMutex.Unlock()
	require.NotNil(t, receiver)

	require.NoError(t, writeUnregisterTestSegment(remoteConn, segment))
	require.Eventually(t, func() bool {
		if receiver.mu.TryLock() {
			receiver.mu.Unlock()
			return false
		}
		return true
	}, time.Second, time.Millisecond)

	unregistered := make(chan struct{})
	go func() {
		m.UnregisterProtocol(protocolId, ProtocolRoleResponder)
		close(unregistered)
	}()
	select {
	case <-unregistered:
	case <-time.After(time.Second):
		t.Fatal("protocol unregistration blocked behind receiver delivery")
	}
	for range cap(recvChan) {
		_, ok := <-recvChan
		require.True(t, ok)
	}
	_, ok := <-recvChan
	require.False(t, ok, "unregistration must close the receiver channel")

	_, replacementRecv, _ := m.RegisterProtocol(
		protocolId,
		ProtocolRoleResponder,
	)
	require.NoError(t, writeUnregisterTestSegment(remoteConn, segment))
	select {
	case replacement := <-replacementRecv:
		require.Equal(t, uint16(protocolId), replacement.GetProtocolId())
	case <-time.After(time.Second):
		t.Fatal("muxer did not deliver after protocol re-registration")
	}
}
