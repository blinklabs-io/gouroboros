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
