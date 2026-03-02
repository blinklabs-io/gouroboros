// Copyright 2023 Blink Labs Software
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

package ouroboros_test

import (
	"fmt"
	"log/slog"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

// Ensure that we don't panic when closing the Connection object after a failed Dial() call
func TestDialFailClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	oConn, err := ouroboros.New()
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}
	err = oConn.Dial("unix", "/path/does/not/exist")
	if err == nil {
		t.Fatalf("did not get expected failure on Dial()")
	}
	// Close connection
	oConn.Close()
}

func TestDoubleClose(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	if err != nil {
		t.Fatalf("unexpected error when creating Connection object: %s", err)
	}
	// Async error handler
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		// We can't call t.Fatalf() from a different Goroutine, so we panic instead
		panic(fmt.Sprintf("unexpected Ouroboros connection error: %s", err))
	}()
	// Close connection
	if err := oConn.Close(); err != nil {
		t.Fatalf("unexpected error when closing Connection object: %s", err)
	}
	// Close connection again
	if err := oConn.Close(); err != nil {
		t.Fatalf(
			"unexpected error when closing Connection object again: %s",
			err,
		)
	}
	// Wait for connection shutdown
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

func TestConnectionOptions(t *testing.T) {
	defer goleak.VerifyNone(t)

	testCases := []struct {
		name   string
		option ouroboros.ConnectionOptionFunc
	}{
		{"WithNetworkMagic", ouroboros.WithNetworkMagic(12345)},
		{"WithServer", ouroboros.WithServer(true)},
		{"WithNodeToNode", ouroboros.WithNodeToNode(true)},
		{"WithKeepAlive", ouroboros.WithKeepAlive(true)},
		{"WithDelayMuxerStart", ouroboros.WithDelayMuxerStart(true)},
		{"WithDelayProtocolStart", ouroboros.WithDelayProtocolStart(true)},
		{"WithFullDuplex", ouroboros.WithFullDuplex(true)},
		{"WithPeerSharing", ouroboros.WithPeerSharing(true)},
		{"WithLogger", ouroboros.WithLogger(slog.Default())},
		{"WithErrorChan", ouroboros.WithErrorChan(make(chan error, 10))},
		{"WithNetwork", ouroboros.WithNetwork(ouroboros.NetworkMainnet)},
		{"WithBlockFetchConfig", ouroboros.WithBlockFetchConfig(blockfetch.Config{})},
		{"WithChainSyncConfig", ouroboros.WithChainSyncConfig(chainsync.Config{})},
		{"WithKeepAliveConfig", ouroboros.WithKeepAliveConfig(keepalive.Config{})},
		{"WithLocalStateQueryConfig", ouroboros.WithLocalStateQueryConfig(localstatequery.Config{})},
		{"WithLocalTxMonitorConfig", ouroboros.WithLocalTxMonitorConfig(localtxmonitor.Config{})},
		{"WithLocalTxSubmissionConfig", ouroboros.WithLocalTxSubmissionConfig(localtxsubmission.Config{})},
		{"WithPeerSharingConfig", ouroboros.WithPeerSharingConfig(peersharing.Config{})},
		{"WithTxSubmissionConfig", ouroboros.WithTxSubmissionConfig(txsubmission.Config{})},
		{"WithLeiosFetchConfig", ouroboros.WithLeiosFetchConfig(leiosfetch.Config{})},
		{"WithLeiosNotifyConfig", ouroboros.WithLeiosNotifyConfig(leiosnotify.Config{})},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := ouroboros.New(tc.option)
			require.NoError(t, err)
			require.NotNil(t, conn)
		})
	}
}

func TestConnectionOptionCombinations(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Test applying multiple options in sequence
	options := []ouroboros.ConnectionOptionFunc{
		ouroboros.WithNetworkMagic(12345),
		ouroboros.WithServer(false),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithDelayMuxerStart(false),
		ouroboros.WithFullDuplex(true),
		ouroboros.WithPeerSharing(true),
		ouroboros.WithLogger(slog.Default()),
	}

	conn, err := ouroboros.New(options...)
	require.NoError(t, err, "should create connection with multiple options")
	require.NotNil(t, conn, "connection should not be nil")
}

