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

package protocol

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// budgetProtocol is a server-role protocol whose single state accepts any
// number of type 0 messages, with a handler that blocks until released.
type budgetProtocol struct {
	p         *Protocol
	m         *muxer.Muxer
	errorChan chan error
	handled   chan struct{}
	release   func()
}

func newBudgetProtocol(t *testing.T, entry StateMapEntry) *budgetProtocol {
	t.Helper()
	const requestType = uint8(0)
	stateIdle := NewState(1, "Idle")
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
	}()
	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	// Registered after the connection teardown below, so it runs first and a
	// failing test cannot leave a handler blocking the protocol's Stop.
	t.Cleanup(func() {
		m.Stop()
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	t.Cleanup(release)
	bp := &budgetProtocol{
		m:         m,
		errorChan: make(chan error, 1),
		handled:   make(chan struct{}, 16),
		release:   release,
	}
	entry.Agency = AgencyClient
	entry.Transitions = []StateTransition{
		{MsgType: requestType, NewState: stateIdle},
	}
	bp.p = New(ProtocolConfig{
		Name:       "pending-budget",
		ProtocolId: 1,
		ErrorChan:  bp.errorChan,
		Muxer:      m,
		Role:       ProtocolRoleServer,
		MessageHandlerFunc: func(Message) error {
			<-releaseCh
			bp.handled <- struct{}{}
			return nil
		},
		MessageFromCborFunc: func(msgType uint, _ []byte) (Message, error) {
			return &MessageBase{MessageType: uint8(msgType)}, nil
		},
		StateMap:     StateMap{stateIdle: entry},
		InitialState: stateIdle,
		// The muxer's ingress queue is not under test here.
		IngressLimit: 1 << 20,
	})
	bp.p.Start()
	t.Cleanup(bp.p.Stop)
	return bp
}

// sendMessage delivers one message of exactly size bytes: a two element CBOR
// array of the message type and a byte string.
func (bp *budgetProtocol) sendMessage(t *testing.T, size int) {
	t.Helper()
	// 0x82 0x00 is the array header and type; the byte string header is
	// 0x58 len (2 bytes) for lengths up to 255, then len bytes.
	const header = 4
	require.Greater(t, size, header)
	require.LessOrEqual(t, size-header, 255)
	payload := append(
		[]byte{0x82, 0x00, 0x58, byte(size - header)},
		bytes.Repeat([]byte{0xAA}, size-header)...,
	)
	require.Len(t, payload, size)
	segment := muxer.NewSegment(1, payload, false)
	require.NotNil(t, segment)
	select {
	case bp.p.muxerRecvChan <- segment:
	case <-time.After(5 * time.Second):
		t.Fatal("protocol did not accept the peer message")
	}
}

func (bp *budgetProtocol) pendingRecv() int {
	bp.p.pendingBytesMu.Lock()
	defer bp.p.pendingBytesMu.Unlock()
	return bp.p.pendingRecvBytes
}

// TestPendingReceiveByteBudgetBoundsQueuedMessages checks a state with a
// receive budget and no message size limit holds back messages past the
// budget while the handler is stalled, then admits them as it drains.
func TestPendingReceiveByteBudgetBoundsQueuedMessages(t *testing.T) {
	t.Parallel()

	bp := newBudgetProtocol(t, StateMapEntry{PendingReceiveByteBudget: 100})
	const size = 40
	for range 3 {
		bp.sendMessage(t, size)
	}
	require.Eventually(t, func() bool {
		return bp.pendingRecv() >= 2*size
	}, 5*time.Second, time.Millisecond)
	// The third message does not fit: the budget holds across a window in
	// which the read loop would have admitted it.
	require.Never(t, func() bool {
		return bp.pendingRecv() > 100
	}, 200*time.Millisecond, time.Millisecond,
		"queued messages exceeded the receive budget")
	select {
	case err := <-bp.errorChan:
		t.Fatalf("budget waiting must not fail the protocol: %s", err)
	default:
	}

	bp.release()
	for range 3 {
		select {
		case <-bp.handled:
		case err := <-bp.errorChan:
			t.Fatalf("protocol failed while draining: %s", err)
		case <-time.After(5 * time.Second):
			t.Fatal("message held by the budget was never handled")
		}
	}
	require.Eventually(t, func() bool {
		return bp.pendingRecv() == 0
	}, 5*time.Second, time.Millisecond)
}

// TestPendingReceiveByteBudgetDoesNotLimitMessageSize checks a message larger
// than the whole budget is still accepted, so the budget changes how much is
// queued and not which messages the protocol takes.
func TestPendingReceiveByteBudgetDoesNotLimitMessageSize(t *testing.T) {
	t.Parallel()

	bp := newBudgetProtocol(t, StateMapEntry{PendingReceiveByteBudget: 50})
	bp.sendMessage(t, 200)
	require.Eventually(t, func() bool {
		return bp.pendingRecv() == 200
	}, 5*time.Second, time.Millisecond)
	bp.release()
	select {
	case <-bp.handled:
	case err := <-bp.errorChan:
		t.Fatalf("message larger than the budget was rejected: %s", err)
	case <-time.After(5 * time.Second):
		t.Fatal("message larger than the budget was never handled")
	}
}

// TestPendingMessageByteLimitStillRejectsOversizedMessages checks the
// receive budget does not stand in for a state's message size limit.
func TestPendingMessageByteLimitStillRejectsOversizedMessages(t *testing.T) {
	t.Parallel()

	bp := newBudgetProtocol(t, StateMapEntry{
		PendingMessageByteLimit:  100,
		PendingReceiveByteBudget: 1000,
	})
	bp.sendMessage(t, 200)
	select {
	case err := <-bp.errorChan:
		require.ErrorContains(t, err, "oversized message")
	case <-time.After(5 * time.Second):
		t.Fatal("message past the state's size limit was not rejected")
	}
}
