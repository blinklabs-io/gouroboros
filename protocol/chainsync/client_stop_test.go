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
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
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
