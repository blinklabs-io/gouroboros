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

package perasvotes

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// The reference codec's encodings of the two await messages
// (ObjectDiffusion/Codec.hs lines 166-171): a one-element list holding the
// tag.
var (
	wireServerIdle = []byte{0x81, 0x06}
	wireAwaitReply = []byte{0x81, 0x07}
)

// rawPeer speaks ObjectDiffusion as raw CBOR on the far side of a muxer, so
// it can send what the reference sends whether or not gOuroboros knows the
// message.
type rawPeer struct {
	send       chan *muxer.Segment
	recv       chan *muxer.Segment
	isResponse bool
	pending    []byte
}

// newRawPeerPair connects a raw peer to a muxer for the gOuroboros
// endpoint. local is the role of the gOuroboros endpoint.
func newRawPeerPair(
	t *testing.T,
	local muxer.ProtocolRole,
) (*rawPeer, *muxer.Muxer, chan error) {
	t.Helper()
	localConn, peerConn := net.Pipe()
	localMuxer, peerMuxer := muxer.New(localConn), muxer.New(peerConn)
	peerRole := muxer.ProtocolRoleResponder
	if local == muxer.ProtocolRoleResponder {
		peerRole = muxer.ProtocolRoleInitiator
	}
	send, recv, _ := peerMuxer.RegisterProtocol(ProtocolId, peerRole)
	require.NotNil(t, send)
	t.Cleanup(func() {
		localMuxer.Stop()
		peerMuxer.Stop()
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	peerMuxer.Start()
	return &rawPeer{
		send:       send,
		recv:       recv,
		isResponse: peerRole == muxer.ProtocolRoleResponder,
	}, localMuxer, make(chan error, 8)
}

func (p *rawPeer) write(t *testing.T, payload []byte) {
	t.Helper()
	segment := muxer.NewSegment(ProtocolId, payload, p.isResponse)
	require.NotNil(t, segment)
	select {
	case p.send <- segment:
	case <-time.After(time.Second):
		t.Fatal("raw peer could not send")
	}
}

func (p *rawPeer) writeMsg(t *testing.T, msg any) {
	t.Helper()
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	p.write(t, data)
}

// read returns the next whole message the gOuroboros endpoint sent, or nil
// if none arrives within wait.
func (p *rawPeer) read(t *testing.T, wait time.Duration) []byte {
	t.Helper()
	deadline := time.After(wait)
	for {
		if len(p.pending) > 0 {
			dec, err := cbor.NewStreamDecoder(p.pending)
			require.NoError(t, err)
			if _, length, err := dec.Skip(); err == nil {
				msg := append([]byte(nil), p.pending[:length]...)
				p.pending = p.pending[length:]
				return msg
			}
		}
		select {
		case segment, ok := <-p.recv:
			if !ok {
				return nil
			}
			p.pending = append(p.pending, segment.Payload...)
		case <-deadline:
			return nil
		}
	}
}

func (p *rawPeer) readType(t *testing.T) (uint8, []byte) {
	t.Helper()
	data := p.read(t, time.Second)
	require.NotNil(t, data, "no message from the gOuroboros endpoint")
	fields, err := decodeArray(data, -1, false)
	require.NoError(t, err)
	var msgType uint8
	_, err = cbor.Decode(fields[0], &msgType)
	require.NoError(t, err)
	return msgType, data
}

func requireNoError(t *testing.T, errs chan error) {
	t.Helper()
	select {
	case err := <-errs:
		t.Fatalf("connection failed: %v", err)
	default:
	}
}

// startClientAgainstRawServer starts a gOuroboros client facing a raw
// reference server, and consumes the client's MsgInit.
func startClientAgainstRawServer(
	t *testing.T,
	cfg *Config,
) (*Client, *rawPeer, chan error) {
	t.Helper()
	peer, localMuxer, errs := newRawPeerPair(t, muxer.ProtocolRoleInitiator)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:        localMuxer,
		ErrorChan:    errs,
		ConnectionId: testConnectionId(),
	}, cfg)
	client.Protocol.EnsureRegistered()
	localMuxer.Start()
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	msgType, _ := peer.readType(t)
	require.Equal(t, MessageTypeInit, msgType)
	return client, peer, errs
}

