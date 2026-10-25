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

package leiosfetch

import (
	"bytes"
	"encoding/binary"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// pipelineMarker is a small CBOR array that identifies the request, by slot,
// that a reply answers.
func pipelineMarker(slot uint64) cbor.RawMessage {
	return cbor.RawMessage{0x82, 0x01, 0x18, byte(slot)}
}

// pipelineTestServer is the client end of a connection to a started server.
type pipelineTestServer struct {
	conn           net.Conn
	protocolErrors <-chan error
	muxerErrors    <-chan error
	// release unblocks the held first callback.
	release func()
	// holding is closed once the first callback is running.
	holding <-chan struct{}
}

// newPipelineTestServer starts a server whose block and block transactions
// callbacks answer with the request's slot as the reply marker. The first
// callback to run blocks until release is called (the cleanup calls it too,
// so a failing test cannot hang), which holds the server in a
// reply-outstanding state while the test pipelines more requests.
func newPipelineTestServer(t *testing.T) pipelineTestServer {
	t.Helper()
	connA, connB := net.Pipe()
	m := muxer.New(connA)
	protocolErrors := make(chan error, 1)
	release := make(chan struct{})
	holding := make(chan struct{})
	var calls atomic.Int32
	hold := func() {
		if calls.Add(1) == 1 {
			close(holding)
			<-release
		}
	}
	cfg := NewConfig(
		WithBlockRequestFunc(func(
			_ CallbackContext,
			point pcommon.Point,
		) (protocol.Message, error) {
			hold()
			return NewMsgBlock(
				pipelineMarker(point.Slot),
			), nil
		}),
		WithBlockTxsRequestFunc(func(
			_ CallbackContext,
			point pcommon.Point,
			_ map[uint16]uint64,
		) (protocol.Message, error) {
			hold()
			return NewMsgBlockTxs([]cbor.RawMessage{
				pipelineMarker(point.Slot),
			}), nil
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
			RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		},
		ErrorChan: protocolErrors,
		Muxer:     m,
	}, &cfg)
	server.Start()
	m.Start()
	var releaseOnce sync.Once
	releaseFn := func() { releaseOnce.Do(func() { close(release) }) }
	// Release the held callback before the teardown waits on its handler.
	t.Cleanup(func() {
		releaseFn()
		server.ProtocolInstance().Stop()
		m.Stop()
		_ = connA.Close()
		_ = connB.Close()
	})
	return pipelineTestServer{
		conn:           connB,
		protocolErrors: protocolErrors,
		muxerErrors:    m.ErrorChan(),
		release:        releaseFn,
		holding:        holding,
	}
}

// waitForHold waits until the server is running the first callback.
func (s pipelineTestServer) waitForHold(t *testing.T) {
	t.Helper()
	select {
	case <-s.holding:
	case err := <-s.protocolErrors:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start the first request")
	}
}

// pipelineReplyReader decodes replies from a connection. The server may batch
// several replies into one mux segment, so it keeps the bytes of a segment
// that follow the message it returned.
type pipelineReplyReader struct {
	conn net.Conn
	buf  []byte
}

func (r *pipelineReplyReader) next(t *testing.T) protocol.Message {
	t.Helper()
	require.NoError(t, r.conn.SetReadDeadline(time.Now().Add(5*time.Second)))
	for {
		if len(r.buf) > 0 {
			var fields []cbor.RawMessage
			// An undecodable prefix is a partial message that continues in
			// the next segment.
			if n, err := cbor.Decode(r.buf, &fields); err == nil && n > 0 {
				require.NotEmpty(t, fields)
				var tag uint
				_, err = cbor.Decode(fields[0], &tag)
				require.NoError(t, err)
				msg, err := NewMsgFromCbor(tag, r.buf[:n])
				require.NoError(t, err)
				r.buf = r.buf[n:]
				return msg
			}
		}
		segment, err := readLeiosFetchTestSegment(t, r.conn)
		require.NoError(t, err)
		r.buf = append(r.buf, segment.Payload...)
	}
}

// writePipelineTestRequest writes one request segment and returns the write
// error, so it is safe to call from a goroutine that may outlive a failed
// connection.
func writePipelineTestRequest(conn net.Conn, msg protocol.Message) error {
	data, err := cbor.Encode(msg)
	if err != nil {
		return err
	}
	segment := muxer.NewSegment(ProtocolId, data, false)
	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.BigEndian, segment.SegmentHeader); err != nil {
		return err
	}
	buf.Write(segment.Payload)
	_, err = conn.Write(buf.Bytes())
	return err
}

