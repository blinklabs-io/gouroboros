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
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// rawFetchServer is a leios-fetch server written as raw segments, so it can
// declare a message larger than a whole segment.
type rawFetchServer struct {
	send   chan *muxer.Segment
	recv   chan *muxer.Segment
	client *Client
	errs   chan error
}

func newRawFetchServer(t *testing.T) *rawFetchServer {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	clientMuxer, serverMuxer := muxer.New(clientConn), muxer.New(serverConn)
	send, recv, _ := serverMuxer.RegisterProtocol(
		ProtocolId,
		muxer.ProtocolRoleResponder,
	)
	require.NotNil(t, send)
	errs := make(chan error, 8)
	cfg := NewConfig()
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     clientMuxer,
		ErrorChan: errs,
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &cfg)
	client.Protocol.EnsureRegistered()
	serverMuxer.Start()
	clientMuxer.Start()
	client.Start()
	t.Cleanup(func() {
		_ = client.Stop(context.Background())
		_ = clientMuxer.Stop(context.Background())
		_ = serverMuxer.Stop(context.Background())
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	return &rawFetchServer{send: send, recv: recv, client: client, errs: errs}
}

// TestBlockReplyOverReferenceLimitIsRefused checks the client's limit on a
// MsgLeiosBlock. The reference refuses one larger than an endorser block's
// 600,000 bytes plus the ten bytes of framing a MsgLeiosBlock adds
// (LeiosFetch/Type.hs byteLimitsLeiosFetch, Leios/Types.hs
// maxEndorserBlockBytesSize). A reply that declares one byte more is refused
// by its declared size, before the rest arrives, and the request returns
// instead of waiting.
func TestBlockReplyOverReferenceLimitIsRefused(t *testing.T) {
	srv := newRawFetchServer(t)
	result := make(chan error, 1)
	go func() {
		_, err := srv.client.BlockRequest(
			context.Background(),
			pcommon.NewPoint(12345, testPointHash(0x01)),
		)
		result <- err
	}()
	select {
	case <-srv.recv:
	case <-time.After(time.Second):
		t.Fatal("the client sent no block request")
	}
	// [1, h'...'] with a byte string header for the rest of a message of
	// MaxLeiosEndorserBlockSize + 11 bytes: the list and tag take two
	// bytes, the byte string's header five.
	header := []byte{0x82, MessageTypeBlock, 0x5a, 0, 0, 0, 0}
	// #nosec G115 -- a few hundred kilobytes
	binary.BigEndian.PutUint32(
		header[3:],
		uint32(pcommon.MaxLeiosEndorserBlockSize+11-len(header)),
	)
	segment := muxer.NewSegment(ProtocolId, header, true)
	require.NotNil(t, segment)
	srv.send <- segment
	select {
	case err := <-srv.errs:
		require.ErrorContains(t, err, "oversized message")
	case <-time.After(time.Second):
		t.Fatal("the over-limit block was not refused by its size")
	}
	select {
	case err := <-result:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("the block request did not return after the refusal")
	}
	require.Equal(
		t,
		pcommon.MaxLeiosEndorserBlockSize+10,
		StateMap[StateBlock].PendingMessageByteLimit,
	)
}