func TestConnectionErrorChan(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Test with provided error channel
	customErrChan := make(chan error, 10)
	conn, err := ouroboros.New(ouroboros.WithErrorChan(customErrChan))
	require.NoError(t, err)
	require.NotNil(t, conn)

	// ErrorChan should return the provided channel
	errChan := conn.ErrorChan()
	assert.NotNil(t, errChan, "ErrorChan should not be nil")
}

func TestConnectionErrorChanDefault(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Test without providing error channel - should create default
	conn, err := ouroboros.New()
	require.NoError(t, err)
	require.NotNil(t, conn)

	errChan := conn.ErrorChan()
	assert.NotNil(t, errChan, "ErrorChan should not be nil even without explicit config")
}

func TestConnectionMuxerBeforeSetup(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Connection without actual network connection should have nil muxer
	conn, err := ouroboros.New()
	require.NoError(t, err)
	require.NotNil(t, conn)

	muxer := conn.Muxer()
	assert.Nil(t, muxer, "Muxer should be nil before connection setup")
}

func TestConnectionIdBeforeSetup(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Connection without actual network connection should have zero ID
	conn, err := ouroboros.New()
	require.NoError(t, err)
	require.NotNil(t, conn)

	id := conn.Id()
	assert.Nil(t, id.LocalAddr, "LocalAddr should be nil before connection setup")
	assert.Nil(t, id.RemoteAddr, "RemoteAddr should be nil before connection setup")
}

func TestConnectionProtocolGettersBeforeSetup(t *testing.T) {
	defer goleak.VerifyNone(t)

	// Protocol getters should return nil before connection setup
	conn, err := ouroboros.New()
	require.NoError(t, err)
	require.NotNil(t, conn)

	assert.Nil(t, conn.BlockFetch(), "BlockFetch should be nil before setup")
	assert.Nil(t, conn.ChainSync(), "ChainSync should be nil before setup")
	assert.Nil(t, conn.Handshake(), "Handshake should be nil before setup")
	assert.Nil(t, conn.KeepAlive(), "KeepAlive should be nil before setup")
	assert.Nil(t, conn.LeiosFetch(), "LeiosFetch should be nil before setup")
	assert.Nil(t, conn.LeiosNotify(), "LeiosNotify should be nil before setup")
	assert.Nil(t, conn.LocalStateQuery(), "LocalStateQuery should be nil before setup")
	assert.Nil(t, conn.LocalTxMonitor(), "LocalTxMonitor should be nil before setup")
	assert.Nil(t, conn.LocalTxSubmission(), "LocalTxSubmission should be nil before setup")
	assert.Nil(t, conn.PeerSharing(), "PeerSharing should be nil before setup")
	assert.Nil(t, conn.TxSubmission(), "TxSubmission should be nil before setup")
}

func TestConnectionProtocolGettersAfterSetup(t *testing.T) {
	defer goleak.VerifyNone(t)

	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
	)
	conn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)

	go func() {
		for range conn.ErrorChan() {
			// drain errors
		}
	}()

	// NtC protocols should be initialized after handshake
	assert.NotNil(t, conn.ChainSync())
	assert.NotNil(t, conn.Handshake())
	assert.NotNil(t, conn.LocalTxSubmission())

	// NtN protocols should be nil in NtC mode
	assert.Nil(t, conn.BlockFetch())
	assert.Nil(t, conn.TxSubmission())

	// Muxer and ID should be valid after setup
	assert.NotNil(t, conn.Muxer())
	id := conn.Id()
	assert.NotNil(t, id.LocalAddr)
	assert.NotNil(t, id.RemoteAddr)

	err = conn.Close()
	assert.NoError(t, err)
}

func TestConnectionProtocolVersion(t *testing.T) {
	defer goleak.VerifyNone(t)

	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
	)
	conn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	require.NoError(t, err)
	require.NotNil(t, conn)

	go func() {
		for range conn.ErrorChan() {
			// drain errors
		}
	}()

	version, _ := conn.ProtocolVersion()
	assert.NotZero(t, version, "Protocol version should be non-zero after handshake")

	err = conn.Close()
	assert.NoError(t, err)
}
