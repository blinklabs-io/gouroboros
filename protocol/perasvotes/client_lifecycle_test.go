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
	"errors"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// failAfter bounds how long a test waits for a result that the code under
// test should produce at once; it only decides how long a failure takes.
const failAfter = 5 * time.Second

func startPair(
	t *testing.T,
	serverCfg *Config,
	clientCfg *Config,
) (*Client, *Server, chan error) {
	t.Helper()
	leftConn, rightConn := net.Pipe()
	leftMuxer, rightMuxer := muxer.New(leftConn), muxer.New(rightConn)
	errs := make(chan error, 8)
	server := NewServer(protocol.ProtocolOptions{
		Muxer:        leftMuxer,
		ErrorChan:    errs,
		ConnectionId: testConnectionId(),
	}, serverCfg)
	client := NewClient(protocol.ProtocolOptions{
		Muxer:        rightMuxer,
		ErrorChan:    errs,
		ConnectionId: testConnectionId(),
	}, clientCfg)
	server.EnsureRegistered()
	client.Protocol.EnsureRegistered()
	leftMuxer.Start()
	rightMuxer.Start()
	server.Start()
	client.Start()
	t.Cleanup(func() {
		_ = client.Stop(context.Background())
		_ = server.Stop(context.Background())
		_ = leftMuxer.Stop(context.Background())
		_ = rightMuxer.Stop(context.Background())
		_ = leftConn.Close()
		_ = rightConn.Close()
	})
	return client, server, errs
}

// TestDoneHonoursContextWhileSyncRuns: Sync owns the request exchange for as
// long as it runs, so Done waits for it, but only as long as Done's own
// context allows.
func TestDoneHonoursContextWhileSyncRuns(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	serverCfg := NewConfig(
		WithObjectIDsFunc(
			func(CallbackContext, uint16, uint16) ([]VoteID, error) {
				close(entered)
				<-release
				return nil, nil
			},
		),
	)
	clientCfg := NewConfig(
		WithVoteFunc(func(CallbackContext, VoteID, VoteObject) error {
			return nil
		}),
	)
	client, _, _ := startPair(t, &serverCfg, &clientCfg)
	syncCtx, cancelSync := context.WithCancel(context.Background())
	defer cancelSync()
	go func() { _ = client.Sync(syncCtx) }()
	<-entered

	doneCtx, cancelDone := context.WithCancel(context.Background())
	cancelDone()
	result := make(chan error, 1)
	go func() { result <- client.Done(doneCtx) }()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(failAfter):
		t.Fatal("Done ignored its cancelled context while Sync ran")
	}
}

// TestAwaitObjectIDsPanicIsContained: a panic in AwaitObjectIDsFunc is
// reported as a protocol error, as a panic in any other callback is, rather
// than ending the process.
func TestAwaitObjectIDsPanicIsContained(t *testing.T) {
	serverCfg := NewConfig(
		WithObjectIDsFunc(
			func(CallbackContext, uint16, uint16) ([]VoteID, error) {
				return nil, nil
			},
		),
		WithAwaitObjectIDsFunc(
			func(context.Context, CallbackContext, uint16) ([]VoteID, error) {
				panic("await callback panic")
			},
		),
	)
	client, _, errs := startPair(t, &serverCfg, nil)
	ctx, cancel := context.WithTimeout(context.Background(), failAfter)
	defer cancel()
	go func() { _, _ = client.RequestObjectIDs(ctx, true, 0, 1) }()
	select {
	case err := <-errs:
		require.ErrorIs(t, err, protocol.ErrHandlerPanic)
	case <-ctx.Done():
		t.Fatal("no error reported for the panic")
	}
}

// TestVoteFuncCallingItsClientReturnsAtOnce: VoteFunc runs inside Sync, which
// holds the client's request exchange, so a call back into the same client
// can never be served. With the context the client gave VoteFunc, the call
// returns ErrCalledFromVoteFunc instead of waiting on the Sync it runs in.
func TestVoteFuncCallingItsClientReturnsAtOnce(t *testing.T) {
	id := VoteID{RoundNo: 4, SeatIndex: 7}
	object := testVoteObject(t, id)
	serverCfg := NewConfig(
		WithObjectIDsFunc(
			func(CallbackContext, uint16, uint16) ([]VoteID, error) {
				return []VoteID{id}, nil
			},
		),
		WithObjectsFunc(
			func(CallbackContext, []VoteID) ([]VoteObject, error) {
				return []VoteObject{object}, nil
			},
		),
	)
	errStop := errors.New("stop sync")
	type calls struct{ ids, objects, done error }
	got := make(chan calls, 1)
	clientCfg := NewConfig(
		WithVoteFunc(func(cbCtx CallbackContext, _ VoteID, _ VoteObject) error {
			ctx := cbCtx.Context()
			var c calls
			_, c.ids = cbCtx.Client.RequestObjectIDs(ctx, false, 0, 1)
			_, c.objects = cbCtx.Client.RequestObjects(ctx, []VoteID{id})
			c.done = cbCtx.Client.Done(ctx)
			got <- c
			return errStop
		}),
	)
	client, _, _ := startPair(t, &serverCfg, &clientCfg)
	syncErr := make(chan error, 1)
	go func() { syncErr <- client.Sync(context.Background()) }()
	select {
	case c := <-got:
		require.ErrorIs(t, c.ids, ErrCalledFromVoteFunc)
		require.ErrorIs(t, c.objects, ErrCalledFromVoteFunc)
		require.ErrorIs(t, c.done, ErrCalledFromVoteFunc)
	case <-time.After(failAfter):
		t.Fatal("a call from VoteFunc waited on the Sync running it")
	}
	select {
	case err := <-syncErr:
		require.ErrorIs(t, err, errStop)
	case <-time.After(failAfter):
		t.Fatal("Sync did not end after VoteFunc returned an error")
	}
}

// TestCallbackContextOfAnotherClientWaits: only this client's own callback
// context skips the wait. Another client's callback context is an ordinary
// context here, so Done waits for Sync until that context ends.
func TestCallbackContextOfAnotherClientWaits(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	serverCfg := NewConfig(
		WithObjectIDsFunc(
			func(CallbackContext, uint16, uint16) ([]VoteID, error) {
				close(entered)
				<-release
				return nil, nil
			},
		),
	)
	clientCfg := NewConfig(
		WithVoteFunc(func(CallbackContext, VoteID, VoteObject) error {
			return nil
		}),
	)
	client, _, _ := startPair(t, &serverCfg, &clientCfg)
	other := NewClient(protocol.ProtocolOptions{}, &clientCfg)
	syncCtx, cancelSync := context.WithCancel(context.Background())
	defer cancelSync()
	go func() { _ = client.Sync(syncCtx) }()
	<-entered

	doneCtx, cancelDone := context.WithCancel(
		other.callbackContext.Context(),
	)
	result := make(chan error, 1)
	go func() { result <- client.Done(doneCtx) }()
	cancelDone()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, ErrCalledFromVoteFunc)
	case <-time.After(failAfter):
		t.Fatal("Done did not end with its context")
	}
}
