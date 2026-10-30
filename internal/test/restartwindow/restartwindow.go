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

// Package restartwindow is a test harness for a mini-protocol server that
// restarts itself after the client sends MsgDone. It holds the server inside
// its Done handler, sends the client's next messages into that window, and
// only then lets the restart proceed.
package restartwindow

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

const (
	// sentinelProtocolId is a protocol id no mini-protocol uses.
	sentinelProtocolId uint16 = 4000
	// Timeout bounds every wait so a lost message fails the test.
	Timeout = 5 * time.Second
)

// Harness is a client/server muxer pair connected by a pipe, with the client
// side driven by raw segments.
type Harness struct {
	t          *testing.T
	Client     *muxer.Muxer
	Server     *muxer.Muxer
	protocolId uint16
	clientRecv chan *muxer.Segment
	sentinelRx chan *muxer.Segment
	hold       *holdHandler
}

// New returns a Harness whose client side is registered as an initiator of
// protocolId. The muxers are not started until Start.
func New(t *testing.T, protocolId uint16) *Harness {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	h := &Harness{
		t:          t,
		Client:     muxer.New(clientConn),
		Server:     muxer.New(serverConn),
		protocolId: protocolId,
	}
	t.Cleanup(func() {
		h.Client.Stop()
		h.Server.Stop()
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	_, h.clientRecv, _ = h.Client.RegisterProtocol(
		protocolId,
		muxer.ProtocolRoleInitiator,
	)
	require.NotNil(t, h.clientRecv)
	_, h.sentinelRx, _ = h.Server.RegisterProtocol(
		sentinelProtocolId,
		muxer.ProtocolRoleResponder,
	)
	require.NotNil(t, h.sentinelRx)
	return h
}

// Start starts both muxers.
func (h *Harness) Start() {
	h.Server.Start()
	h.Client.Start()
}

// Logger returns a logger for the server under test. Its first record whose
// message equals holdMessage blocks until Release is called, which keeps the
// server's Done handler from restarting the protocol.
func (h *Harness) Logger(holdMessage string) *slog.Logger {
	h.hold = &holdHandler{
		message: holdMessage,
		reached: make(chan struct{}),
		release: make(chan struct{}),
	}
	h.t.Cleanup(h.Release)
	return slog.New(h.hold)
}

// Release lets the held Done handler continue. It is safe to call more than
// once.
func (h *Harness) Release() {
	if h.hold != nil {
		h.hold.releaseOnce.Do(func() { close(h.hold.release) })
	}
}

// Send encodes msg and writes it to the server as the client. It writes the
// segment itself rather than queueing it on a registered protocol: each
// protocol's queue is drained by its own goroutine, so queued segments of
// different protocols reach the wire in no fixed order, and Settle could
// return before this segment was written.
func (h *Harness) Send(msg any) {
	h.t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(h.t, err)
	h.write(muxer.NewSegment(h.protocolId, data, false))
}

func (h *Harness) write(seg *muxer.Segment) {
	h.t.Helper()
	require.NotNil(h.t, seg)
	require.NoError(h.t, h.Client.Send(seg))
}

// WaitHeld blocks until the server's Done handler has reached the hold.
func (h *Harness) WaitHeld() {
	h.t.Helper()
	select {
	case <-h.hold.reached:
	case <-time.After(Timeout):
		h.t.Fatal("server never reached its Done handler")
	}
}

// Settle returns once the server muxer has routed every segment the client
// sent before the call, by sending a segment on a second protocol and waiting
// for it: the muxer routes segments in wire order.
func (h *Harness) Settle() {
	h.t.Helper()
	h.write(muxer.NewSegment(sentinelProtocolId, []byte{0x80}, false))
	select {
	case <-h.sentinelRx:
	case <-time.After(Timeout):
		h.t.Fatal("sentinel never reached the server muxer")
	}
}

// RecvType waits for the next message the server sends to the client and
// returns its CBOR message type. what names the expected reply for the
// failure message.
func (h *Harness) RecvType(what string) uint {
	h.t.Helper()
	select {
	case seg, ok := <-h.clientRecv:
		require.True(h.t, ok, "client receive channel closed")
		var msg []cbor.RawMessage
		_, err := cbor.Decode(seg.Payload, &msg)
		require.NoError(h.t, err)
		require.NotEmpty(h.t, msg)
		var msgType uint
		_, err = cbor.Decode(msg[0], &msgType)
		require.NoError(h.t, err)
		return msgType
	case <-time.After(Timeout):
		h.t.Fatalf("no reply from restarted server: %s", what)
		return 0
	}
}

type holdHandler struct {
	message     string
	reached     chan struct{}
	release     chan struct{}
	reachedOnce sync.Once
	releaseOnce sync.Once
}

func (*holdHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *holdHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Message == h.message {
		h.reachedOnce.Do(func() {
			close(h.reached)
			<-h.release
		})
	}
	return nil
}

func (h *holdHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *holdHandler) WithGroup(string) slog.Handler { return h }
