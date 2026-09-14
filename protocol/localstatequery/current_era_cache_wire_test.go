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
	"net"
	"sync/atomic"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

type wireCacheHarness struct {
	client  *ouroboros.Connection
	server  *ouroboros.Connection
	queries atomic.Int32
}

func newWireCacheHarness(t *testing.T, failFirst bool) *wireCacheHarness {
	t.Helper()
	clientRaw, serverRaw := net.Pipe()
	t.Cleanup(func() {
		_ = clientRaw.Close()
		_ = serverRaw.Close()
	})
	h := &wireCacheHarness{}
	cfg := localstatequery.NewConfig(
		localstatequery.WithAcquireTimeout(time.Second),
		localstatequery.WithQueryTimeout(time.Second),
		localstatequery.WithAcquireFunc(
			func(localstatequery.CallbackContext, localstatequery.AcquireTarget, bool) error {
				return nil
			},
		),
		localstatequery.WithQueryFunc(
			func(_ localstatequery.CallbackContext, query localstatequery.QueryWrapper) (any, error) {
				h.queries.Add(1)
				block, ok := query.Query.(*localstatequery.BlockQuery)
				if !ok {
					t.Errorf("unexpected query shape: %T", query.Query)
				} else if hardFork, ok := block.Query.(*localstatequery.HardForkQuery); !ok {
					t.Errorf("unexpected block query shape: %T", block.Query)
				} else if _, ok := hardFork.Query.(*localstatequery.HardForkCurrentEraQuery); !ok {
					t.Errorf("unexpected hard-fork query shape: %T", hardFork.Query)
				}
				if failFirst && h.queries.Load() == 1 {
					return "not an era", nil
				}
				return int(h.queries.Load() + 4), nil
			},
		),
		localstatequery.WithReleaseFunc(
			func(localstatequery.CallbackContext) error { return nil },
		),
	)
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		var err error
		h.server, err = ouroboros.New(
			ouroboros.WithConnection(serverRaw),
			ouroboros.WithServer(true),
			ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
			ouroboros.WithLocalStateQueryConfig(cfg),
		)
		if err != nil {
			t.Errorf("server connection: %v", err)
		}
	}()
	var err error
	h.client, err = ouroboros.New(
		ouroboros.WithConnection(clientRaw),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		ouroboros.WithLocalStateQueryConfig(cfg),
	)
	require.NoError(t, err)
	select {
	case <-serverDone:
	case <-time.After(5 * time.Second):
		t.Fatal("server handshake timed out")
	}
	require.NotNil(t, h.server)
	t.Cleanup(func() {
		if h.client != nil {
			_ = h.client.Close()
		}
		if h.server != nil {
			_ = h.server.Close()
		}
		goleak.VerifyNone(t)
	})
	return h
}

func TestCurrentEraCacheAcrossSnapshotTransitions(t *testing.T) {
	h := newWireCacheHarness(t, false)
	client := h.client.LocalStateQuery().Client
	era, err := client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 5, era)
	era, err = client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(
		t,
		int32(1),
		h.queries.Load(),
		"successful result should be cached",
	)
	require.Equal(t, 5, era)
	require.NoError(t, client.Release())
	require.NoError(t, client.AcquireVolatileTip())
	era, err = client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 6, era)
	require.Equal(
		t,
		int32(2),
		h.queries.Load(),
		"successful reacquire should invalidate cache",
	)
}

func TestCurrentEraCacheInvalidatesAfterReacquire(t *testing.T) {
	h := newWireCacheHarness(t, false)
	client := h.client.LocalStateQuery().Client
	era, err := client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 5, era)
	require.NoError(t, client.AcquireVolatileTip())
	era, err = client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 6, era)
	require.Equal(
		t,
		int32(2),
		h.queries.Load(),
		"successful reacquire should invalidate cache",
	)
}

func TestCurrentEraQueryFailureDoesNotPopulateCache(t *testing.T) {
	h := newWireCacheHarness(t, true)
	client := h.client.LocalStateQuery().Client
	_, err := client.GetCurrentEra()
	require.Error(t, err)
	era, err := client.GetCurrentEra()
	require.NoError(t, err)
	require.Equal(t, 6, era)
	require.Equal(
		t,
		int32(2),
		h.queries.Load(),
		"failed result must not populate cache",
	)
}
