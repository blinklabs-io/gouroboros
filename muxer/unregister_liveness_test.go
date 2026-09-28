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

// TestUnregisterProtocolWakesBlockedDelivery covers a receiver whose
// consumer never drains. Writes over the synchronous net.Pipe must keep
// returning however far ingress runs past the receive channel's capacity,
// the excess must be held in the receiver's own queue, and unregistering
// must not deadlock behind the stuck consumer.
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

	// Never drain recvChan. Write well past its capacity: if the read loop
	// still blocked delivering to a full channel, one of these writes over
	// the unbuffered net.Pipe would hang forever waiting for a read that
	// never comes.
	const totalSegments = 25
	require.Greater(t, totalSegments, cap(recvChan))
	written := make(chan struct{})
	go func() {
		defer close(written)
		for range totalSegments {
			if err := writeUnregisterTestSegment(remoteConn, segment); err != nil {
				return
			}
		}
	}()
	select {
	case <-written:
	case <-time.After(2 * time.Second):
		t.Fatal("read loop blocked delivering to a full protocol channel")
	}

	require.Eventually(t, func() bool {
		return len(recvChan) == cap(recvChan)
	}, time.Second, time.Millisecond)

	m.protocolReceiversMutex.Lock()
	receiver := m.protocolReceivers[protocolId][ProtocolRoleResponder]
	m.protocolReceiversMutex.Unlock()
	require.NotNil(t, receiver)

	// The segments beyond the channel's capacity are held in the
	// protocol's own ingress queue, not blocking anything shared.
	require.Eventually(t, func() bool {
		receiver.mu.Lock()
		defer receiver.mu.Unlock()
		return receiver.pendingBytes > 0
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
