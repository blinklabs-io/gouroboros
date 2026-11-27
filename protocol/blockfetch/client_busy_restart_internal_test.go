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

package blockfetch

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// A fetch that has taken the busy lock but not yet sent keeps it across a
// Stop and Start: it then sends on the new instance, so no second fetch may
// start before it finishes.
func TestBusyLockHeldAcrossRestart(t *testing.T) {
	acquired := make(chan int, 2)
	hold := make(chan struct{})
	var calls atomic.Int32
	p := newIdlePeer(t, &Config{
		SkipBlockValidation: true,
		BlockRawFunc:        func(CallbackContext, uint, []byte) error { return nil },
	})
	c := p.client
	c.afterAcquireBusy = func() {
		n := int(calls.Add(1))
		acquired <- n
		if n == 1 {
			<-hold
		}
	}
	_, point := queueTestBlock(t, 100, lcommon.Blake2b256{})
	first := make(chan error, 1)
	go func() { first <- c.GetBlockRange(point, point) }()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-acquired:
	case <-timer.C:
		t.Fatal("the first fetch did not take the busy lock")
	}
	require.NoError(t, c.Stop(context.Background()))
	c.Start()
	second := make(chan error, 1)
	go func() { second <- c.GetBlockRange(point, point) }()
	window := time.NewTimer(200 * time.Millisecond)
	defer window.Stop()
	select {
	case <-acquired:
		close(hold)
		t.Fatal("a second fetch took the busy lock while the first was in progress")
	case <-window.C:
	}
	requestBytes := requestRangeWireBytes(t, point)
	start := p.outboundRead.value()
	close(hold)
	p.waitForOutboundBytes(t, start+requestBytes)
	p.send(t, NewMsgNoBlocks())
	select {
	case err := <-first:
		require.ErrorIs(t, err, ErrNoBlocks)
	case <-timer.C:
		t.Fatal("the first fetch did not finish")
	}
	select {
	case <-acquired:
	case <-timer.C:
		t.Fatal("the second fetch did not take the busy lock after the first")
	}
	p.waitForOutboundBytes(t, start+2*requestBytes)
	p.send(t, NewMsgNoBlocks())
	select {
	case err := <-second:
		require.ErrorIs(t, err, ErrNoBlocks)
	case <-timer.C:
		t.Fatal("the second fetch did not finish")
	}
}

// requestRangeWireBytes is the size on the wire of one MsgRequestRange for
// point, in its own segment.
func requestRangeWireBytes(t *testing.T, point pcommon.Point) int {
	t.Helper()
	data, err := cbor.Encode(NewMsgRequestRange(point, point))
	require.NoError(t, err)
	return binary.Size(muxer.SegmentHeader{}) + len(data)
}

// A fetch on a client that was never started, held after taking the busy
// lock while Stop runs, sends on an instance Stop has already stopped: it
// fails, and releases the lock, rather than waiting forever on an instance
// that will never start or finish. A fetch after Start then takes the lock.
func TestFetchOnNeverStartedClientFailsAfterStop(t *testing.T) {
	localConn, peerConn := net.Pipe()
	m := muxer.New(localConn)
	m.Start()
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
		close(peerDone)
	}()
	c := NewClient(protocol.ProtocolOptions{
		Muxer:     m,
		ErrorChan: make(chan error, 10),
		Mode:      protocol.ProtocolModeNodeToNode,
	}, &Config{
		SkipBlockValidation: true,
		BlockRawFunc:        func(CallbackContext, uint, []byte) error { return nil },
	})
	t.Cleanup(func() {
		_ = c.Stop(context.Background())
		_ = m.Stop(context.Background())
		_ = peerConn.Close()
		<-peerDone
	})
	acquired := make(chan struct{}, 2)
	hold := make(chan struct{})
	var calls atomic.Int32
	c.afterAcquireBusy = func() {
		acquired <- struct{}{}
		if calls.Add(1) == 1 {
			<-hold
		}
	}
	_, point := queueTestBlock(t, 100, lcommon.Blake2b256{})
	first := make(chan error, 1)
	go func() { first <- c.GetBlockRange(point, point) }()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	select {
	case <-acquired:
	case <-timer.C:
		t.Fatal("the first fetch did not take the busy lock")
	}
	require.NoError(t, c.Stop(context.Background()))
	close(hold)
	select {
	case err := <-first:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-timer.C:
		t.Fatal("the fetch on the stopped instance did not return")
	}
	c.Start()
	go func() { _ = c.GetBlockRange(point, point) }()
	select {
	case <-acquired:
	case <-timer.C:
		t.Fatal("a fetch after Start did not take the busy lock")
	}
}
