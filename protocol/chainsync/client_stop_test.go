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

package chainsync

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/pipeline"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestConcurrentStopReleasesBusyMutexDuringActiveSync(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	m := muxer.New(clientConn)
	m.Start()
	defer m.Stop()
	defer clientConn.Close()

	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
		close(peerDone)
	}()
	defer func() {
		_ = peerConn.Close()
		<-peerDone
	}()

	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: connection.ConnectionId{
				LocalAddr:  &net.TCPAddr{},
				RemoteAddr: &net.TCPAddr{},
			},
			Muxer: m,
			Mode:  protocol.ProtocolModeNodeToClient,
		},
		nil,
	)
	client.Start()

	firstStopInitiated := make(chan struct{})
	secondStopWaiting := make(chan struct{})
	syncLoopBeforeBusyLock := make(chan struct{})
	releaseSyncLoop := make(chan struct{})
	var releaseSyncLoopOnce sync.Once
	release := func() {
		releaseSyncLoopOnce.Do(func() { close(releaseSyncLoop) })
	}
	defer release()

	client.testStopInitiated = func() { close(firstStopInitiated) }
	client.testStopWaitingForComplete = func() { close(secondStopWaiting) }
	client.testSyncLoopBeforeBusyLock = func() {
		close(syncLoopBeforeBusyLock)
		<-releaseSyncLoop
	}

	// Start an active sync loop and stop it immediately before it would acquire
	// busyMutex. This leaves the lock available for the second Stop call.
	readyForNextBlockChan := client.readyForNextBlockChan
	doneChan := client.DoneChan()
	client.syncLoopWaitGroup.Add(1)
	go func() {
		defer client.syncLoopWaitGroup.Done()
		client.syncLoop(readyForNextBlockChan, doneChan)
	}()
	readyForNextBlockChan <- true
	<-syncLoopBeforeBusyLock

	// Give the first Stop call exclusive access to busyMutex, then wait until it
	// has transitioned to stopping and released that mutex.
	client.busyMutex.Lock()
	firstStopDone := make(chan error, 1)
	go func() { firstStopDone <- client.Stop() }()
	client.busyMutex.Unlock()
	<-firstStopInitiated
	client.busyMutex.Lock()

	// The active sync loop is still blocked at its test hook, so the second Stop
	// is the only waiter that can acquire busyMutex.
	secondStopDone := make(chan error, 1)
	go func() { secondStopDone <- client.Stop() }()
	client.busyMutex.Unlock()
	<-secondStopWaiting

	// The first Stop waits for the sync loop. It can complete only if the second
	// Stop released busyMutex before waiting for stoppingDone.
	release()
	select {
	case err := <-firstStopDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("first Stop did not complete after the active sync loop was released")
	}
	select {
	case err := <-secondStopDone:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("second Stop did not complete after the active sync loop was released")
	}
}

func TestStopWaitsForInitialRequestLifecycleFence(t *testing.T) {
	client, cleanup := newStartedTestClient(t, nil)
	defer cleanup()

	beforeEnqueue := make(chan struct{})
	allowEnqueue := make(chan struct{})
	enqueued := make(chan struct{})
	stopInitiated := make(chan struct{})
	client.testInitialRequestBeforeEnqueue = func() {
		close(beforeEnqueue)
		<-allowEnqueue
	}
	client.testInitialRequestAfterEnqueue = func() { close(enqueued) }
	client.testStopInitiated = func() {
		select {
		case <-enqueued:
		case <-time.After(time.Second):
			t.Error("Stop began before Sync queued its initial RequestNext")
		}
		close(stopInitiated)
	}

	syncDone := make(chan error, 1)
	go func() { syncDone <- client.sendInitialRequestAndStartSyncLoop() }()
	select {
	case <-beforeEnqueue:
	case <-time.After(time.Second):
		t.Fatal("Sync did not reach the initial request fence")
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- client.Stop() }()
	select {
	case <-stopInitiated:
		t.Fatal("Stop began while Sync held the initial request lifecycle fence")
	default:
	}

	close(allowEnqueue)
	select {
	case err := <-syncDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Sync did not finish queuing its initial RequestNext")
	}
	select {
	case <-stopInitiated:
	case <-time.After(time.Second):
		t.Fatal("Stop did not begin after Sync released its lifecycle fence")
	}
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish")
	}
}

func TestStopWaitsForPipelineRequestBurstLifecycleFence(t *testing.T) {
	cfg := NewConfig(WithPipelineLimit(3))
	client, cleanup := newStartedTestClient(t, &cfg)
	defer cleanup()

	beforeEnqueue := make(chan struct{})
	allowEnqueue := make(chan struct{})
	enqueued := make(chan struct{})
	stopInitiated := make(chan struct{})
	client.testSyncLoopBeforeRequestNext = func() {
		close(beforeEnqueue)
		<-allowEnqueue
	}
	client.testSyncLoopAfterRequestNext = func() { close(enqueued) }
	client.testStopInitiated = func() {
		select {
		case <-enqueued:
		case <-time.After(time.Second):
			t.Error("Stop began before the sync loop queued its RequestNext burst")
		}
		close(stopInitiated)
	}

	readyForNextBlockChan := make(chan bool, 1)
	client.syncLoopWaitGroup.Add(1)
	go func() {
		defer client.syncLoopWaitGroup.Done()
		client.syncLoop(readyForNextBlockChan, client.DoneChan())
	}()
	readyForNextBlockChan <- true
	select {
	case <-beforeEnqueue:
	case <-time.After(time.Second):
		t.Fatal("sync loop did not reach the pipeline request fence")
	}
	stopDone := make(chan error, 1)
	go func() { stopDone <- client.Stop() }()
	select {
	case <-stopInitiated:
		t.Fatal("Stop began while the sync loop held its request lifecycle fence")
	default:
	}

	close(allowEnqueue)
	select {
	case <-enqueued:
	case <-time.After(time.Second):
		t.Fatal("sync loop did not queue its RequestNext burst")
	}
	select {
	case <-stopInitiated:
	case <-time.After(time.Second):
		t.Fatal("Stop did not begin after the sync loop released its lifecycle fence")
	}
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish")
	}
}

