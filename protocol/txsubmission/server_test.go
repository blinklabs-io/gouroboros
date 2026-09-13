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

package txsubmission

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestRequestTxIdsRejectsReplyExceedingRequest(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, localConn.Close())
		require.NoError(t, remoteConn.Close())
	})
	serverMuxer := muxer.New(localConn)
	clientMuxer := muxer.New(remoteConn)
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(serverMuxer.Stop)
	t.Cleanup(clientMuxer.Stop)
	connectionId := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}

	returned := []TxIdAndSize{
		{TxId: TxId{EraId: 1}},
		{TxId: TxId{EraId: 1}},
	}
	returned[0].TxId.TxId[0] = 2
	returned[1].TxId.TxId[0] = 3
	requests := 0
	acknowledged := make(chan uint16, 2)
	client := NewClient(
		protocol.ProtocolOptions{Muxer: clientMuxer, ConnectionId: connectionId},
		&Config{RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			ack uint16,
			_ uint16,
		) ([]TxIdAndSize, error) {
			acknowledged <- ack
			requests++
			if requests == 1 {
				first := TxIdAndSize{TxId: TxId{EraId: 1}}
				first.TxId.TxId[0] = 1
				return []TxIdAndSize{first}, nil
			}
			return returned, nil
		}},
	)
	initReceived := make(chan struct{})
	server := NewServer(
		protocol.ProtocolOptions{Muxer: serverMuxer, ConnectionId: connectionId},
		&Config{InitFunc: func(CallbackContext) error {
			close(initReceived)
			return nil
		}},
	)
	server.Start()
	client.Start()
	client.Init()
	select {
	case <-initReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive Init")
	}

	result, err := server.RequestTxIds(false, 1)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Equal(t, 1, server.ackCount)
	require.Equal(t, uint16(0), <-acknowledged)

	result, err = server.RequestTxIds(false, 1)
	require.ErrorIs(t, err, protocol.ErrProtocolViolationRequestExceeded)
	require.Nil(t, result)
	require.Equal(t, 1, server.ackCount)
	require.Equal(t, uint16(1), <-acknowledged)
}

func TestRequestTxIdsAcceptsReplyWithinRequest(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() {
		require.NoError(t, localConn.Close())
		require.NoError(t, remoteConn.Close())
	})
	serverMuxer := muxer.New(localConn)
	clientMuxer := muxer.New(remoteConn)
	serverMuxer.Start()
	clientMuxer.Start()
	t.Cleanup(serverMuxer.Stop)
	t.Cleanup(clientMuxer.Stop)
	connectionId := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	client := NewClient(
		protocol.ProtocolOptions{Muxer: clientMuxer, ConnectionId: connectionId},
		&Config{RequestTxIdsFunc: func(
			_ CallbackContext,
			_ bool,
			_ uint16,
			req uint16,
		) ([]TxIdAndSize, error) {
			if req == 0 {
				return nil, nil
			}
			return []TxIdAndSize{{TxId: TxId{EraId: 1}}}, nil
		}},
	)
	initReceived := make(chan struct{})
	server := NewServer(
		protocol.ProtocolOptions{Muxer: serverMuxer, ConnectionId: connectionId},
		&Config{InitFunc: func(CallbackContext) error {
			close(initReceived)
			return nil
		}},
	)
	server.Start()
	client.Start()
	client.Init()
	select {
	case <-initReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive Init")
	}

	result, err := server.RequestTxIds(false, 1)
	require.NoError(t, err)
	require.Len(t, result, 1)
	result, err = server.RequestTxIds(false, 2)
	require.NoError(t, err)
	require.Len(t, result, 1)
	result, err = server.RequestTxIds(false, 0)
	require.NoError(t, err)
	require.Empty(t, result)
}

func TestReplyHandlersSynchronizeProtocolAccessDuringRestart(t *testing.T) {
	const iterations = 10_000

	server := NewServer(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
			RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
		},
	}, nil)
	server.requestTxIdsResultChan = make(
		chan requestTxIdsResult,
		iterations,
	)
	server.requestTxsResultChan = make(chan []TxBody, iterations)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			server.initProtocol()
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			server.handleReplyTxIds(NewMsgReplyTxIds(nil))
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		for range iterations {
			server.handleReplyTxs(NewMsgReplyTxs(nil))
		}
	}()

	close(start)
	wg.Wait()

	require.Len(t, server.requestTxIdsResultChan, iterations)
	require.Len(t, server.requestTxsResultChan, iterations)
}
