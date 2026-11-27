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
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// lockOrderWait bounds how long the test waits for a lock that a correct
// lock order always frees.
const lockOrderWait = 5 * time.Second

// waitBlockedIn waits until some goroutine is blocked taking a sync.Mutex
// with fn on its stack. The test cannot otherwise tell that the handler has
// passed its first read of the instance and queued on stateMu.
func waitBlockedIn(t *testing.T, fn string) {
	t.Helper()
	deadline := time.Now().Add(lockOrderWait)
	buf := make([]byte, 1<<20)
	for time.Now().Before(deadline) {
		n := runtime.Stack(buf, true)
		// A full buffer may have cut off the goroutine being looked for.
		if n == len(buf) {
			buf = make([]byte, 2*len(buf))
			continue
		}
		for _, g := range strings.Split(string(buf[:n]), "\n\n") {
			if strings.Contains(g, "sync.(*Mutex).Lock") &&
				strings.Contains(g, fn) {
				return
			}
		}
		runtime.Gosched()
	}
	t.Fatalf("no goroutine blocked on a mutex in %s", fn)
}

// TestServerInitDoesNotInvertLockOrder stages three goroutines: a request
// caller that holds protocolMu for reading and is about to take stateMu, as
// RequestTxIds and RequestTxs do; Stop waiting for protocolMu; and the
// handler running MsgInit. sync.RWMutex blocks a new reader while a writer
// waits, so a handler that reads the protocol instance while it holds
// stateMu waits for Stop, Stop waits for the caller, and the caller waits
// for the handler.
func TestServerInitDoesNotInvertLockOrder(t *testing.T) {
	t.Parallel()
	s := NewServer(
		protocol.ProtocolOptions{Mode: protocol.ProtocolModeNodeToNode},
		&Config{},
	)

	// The handler queues on stateMu first, so that it, and not the
	// caller, is the next to take it.
	s.stateMu.Lock()
	handled := make(chan error, 1)
	go func() {
		handled <- s.messageHandler(NewMsgInit())
	}()
	waitBlockedIn(t, "txsubmission.(*Server).handleInit")

	// The caller's read lock, as RequestTxIds holds it between taking
	// protocolMu and stateMu.
	s.protocolMu.RLock()
	stopped := make(chan error, 1)
	go func() {
		stopped <- s.Stop(context.Background())
	}()
	// TryRLock fails once Stop waits for protocolMu.
	for {
		if !s.protocolMu.TryRLock() {
			break
		}
		s.protocolMu.RUnlock()
		runtime.Gosched()
	}

	// Let the handler take stateMu, then wait until it holds it or has
	// finished with it.
	s.stateMu.Unlock()
	for {
		select {
		case err := <-handled:
			handled <- err
		default:
			if s.stateMu.TryLock() {
				s.stateMu.Unlock()
				runtime.Gosched()
				continue
			}
		}
		break
	}

	// The caller now takes stateMu.
	locked := make(chan struct{})
	go func() {
		s.stateMu.Lock()
		close(locked)
		s.stateMu.Unlock()
	}()
	select {
	case <-locked:
	case <-time.After(lockOrderWait):
		// Release the caller's read lock so every goroutine can finish.
		s.protocolMu.RUnlock()
		<-locked
		<-stopped
		<-handled
		t.Fatal(
			"the MsgInit handler waits for protocolMu while holding stateMu",
		)
	}
	s.protocolMu.RUnlock()
	require.NoError(t, <-stopped)
	require.NoError(t, <-handled)
}