func TestStopDuringStartAbortsUnstartedProtocol(t *testing.T) {
	client, cleanup := newStartedTestClient(t, nil)
	defer cleanup()
	require.NoError(t, client.Stop())

	beforeProtocolStart := make(chan struct{})
	allowProtocolStart := make(chan struct{})
	client.testStartBeforeProtocolStart = func() {
		close(beforeProtocolStart)
		<-allowProtocolStart
	}
	startDone := make(chan struct{})
	go func() {
		client.Start()
		close(startDone)
	}()
	select {
	case <-beforeProtocolStart:
	case <-time.After(time.Second):
		t.Fatal("Start did not reach the protocol-start fence")
	}

	require.NoError(t, client.Stop())
	client.lifecycleMutex.Lock()
	require.Equal(t, clientStateStopped, client.lifecycleState)
	require.Nil(t, client.startingDone)
	require.False(t, client.protocolStarted)
	client.lifecycleMutex.Unlock()

	close(allowProtocolStart)
	select {
	case <-startDone:
	case <-time.After(time.Second):
		t.Fatal("Start did not abort after Stop returned")
	}
	client.lifecycleMutex.Lock()
	require.Equal(t, clientStateStopped, client.lifecycleState)
	require.False(t, client.protocolStarted)
	client.lifecycleMutex.Unlock()
}

func TestStopCancelsAwaitReplyPipelineFence(t *testing.T) {
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseApply) })
	}
	p := pipeline.NewBlockPipeline(
		pipeline.WithDecodeWorkers(1),
		pipeline.WithValidateWorkers(0),
		pipeline.WithSkipBodyHashValidation(true),
		pipeline.WithApplyFunc(func(*pipeline.BlockItem) error {
			close(applyStarted)
			<-releaseApply
			return nil
		}),
	)
	require.NoError(t, p.Start(context.Background()))
	defer func() {
		release()
		require.NoError(t, p.Stop())
	}()

	clientConn, peerConn := net.Pipe()
	clientMuxer := muxer.New(clientConn)
	peerMuxer := muxer.New(peerConn)
	clientMuxer.Start()
	peerMuxer.Start()
	defer func() {
		_ = clientConn.Close()
		_ = peerConn.Close()
		clientMuxer.Stop()
		peerMuxer.Stop()
	}()
	peerSendChan, peerRecvChan, _ := peerMuxer.RegisterProtocol(
		ProtocolIdNtC,
		muxer.ProtocolRoleResponder,
	)

	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        clientMuxer,
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{Pipeline: p},
	)
	client.Start()
	defer func() { require.NoError(t, client.Stop()) }()

	require.NoError(
		t,
		p.Submit(
			context.Background(),
			uint(ledger.BlockTypeConway),
			testdata.MustDecodeHex(testdata.ConwayBlockHex),
			Tip{},
		),
	)
	select {
	case <-applyStarted:
	case <-time.After(time.Second):
		t.Fatal("pipeline apply did not start")
	}

	fenceStarted := make(chan struct{})
	client.testAwaitReplyBeforeFence = func() { close(fenceStarted) }
	require.NoError(t, client.SendMessage(NewMsgRequestNext()))
	select {
	case <-peerRecvChan:
	case <-time.After(time.Second):
		t.Fatal("client did not send RequestNext")
	}
	awaitReplyCbor, err := cbor.Encode(NewMsgAwaitReply())
	require.NoError(t, err)
	peerSendChan <- muxer.NewSegment(ProtocolIdNtC, awaitReplyCbor, true)
	select {
	case <-fenceStarted:
	case <-time.After(time.Second):
		t.Fatal("AwaitReply did not enter the pipeline fence")
	}

	stopDone := make(chan error, 1)
	go func() { stopDone <- client.Stop() }()
	select {
	case err := <-stopDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Stop waited for the blocked AwaitReply pipeline fence")
	}

	release()
}

func newStartedTestClient(t *testing.T, cfg *Config) (*Client, func()) {
	t.Helper()
	clientConn, peerConn := net.Pipe()
	m := muxer.New(clientConn)
	m.Start()
	peerDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, peerConn)
		close(peerDone)
	}()
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        m,
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		cfg,
	)
	client.Start()
	return client, func() {
		_ = client.Stop()
		_ = clientConn.Close()
		_ = peerConn.Close()
		m.Stop()
		<-peerDone
	}
}
