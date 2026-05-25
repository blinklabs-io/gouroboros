// Copyright 2024 Blink Labs Software
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

package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testConnection implements a simplified NodeConnection for testing
type testConnection struct {
	dialFunc    func(string, string) error
	getTipFunc  func() (*chainsync.Tip, error)
	closeFunc   func() error
	dialed      bool
	networkType string
}

func (t *testConnection) Dial(network, address string) error {
	t.dialed = true
	t.networkType = network
	if t.dialFunc != nil {
		return t.dialFunc(network, address)
	}
	return nil
}

func (t *testConnection) ChainSync() *chainsync.ChainSync {
	// We don't actually need the real ChainSync implementation for testing
	// We'll override getTip globally for our tests
	return nil
}

func (t *testConnection) Close() error {
	if t.closeFunc != nil {
		return t.closeFunc()
	}
	return nil
}

func restoreDefaultGetTip() {
	getTip = func(sync *chainsync.ChainSync) (*chainsync.Tip, error) {
		return sync.Client.GetCurrentTip()
	}
}

func TestPingNode_Success(t *testing.T) {
	// Override getTip for this test
	getTip = func(_ *chainsync.ChainSync) (*chainsync.Tip, error) {
		return &chainsync.Tip{}, nil
	}
	defer restoreDefaultGetTip()

	conn := &testConnection{}

	cfg := &Config{
		Address: "test.example.com:3001",
		Network: "testnet",
	}

	result := PingNode(conn, cfg)

	assert.True(t, conn.dialed, "Expected connection to be dialed")
	assert.Equal(t, "tcp", conn.networkType, "Expected TCP connection")
	require.NoError(t, result.Error)
	assert.Positive(t, result.ConnectionTime, "Expected positive connection time")
	assert.Positive(t, result.ProtocolTime, "Expected positive protocol time")
}

func TestPingNode_ProtocolError(t *testing.T) {
	// Override getTip for this test
	getTip = func(_ *chainsync.ChainSync) (*chainsync.Tip, error) {
		return nil, errors.New("protocol error")
	}
	defer restoreDefaultGetTip()

	conn := &testConnection{}

	cfg := &Config{
		Address: "test.example.com:3001",
		Network: "testnet",
	}

	result := PingNode(conn, cfg)

	require.Error(t, result.Error)
	assert.Equal(t, "protocol error: protocol error", result.Error.Error())
}

func TestPingNode_ConnectionError(t *testing.T) {
	// Override getTip for this test
	getTip = func(_ *chainsync.ChainSync) (*chainsync.Tip, error) {
		t.Error("getTip should not be called when connection fails")
		return nil, nil
	}
	defer restoreDefaultGetTip()

	conn := &testConnection{
		dialFunc: func(_, _ string) error {
			return errors.New("connection failed")
		},
	}

	cfg := &Config{
		Address: "test.example.com:3001",
		Network: "testnet",
	}

	result := PingNode(conn, cfg)

	require.Error(t, result.Error)
	assert.Equal(t, "connection failed: connection failed", result.Error.Error())
}

func TestPingNode_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	socketPath := os.Getenv("CARDANO_NODE_SOCKET_PATH")
	network := os.Getenv("CARDANO_NODE_NETWORK")
	address := os.Getenv("CARDANO_NODE_ADDRESS")

	if socketPath == "" && address == "" {
		t.Skip(
			"skipping integration test: set CARDANO_NODE_SOCKET_PATH or CARDANO_NODE_ADDRESS to run",
		)
	}

	cfg := &Config{
		SocketPath: socketPath,
		Address:    address,
		Network:    network,
	}

	conn, err := NewConnection(cfg)
	require.NoError(t, err, "failed to create connection")
	defer conn.Close()

	resultChan := make(chan PingResult, 1)
	go func() {
		resultChan <- PingNode(conn, cfg)
	}()

	select {
	case result := <-resultChan:
		require.NoError(t, result.Error, "ping failed")
		t.Logf("Integration test successful:")
		t.Logf("Connection time: %v", result.ConnectionTime)
		t.Logf("Protocol time: %v", result.ProtocolTime)
	case <-time.After(30 * time.Second):
		t.Fatal("test timed out after 30 seconds")
	}
}
