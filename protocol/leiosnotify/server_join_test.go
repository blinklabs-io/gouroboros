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

package leiosnotify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// TestStopWaitsForRequestSource checks that Stop waits for a RequestNextFunc
// that is still running, bounded by its context, and that no goroutine is
// left once it returns. The reference runs a mini-protocol's work in its own
// thread, which the mux waits for before it returns (network-mux
// Network/Mux.hs:280-287).
func TestStopWaitsForRequestSource(t *testing.T) {
	// Registered first, so it runs after every other cleanup.
	t.Cleanup(func() { goleak.VerifyNone(t) })
	entered := make(chan struct{})
	release := make(chan struct{})
	server, conn := newQuitTestServer(t, NewConfig(
		WithRequestNextFunc(func(CallbackContext) (protocol.Message, error) {
			close(entered)
			// This source ignores QuitChan.
			<-release
			return nil, errors.New("released")
		}),
	))
	writeQuitTestMessage(t, conn, NewMsgNotificationRequestNext())
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("request source did not run")
	}
	// Stop cannot finish while the source runs; this bounds how long the
	// test gives it to do so wrongly.
	short, cancelShort := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancelShort()
	require.ErrorIs(
		t, server.Stop(short), context.DeadlineExceeded,
		"Stop returned while the request source was running",
	)
	close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, server.Stop(ctx))
}
