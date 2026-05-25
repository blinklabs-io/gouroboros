package main

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
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

	if !conn.dialed {
		t.Error("Expected connection to be dialed")
	}
	if conn.networkType != "tcp" {
		t.Error("Expected TCP connection")
	}
	if result.Error != nil {
		t.Errorf("Expected no error, got: %v", result.Error)
	}
	if result.ConnectionTime <= 0 {
		t.Error("Expected positive connection time")
	}
	if result.ProtocolTime <= 0 {
		t.Error("Expected positive protocol time")
	}
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

	if result.Error == nil ||
		result.Error.Error() != "protocol error: protocol error" {
		t.Errorf("Expected protocol error, got: %v", result.Error)
	}
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

	if result.Error == nil ||
		result.Error.Error() != "connection failed: connection failed" {
		t.Errorf("Expected connection error, got: %v", result.Error)
	}
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
	if err != nil {
		t.Fatalf("failed to create connection: %v", err)
	}
	defer conn.Close()

	resultChan := make(chan PingResult)
	go func() {
		resultChan <- PingNode(conn, cfg)
	}()

	select {
	case result := <-resultChan:
		if result.Error != nil {
			t.Fatalf("ping failed: %v", result.Error)
		}
		t.Logf("Integration test successful:")
		t.Logf("Connection time: %v", result.ConnectionTime)
		t.Logf("Protocol time: %v", result.ProtocolTime)
	case <-time.After(30 * time.Second):
		t.Fatal("test timed out after 30 seconds")
	}
}
