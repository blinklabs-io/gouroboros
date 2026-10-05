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
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestClientServerObjectDiffusionFlow(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	leftMuxer, rightMuxer := muxer.New(leftConn), muxer.New(rightConn)
	errors := make(chan error, 8)
	id := VoteID{RoundNo: 4, SeatIndex: 7}
	object := testVoteObject(t, id)
	serverCfg := NewConfig(
		WithObjectIDsFunc(
			func(_ CallbackContext, ack, count uint16) ([]VoteID, error) {
				if ack != 0 || count != 1 {
					return nil, fmt.Errorf(
						"unexpected object ID request: ack=%d count=%d",
						ack,
						count,
					)
				}
				return []VoteID{id}, nil
			},
		),
		WithObjectsFunc(func(_ CallbackContext, ids []VoteID) ([]VoteObject, error) {
			if len(ids) != 1 || ids[0] != id {
				return nil, fmt.Errorf("unexpected object request: %v", ids)
			}
			return []VoteObject{object}, nil
		}),
	)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:        leftMuxer,
		ErrorChan:    errors,
		ConnectionId: testConnectionId(),
	}, &serverCfg)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:        rightMuxer,
		ErrorChan:    errors,
		ConnectionId: testConnectionId(),
	}, nil)
	server.Protocol.EnsureRegistered()
	client.Protocol.EnsureRegistered()
	leftMuxer.Start()
	rightMuxer.Start()
	server.Start()
	client.Start()
	t.Cleanup(func() {
		client.Stop()
		server.Stop()
		leftMuxer.Stop()
		rightMuxer.Stop()
		_ = leftConn.Close()
		_ = rightConn.Close()
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ids, err := client.RequestObjectIDs(ctx, true, 0, 1)
	require.NoError(t, err)
	require.Equal(t, []VoteID{id}, ids)
	objects, err := client.RequestObjects(ctx, ids)
	require.NoError(t, err)
	require.Len(t, objects, 1)
	require.True(t, bytes.Equal(object, objects[0]))
	require.NoError(t, client.Done(ctx))
	require.Eventually(t, server.Protocol.IsDone, time.Second, time.Millisecond)
}

func TestObjectDiffusionWindowAndAgencyRules(t *testing.T) {
	ctx := newStateContext(2)
	ids := []VoteID{{RoundNo: 1, SeatIndex: 1}, {RoundNo: 1, SeatIndex: 2}}
	require.True(t, matchBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(true, 0, 2),
	))
	require.True(t, matchReplyObjectIDs(ctx, NewMsgReplyObjectIDs(ids)))
	require.False(t, matchBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(true, 0, 1),
	))
	require.True(t, matchNonBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(false, 1, 1),
	))
	newID := VoteID{RoundNo: 2, SeatIndex: 1}
	require.True(t, matchReplyObjectIDs(
		ctx,
		NewMsgReplyObjectIDs([]VoteID{newID}),
	))
	require.Equal(t, []VoteID{ids[1], newID}, ctx.outstanding)
	request := NewMsgRequestObjects([]VoteID{ids[1]})
	require.True(t, matchRequestObjects(ctx, request))
	require.False(t, matchRequestObjects(ctx, request))
	require.True(t, matchReplyObjects(
		ctx,
		NewMsgReplyObjects([]VoteObject{testVoteObject(t, ids[1])}),
	))
	require.False(t, matchRequestObjects(ctx, request))
	require.True(t, matchBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(true, 2, 1),
	))
	require.False(t, matchReplyObjectIDs(ctx, NewMsgReplyObjectIDs(nil)))
}

func TestNonBlockingAcknowledgementWithoutNewIDs(t *testing.T) {
	ctx := newStateContext(2)
	ids := []VoteID{{RoundNo: 1, SeatIndex: 3}, {RoundNo: 1, SeatIndex: 4}}
	require.True(t, matchBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(true, 0, 2),
	))
	require.True(t, matchReplyObjectIDs(ctx, NewMsgReplyObjectIDs(ids)))
	require.False(t, matchNonBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(false, 0, 0),
	))
	require.True(t, matchNonBlockingObjectIDsRequest(
		ctx,
		NewMsgRequestObjectIDs(false, 1, 0),
	))
	require.True(t, matchReplyObjectIDs(ctx, NewMsgReplyObjectIDs(nil)))
	require.Equal(t, []VoteID{ids[1]}, ctx.outstanding)
}

func TestClientRequestReturnsOnProtocolShutdown(t *testing.T) {
	leftConn, rightConn := net.Pipe()
	leftMuxer, rightMuxer := muxer.New(leftConn), muxer.New(rightConn)
	entered, release := make(chan struct{}), make(chan struct{})
	serverCfg := NewConfig(
		WithObjectIDsFunc(
			func(_ CallbackContext, _, _ uint16) ([]VoteID, error) {
				close(entered)
				<-release
				return []VoteID{{RoundNo: 1, SeatIndex: 1}}, nil
			},
		),
	)
	errors := make(chan error, 8)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:     leftMuxer,
		ErrorChan: errors,
	}, &serverCfg)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:     rightMuxer,
		ErrorChan: errors,
	}, nil)
	server.Protocol.EnsureRegistered()
	client.Protocol.EnsureRegistered()
	leftMuxer.Start()
	rightMuxer.Start()
	server.Start()
	client.Start()
	t.Cleanup(func() {
		close(release)
		client.Stop()
		server.Stop()
		leftMuxer.Stop()
		rightMuxer.Stop()
		_ = leftConn.Close()
		_ = rightConn.Close()
	})
	requestDone := make(chan error, 1)
	go func() {
		_, err := client.RequestObjectIDs(context.Background(), true, 0, 1)
		requestDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("server did not receive the object ID request")
	}
	client.Stop()
	select {
	case err := <-requestDone:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("client request did not return after protocol shutdown")
	}
}

func testConnectionId() connection.ConnectionId {
	return connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)},
	}
}