// A reference client pipelines further Block and BlockTxs requests behind one
// the server has not answered. The server must accept them and reply to each
// in request order instead of failing the connection.
func TestServerAnswersPipelinedRequestsInOrder(t *testing.T) {
	t.Parallel()
	point := func(slot uint64) pcommon.Point {
		return pcommon.NewPoint(slot, testPointHash(byte(slot)))
	}
	bitmaps := map[uint16]uint64{0: 1}
	type request struct {
		txs  bool
		slot uint64
	}
	tests := []struct {
		name     string
		requests []request
	}{
		{"block txs behind block txs", []request{{true, 1}, {true, 2}, {true, 3}}},
		{"block behind block", []request{{false, 1}, {false, 2}, {false, 3}}},
		{
			"mixed",
			[]request{{true, 1}, {false, 2}, {true, 3}, {false, 4}, {false, 5}},
		},
	}
	// The reference client keeps up to 150 requests outstanding on one
	// connection (leiosFetchPipelineDepth).
	deep := make([]request, 150)
	for i := range deep {
		deep[i] = request{txs: i%3 != 0, slot: uint64(i + 1)}
	}
	tests = append(tests, struct {
		name     string
		requests []request
	}{"reference pipeline depth", deep})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := newPipelineTestServer(t)
			send := func(r request) error {
				if r.txs {
					return writePipelineTestRequest(
						srv.conn, NewMsgBlockTxsRequest(point(r.slot), bitmaps),
					)
				}
				return writePipelineTestRequest(
					srv.conn, NewMsgBlockRequest(point(r.slot)),
				)
			}
			require.NoError(t, send(tc.requests[0]))
			srv.waitForHold(t)
			// The first reply is outstanding while the rest arrive. A pipe
			// write returns once the server has read it, so every request is
			// on the server before the first reply is released.
			sent := make(chan error, 1)
			go func() {
				for _, r := range tc.requests[1:] {
					if err := send(r); err != nil {
						sent <- err
						return
					}
				}
				sent <- nil
			}()
			select {
			case err := <-sent:
				require.NoError(t, err)
			case err := <-srv.protocolErrors:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("server did not read the pipelined requests")
			}
			srv.release()
			replies := &pipelineReplyReader{conn: srv.conn}
			for i, r := range tc.requests {
				msg := replies.next(t)
				if r.txs {
					got, ok := msg.(*MsgBlockTxs)
					require.True(t, ok, "reply %d is %T", i, msg)
					require.Equal(t, []cbor.RawMessage{pipelineMarker(r.slot)}, got.TxsRaw)
					continue
				}
				got, ok := msg.(*MsgBlock)
				require.True(t, ok, "reply %d is %T", i, msg)
				require.Equal(t, pipelineMarker(r.slot), got.BlockRaw)
			}
			select {
			case err := <-srv.protocolErrors:
				require.NoError(t, err)
			default:
			}
		})
	}
}

// Only the requests a reference client pipelines are accepted: a request of
// another kind sent while a reply is outstanding is still a protocol
// violation.
func TestServerRejectsUnpipelinedRequestWhileReplying(t *testing.T) {
	t.Parallel()
	srv := newPipelineTestServer(t)
	require.NoError(t, writePipelineTestRequest(srv.conn, NewMsgBlockRequest(
		pcommon.NewPoint(1, testPointHash(1)),
	)))
	srv.waitForHold(t)
	go func() { _ = writePipelineTestRequest(srv.conn, NewMsgVotesRequest(nil)) }()
	select {
	case err := <-srv.protocolErrors:
		require.ErrorContains(t, err, "message type")
	case <-time.After(5 * time.Second):
		t.Fatal("votes request pipelined behind a block request was accepted")
	}
}

// Pipelined requests are held in bounded queues while a reply is outstanding.
// A client that keeps sending past them overruns the protocol's ingress limit,
// and the connection fails instead of the queue growing.
func TestServerFailsConnectionWhenPipelinedRequestsOverrunIngress(t *testing.T) {
	t.Parallel()
	srv := newPipelineTestServer(t)
	require.NoError(t, writePipelineTestRequest(srv.conn, NewMsgBlockRequest(
		pcommon.NewPoint(1, testPointHash(1)),
	)))
	srv.waitForHold(t)
	// The largest request the decoder accepts, so a few hundred of them
	// exceed the receive queue and the ingress limit together.
	bitmaps := make(map[uint16]uint64, maxWireBlockBitmaps)
	for i := range uint16(maxWireBlockBitmaps) {
		bitmaps[i] = 1
	}
	request := NewMsgBlockTxsRequest(
		pcommon.NewPoint(2, testPointHash(2)),
		bitmaps,
	)
	data, err := cbor.Encode(request)
	require.NoError(t, err)
	require.Less(t, len(data), MaxIdleMessageBytes)
	count := 4*MaxBlockMessageBytes/len(data) + 1
	go func() {
		for range count {
			if writePipelineTestRequest(srv.conn, request) != nil {
				return
			}
		}
	}()
	select {
	case err := <-srv.muxerErrors:
		require.ErrorIs(t, err, muxer.ErrIngressOverflow)
	case err := <-srv.protocolErrors:
		t.Fatalf("protocol failed before the ingress limit: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("pipelined requests past the ingress limit were accepted")
	}
}
