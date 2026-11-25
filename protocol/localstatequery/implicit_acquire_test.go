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

package localstatequery

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// snapshotLedger is a server-side ledger whose tip the test moves. An
// acquire copies the tip into the snapshot, and every query answers from the
// snapshot, as a node answers from the state it acquired.
type snapshotLedger struct {
	mu       sync.Mutex
	tip      uint64
	snap     uint64
	acquires int
	releases chan struct{}
	// afterEra runs once the era query has been answered from the snapshot,
	// to move the chain between the two queries of one call.
	afterEra func(*snapshotLedger)
}

func (l *snapshotLedger) setTip(tip uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.tip = tip
}

func (l *snapshotLedger) acquireCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.acquires
}

func (l *snapshotLedger) acquire(CallbackContext, AcquireTarget, bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.acquires++
	l.snap = l.tip
	return nil
}

func (l *snapshotLedger) release(CallbackContext) error {
	l.releases <- struct{}{}
	return nil
}

func (l *snapshotLedger) query(_ CallbackContext, q QueryWrapper) (any, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	block, ok := q.Query.(*BlockQuery)
	if !ok {
		// GetChainBlockNo: At blockNo.
		return []uint64{1, l.snap}, nil
	}
	switch block.Query.(type) {
	case *HardForkQuery:
		if l.afterEra != nil {
			l.afterEra(l)
		}
		return uint64(ledger.EraIdConway), nil
	default:
		// GetEpochNo: the epoch is the snapshot's tip.
		return []uint64{l.snap}, nil
	}
}

// waitReleases waits until the server has seen n releases.
func (l *snapshotLedger) waitReleases(t *testing.T, n int) {
	t.Helper()
	for i := range n {
		select {
		case <-l.releases:
		case <-time.After(10 * time.Second):
			t.Fatalf("server saw %d releases, want %d", i, n)
		}
	}
}

// requireNoRelease checks that no release reached the server before a later
// request did: the server reads in order, so any release sent earlier has
// already been counted.
func (l *snapshotLedger) requireNoRelease(t *testing.T) {
	t.Helper()
	select {
	case <-l.releases:
		t.Fatal("server saw a release")
	default:
	}
}

func newSnapshotPair(t *testing.T, l *snapshotLedger) *Client {
	t.Helper()
	local, remote := net.Pipe()
	serverMuxer := muxer.New(local)
	clientMuxer := muxer.New(remote)
	errs := make(chan error, 8)
	connID := connection.ConnectionId{
		LocalAddr:  &net.UnixAddr{Name: "local", Net: "unix"},
		RemoteAddr: &net.UnixAddr{Name: "remote", Net: "unix"},
	}
	server := NewServer(protocol.ProtocolOptions{
		Muxer: serverMuxer, ConnectionId: connID, ErrorChan: errs,
	}, &Config{
		AcquireFunc: l.acquire,
		QueryFunc:   l.query,
		ReleaseFunc: l.release,
	})
	client := NewClient(protocol.ProtocolOptions{
		Muxer: clientMuxer, ConnectionId: connID, ErrorChan: errs,
	}, &Config{AcquireTimeout: time.Minute, QueryTimeout: time.Minute})
	t.Cleanup(func() {
		select {
		case err := <-errs:
			t.Errorf("protocol error: %v", err)
		default:
		}
		_ = local.Close()
		_ = remote.Close()
		server.Stop()
		client.Stop()
		serverMuxer.Stop()
		clientMuxer.Stop()
	})
	server.Start()
	client.Start()
	serverMuxer.Start()
	clientMuxer.Start()
	return client
}

func newSnapshotLedger(tip uint64) *snapshotLedger {
	return &snapshotLedger{tip: tip, releases: make(chan struct{}, 16)}
}

// TestImplicitQueryAnswersFromCurrentTip checks that a query made without an
// explicit acquire answers from the chain as it is when the call is made, not
// from the state an earlier call acquired for itself.
func TestImplicitQueryAnswersFromCurrentTip(t *testing.T) {
	t.Parallel()
	l := newSnapshotLedger(100)
	client := newSnapshotPair(t, l)

	blockNo, err := client.GetChainBlockNo()
	require.NoError(t, err)
	require.Equal(t, int64(100), blockNo)

	l.setTip(200)
	blockNo, err = client.GetChainBlockNo()
	require.NoError(t, err)
	require.Equal(t, int64(200), blockNo, "answered from a stale snapshot")
	require.Equal(t, 2, l.acquireCount())
	// Each call released the state it acquired, leaving the client idle.
	l.waitReleases(t, 2)
	client.busyMutex.Lock()
	defer client.busyMutex.Unlock()
	require.False(t, client.acquired)
}

// TestImplicitAcquireSpansTheCall checks that the queries of one call all
// answer from one state. GetEpochNo asks for the era and then for the epoch;
// releasing between them would let the chain move, and the epoch would no
// longer belong to the era the call decoded it with.
func TestImplicitAcquireSpansTheCall(t *testing.T) {
	t.Parallel()
	l := newSnapshotLedger(7)
	l.afterEra = func(l *snapshotLedger) { l.tip++ }
	client := newSnapshotPair(t, l)

	epoch, err := client.GetEpochNo()
	require.NoError(t, err)
	require.Equal(t, 7, epoch, "the epoch query used a second state")
	require.Equal(t, 1, l.acquireCount())
	l.waitReleases(t, 1)

	// The next call acquires again and sees the chain the first one moved.
	epoch, err = client.GetEpochNo()
	require.NoError(t, err)
	require.Equal(t, 8, epoch)
	require.Equal(t, 2, l.acquireCount())
	l.waitReleases(t, 1)
}

// TestExplicitAcquireKeepsSnapshot checks that a state the caller acquired
// stays in place across queries, however the chain moves, until the caller
// releases it.
func TestExplicitAcquireKeepsSnapshot(t *testing.T) {
	t.Parallel()
	l := newSnapshotLedger(100)
	client := newSnapshotPair(t, l)

	require.NoError(t, client.AcquireVolatileTip())
	for range 3 {
		blockNo, err := client.GetChainBlockNo()
		require.NoError(t, err)
		require.Equal(t, int64(100), blockNo)
		l.setTip(200)
	}
	epoch, err := client.GetEpochNo()
	require.NoError(t, err)
	require.Equal(t, 100, epoch)
	require.Equal(t, 1, l.acquireCount())
	l.requireNoRelease(t)

	require.NoError(t, client.Release())
	l.waitReleases(t, 1)
	blockNo, err := client.GetChainBlockNo()
	require.NoError(t, err)
	require.Equal(t, int64(200), blockNo)
	require.Equal(t, 2, l.acquireCount())
	l.waitReleases(t, 1)
}