// TestClientAcceptsAwaitReplyThenIDs: a reference server with no vote
// answers a blocking request with MsgAwaitReply, then the identifiers once
// one arrives (Type.hs lines 279-282, 346-350).
func TestClientAcceptsAwaitReplyThenIDs(t *testing.T) {
	client, peer, errs := startClientAgainstRawServer(t, nil)
	id := VoteID{RoundNo: 3, SeatIndex: 1}
	result := make(chan error, 1)
	var got []VoteID
	go func() {
		var err error
		got, err = client.RequestObjectIDs(context.Background(), true, 0, 1)
		result <- err
	}()
	msgType, _ := peer.readType(t)
	require.Equal(t, MessageTypeRequestObjectIDs, msgType)
	peer.write(t, wireAwaitReply)
	peer.writeMsg(t, NewMsgReplyObjectIDs([]VoteID{id}))
	select {
	case err := <-result:
		require.NoError(t, err)
		require.Equal(t, []VoteID{id}, got)
	case err := <-errs:
		t.Fatalf("client refused the reference server's await: %v", err)
	case <-time.After(time.Second):
		t.Fatal("blocking request did not return")
	}
	requireNoError(t, errs)
}

// TestSyncContinuesAfterServerIdle: after MsgAwaitReply the reference server
// may give agency back with MsgServerIdle; the client then asks again
// (Type.hs lines 351-362). Sync must keep going and deliver the vote that
// the second blocking request gets, acknowledging nothing twice.
func TestSyncContinuesAfterServerIdle(t *testing.T) {
	id := VoteID{RoundNo: 5, SeatIndex: 2}
	votes := make(chan VoteID, 1)
	cfg := NewConfig(WithVoteFunc(
		func(_ CallbackContext, got VoteID, _ VoteObject) error {
			votes <- got
			return nil
		},
	))
	client, peer, errs := startClientAgainstRawServer(t, &cfg)
	ctx, cancel := context.WithCancel(context.Background())
	syncDone := make(chan error, 1)
	go func() { syncDone <- client.Sync(ctx) }()
	t.Cleanup(cancel)

	msgType, data := peer.readType(t)
	require.Equal(t, MessageTypeRequestObjectIDs, msgType)
	first, err := NewMsgFromCbor(uint(msgType), data)
	require.NoError(t, err)
	require.True(t, first.(*MsgRequestObjectIDs).Blocking)
	peer.write(t, wireAwaitReply)
	peer.write(t, wireServerIdle)

	msgType, data = peer.readType(t)
	require.Equal(t, MessageTypeRequestObjectIDs, msgType, "after idle")
	second, err := NewMsgFromCbor(uint(msgType), data)
	require.NoError(t, err)
	require.True(t, second.(*MsgRequestObjectIDs).Blocking)
	require.Zero(t, second.(*MsgRequestObjectIDs).AckCount)
	peer.writeMsg(t, NewMsgReplyObjectIDs([]VoteID{id}))

	msgType, _ = peer.readType(t)
	require.Equal(t, MessageTypeRequestObjects, msgType)
	peer.writeMsg(t, NewMsgReplyObjects([]VoteObject{testVoteObject(t, id)}))
	select {
	case got := <-votes:
		require.Equal(t, id, got)
	case err := <-syncDone:
		t.Fatalf("Sync ended: %v", err)
	case <-time.After(time.Second):
		t.Fatal("vote not delivered")
	}
	requireNoError(t, errs)
}

