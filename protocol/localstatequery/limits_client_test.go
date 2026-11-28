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

package localstatequery_test

import (
	"context"
	"net"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// newLimitsPair connects a client using clientCfg to a server whose every
// query answers result.
func newLimitsPair(
	t *testing.T,
	clientCfg localstatequery.Config,
	result any,
) *ouroboros.Connection {
	t.Helper()
	clientRaw, serverRaw := net.Pipe()
	t.Cleanup(func() {
		_ = clientRaw.Close()
		_ = serverRaw.Close()
	})
	serverCfg := localstatequery.NewConfig(
		localstatequery.WithAcquireFunc(
			func(localstatequery.CallbackContext, localstatequery.AcquireTarget, bool) error {
				return nil
			},
		),
		localstatequery.WithQueryFunc(
			func(localstatequery.CallbackContext, localstatequery.QueryWrapper) (any, error) {
				return result, nil
			},
		),
		localstatequery.WithReleaseFunc(
			func(localstatequery.CallbackContext) error { return nil },
		),
	)
	serverResult := make(chan *ouroboros.Connection, 1)
	go func() {
		server, err := ouroboros.New(
			ouroboros.WithConnection(serverRaw),
			ouroboros.WithServer(true),
			ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			ouroboros.WithLocalStateQueryConfig(serverCfg),
		)
		if err != nil {
			t.Errorf("server connection: %v", err)
		}
		serverResult <- server
	}()
	client, err := ouroboros.New(
		ouroboros.WithConnection(clientRaw),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithLocalStateQueryConfig(clientCfg),
	)
	require.NoError(t, err)
	var server *ouroboros.Connection
	select {
	case server = <-serverResult:
	case <-time.After(5 * time.Second):
		t.Fatal("server handshake timed out")
	}
	t.Cleanup(func() {
		_ = client.Close(context.Background())
		if server != nil {
			_ = server.Close(context.Background())
		}
	})
	return client
}

// TestOversizedResultReleasesCaller checks that a result larger than
// MaxReadBufferSize is refused: the connection reports the overflow and
// shuts down, and the call waiting for the result returns instead of
// waiting forever.
func TestOversizedResultReleasesCaller(t *testing.T) {
	const limit = 256 * 1024
	client := newLimitsPair(
		t,
		localstatequery.NewConfig(localstatequery.WithMaxReadBufferSize(limit)),
		make([]byte, 2*limit),
	)
	callErr := make(chan error, 1)
	go func() {
		_, err := client.LocalStateQuery().Client.GetCurrentEra()
		callErr <- err
	}()
	select {
	case err := <-callErr:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(5 * time.Second):
		t.Fatal("call still waiting after an oversized result")
	}
	select {
	case err, ok := <-client.ErrorChan():
		require.True(t, ok, "connection closed without reporting the overflow")
		require.ErrorContains(t, err, "exceeded")
	case <-time.After(5 * time.Second):
		t.Fatal("connection did not report the oversized result")
	}
}

// TestResultWithinLimitIsDelivered is the control for the test above: the
// same limit, a result that fits, and the call gets it.
func TestResultWithinLimitIsDelivered(t *testing.T) {
	const limit = 256 * 1024
	client := newLimitsPair(
		t,
		localstatequery.NewConfig(localstatequery.WithMaxReadBufferSize(limit)),
		6,
	)
	era, err := client.LocalStateQuery().Client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 6, era)
}

// TestAcquireTimeoutEndsWithAcquiring checks that the AcquireTimeout
// armed in Acquiring belongs to that state: once the state is acquired the
// client may sit in Acquired, where it holds agency, for well past the
// timeout without the connection failing, and a query is still answered.
func TestAcquireTimeoutEndsWithAcquiring(t *testing.T) {
	client := newLimitsPair(
		t,
		localstatequery.NewConfig(
			localstatequery.WithAcquireTimeout(optInTimeout),
			localstatequery.WithQueryTimeout(optInTimeout),
		),
		6,
	)
	lsq := client.LocalStateQuery().Client
	require.NoError(t, lsq.AcquireVolatileTip())
	select {
	case err := <-client.ErrorChan():
		t.Fatalf("connection failed while acquired: %v", err)
	case <-time.After(5 * optInTimeout):
	}
	era, err := lsq.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 6, era)
	select {
	case err := <-client.ErrorChan():
		t.Fatalf("connection failed after the query: %v", err)
	case <-time.After(5 * optInTimeout):
	}
	require.NoError(t, lsq.Release())
}