// TestClientBlockingRequestTimeouts: the reference client waits shortWait
// (10 s) for the first answer to a blocking request and longWait (60 s)
// after MsgAwaitReply (Codec.hs lines 87-93; Limits.hs lines 113-117).
func TestClientBlockingRequestTimeouts(t *testing.T) {
	cfg := NewConfig()
	stateMap := clientStateMap(&cfg)
	require.Equal(t, 10*time.Second, stateMap[StateObjectIDsBlocking].Timeout)
	require.Equal(
		t,
		60*time.Second,
		stateMap[StateObjectIDsBlockingMustReply].Timeout,
	)
}

// TestAwaitTimeoutStaysBelowMustReply: a server waiting as long as the
// client's MustReply timeout would be timed out by every reference client.
func TestAwaitTimeoutStaysBelowMustReply(t *testing.T) {
	require.Less(t, DefaultAwaitTimeout, MustReplyTimeout)
	require.Panics(t, func() { NewConfig(WithAwaitTimeout(MustReplyTimeout)) })
	require.Panics(t, func() { NewConfig(WithAwaitTimeout(-time.Second)) })
}

// startServerAgainstRawClient starts a gOuroboros server facing a raw
// reference client that has sent MsgInit and a blocking request for one
// identifier.
func startServerAgainstRawClient(
	t *testing.T,
	cfg *Config,
) (*Server, *rawPeer, chan error) {
	t.Helper()
	peer, localMuxer, errs := newRawPeerPair(t, muxer.ProtocolRoleResponder)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:        localMuxer,
		ErrorChan:    errs,
		ConnectionId: testConnectionId(),
	}, cfg)
	server.Protocol.EnsureRegistered()
	localMuxer.Start()
	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	peer.writeMsg(t, NewMsgInit())
	peer.writeMsg(t, NewMsgRequestObjectIDs(true, 0, 1))
	return server, peer, errs
}

// TestServerWithNoVoteSendsAwaitReply: a server with nothing to offer a
// blocking request must answer promptly with MsgAwaitReply rather than fail
// the connection or stay silent past the client's 10 s.
func TestServerWithNoVoteSendsAwaitReply(t *testing.T) {
	cfg := NewConfig(WithObjectIDsFunc(
		func(CallbackContext, uint16, uint16) ([]VoteID, error) {
			return nil, nil
		},
	))
	_, peer, errs := startServerAgainstRawClient(t, &cfg)
	got := peer.read(t, time.Second)
	requireNoError(t, errs)
	require.Equal(t, wireAwaitReply, got)
}

// TestServerAwaitsThenReplies: after MsgAwaitReply the server answers with
// the IDs its await function finds.
func TestServerAwaitsThenReplies(t *testing.T) {
	id := VoteID{RoundNo: 9, SeatIndex: 4}
	arrived := make(chan VoteID)
	cfg := NewConfig(
		WithObjectIDsFunc(
			func(CallbackContext, uint16, uint16) ([]VoteID, error) {
				return nil, nil
			},
		),
		WithAwaitObjectIDsFunc(
			func(ctx context.Context, _ CallbackContext, count uint16) ([]VoteID, error) {
				if count != 1 {
					return nil, fmt.Errorf("await asked for %d IDs", count)
				}
				select {
				case got := <-arrived:
					return []VoteID{got}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			},
		),
	)
	_, peer, errs := startServerAgainstRawClient(t, &cfg)
	require.Equal(t, wireAwaitReply, peer.read(t, time.Second))
	select {
	case arrived <- id:
	case <-time.After(time.Second):
		t.Fatal("await function did not receive the arriving vote")
	}
	msgType, data := peer.readType(t)
	require.Equal(t, MessageTypeReplyObjectIDs, msgType)
	reply, err := NewMsgFromCbor(uint(msgType), data)
	require.NoError(t, err)
	require.Equal(t, []VoteID{id}, reply.(*MsgReplyObjectIDs).ObjectIDs)
	requireNoError(t, errs)
}

// TestServerSendsServerIdleAfterAwaitTimeout: with nothing new within
// AwaitTimeout the server gives agency back with MsgServerIdle, and then
// accepts the client's next blocking request.
func TestServerSendsServerIdleAfterAwaitTimeout(t *testing.T) {
	calls := make(chan uint16, 4)
	cfg := NewConfig(
		WithAwaitTimeout(20*time.Millisecond),
		WithObjectIDsFunc(
			func(_ CallbackContext, ack, _ uint16) ([]VoteID, error) {
				calls <- ack
				return nil, nil
			},
		),
	)
	_, peer, errs := startServerAgainstRawClient(t, &cfg)
	require.Equal(t, wireAwaitReply, peer.read(t, time.Second))
	require.Equal(t, wireServerIdle, peer.read(t, time.Second))
	peer.writeMsg(t, NewMsgRequestObjectIDs(true, 0, 1))
	require.Equal(t, wireAwaitReply, peer.read(t, time.Second))
	require.Len(t, calls, 2)
	requireNoError(t, errs)
}

// TestSyncAgainstServerWithNoVoteYet: gOuroboros on both sides, the server
// has no vote at first. Sync must survive the await and the idle and
// deliver the vote that arrives later.
func TestSyncAgainstServerWithNoVoteYet(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	leftMuxer, rightMuxer := muxer.New(leftConn), muxer.New(rightConn)
	errs := make(chan error, 8)
	id := VoteID{RoundNo: 11, SeatIndex: 3}
	idles := make(chan struct{}, 1)
	release := make(chan struct{})
	serverCfg := NewConfig(
		WithAwaitTimeout(20*time.Millisecond),
		WithObjectIDsFunc(
			func(_ CallbackContext, _, _ uint16) ([]VoteID, error) {
				select {
				case <-release:
					return []VoteID{id}, nil
				default:
					return nil, nil
				}
			},
		),
		WithAwaitObjectIDsFunc(
			func(ctx context.Context, _ CallbackContext, _ uint16) ([]VoteID, error) {
				<-ctx.Done()
				select {
				case idles <- struct{}{}:
				default:
				}
				return nil, nil
			},
		),
		WithObjectsFunc(
			func(_ CallbackContext, ids []VoteID) ([]VoteObject, error) {
				return []VoteObject{testVoteObject(t, ids[0])}, nil
			},
		),
	)
	votes := make(chan VoteID, 1)
	clientCfg := NewConfig(WithVoteFunc(
		func(_ CallbackContext, got VoteID, _ VoteObject) error {
			select {
			case votes <- got:
			default:
			}
			return nil
		},
	))
	server := NewServer(protocol.ProtocolOptions{
		Muxer: leftMuxer, ErrorChan: errs, ConnectionId: testConnectionId(),
	}, &serverCfg)
	client := NewClient(protocol.ProtocolOptions{
		Muxer: rightMuxer, ErrorChan: errs, ConnectionId: testConnectionId(),
	}, &clientCfg)
	server.Protocol.EnsureRegistered()
	client.Protocol.EnsureRegistered()
	leftMuxer.Start()
	rightMuxer.Start()
	server.Start()
	client.Start()
	t.Cleanup(func() {
		client.Stop(context.Background())
		server.Stop(context.Background())
		leftMuxer.Stop()
		rightMuxer.Stop()
		_ = leftConn.Close()
		_ = rightConn.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	syncDone := make(chan error, 1)
	go func() { syncDone <- client.Sync(ctx) }()
	select {
	case <-idles:
	case err := <-errs:
		t.Fatalf("connection failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("server never idled")
	}
	close(release)
	select {
	case got := <-votes:
		require.Equal(t, id, got)
	case err := <-syncDone:
		t.Fatalf("Sync ended: %v", err)
	case err := <-errs:
		t.Fatalf("connection failed: %v", err)
	case <-time.After(time.Second):
		t.Fatal("vote not delivered")
	}
}
