// Copyright 2025 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package txsubmission

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
)

// testAddr implements net.Addr for testing
type testAddr struct{}

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return "test-addr" }

// testConn implements net.Conn for testing with buffered writes
type testConn struct {
	writeChan chan []byte
}

func (c *testConn) Read(b []byte) (n int, err error) { return 0, nil }
func (c *testConn) Write(b []byte) (n int, err error) {
	c.writeChan <- b
	return len(b), nil
}
func (c *testConn) Close() error                       { return nil }
func (c *testConn) LocalAddr() net.Addr                { return testAddr{} }
func (c *testConn) RemoteAddr() net.Addr               { return testAddr{} }
func (c *testConn) SetDeadline(t time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(t time.Time) error { return nil }

func getTestProtocolOptions(conn net.Conn) protocol.ProtocolOptions {
	mux := muxer.New(conn)
	return protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  testAddr{},
			RemoteAddr: testAddr{},
		},
		Muxer:  mux,
		Logger: slog.Default(),
	}
}

func TestNewTxSubmission(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	ts := New(getTestProtocolOptions(conn), &cfg)
	assert.NotNil(t, ts.Client)
	assert.NotNil(t, ts.Server)
}

func TestConfigOptions(t *testing.T) {
	t.Run("Default config", func(t *testing.T) {
		cfg := NewConfig()
		assert.Equal(t, 300*time.Second, cfg.IdleTimeout)
	})

	t.Run("Custom config", func(t *testing.T) {
		cfg := NewConfig(
			WithIdleTimeout(60*time.Second),
			WithRequestTxIdsFunc(func(ctx CallbackContext, blocking bool, ack, req uint16) ([]TxIdAndSize, error) {
				return nil, nil
			}),
			WithRequestTxsFunc(func(ctx CallbackContext, txIds []TxId) ([]TxBody, error) {
				return nil, nil
			}),
		)
		assert.Equal(t, 60*time.Second, cfg.IdleTimeout)
		assert.NotNil(t, cfg.RequestTxIdsFunc)
		assert.NotNil(t, cfg.RequestTxsFunc)
	})
}

func TestConnectionErrorHandling(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	ts := New(getTestProtocolOptions(conn), &cfg)

	t.Run("Non-EOF error when not done", func(t *testing.T) {
		err := ts.HandleConnectionError(errors.New("test error"))
		assert.Error(t, err)
	})

	t.Run("EOF error when not done", func(t *testing.T) {
		err := ts.HandleConnectionError(io.EOF)
		assert.Error(t, err)
	})

	t.Run("Connection reset error when not done", func(t *testing.T) {
		err := ts.HandleConnectionError(errors.New("connection reset by peer"))
		assert.Error(t, err)
	})

	t.Run("EOF error when done", func(t *testing.T) {
		ts.currentState = stateDone
		err := ts.HandleConnectionError(io.EOF)
		assert.NoError(t, err)
	})
}

func TestCallbackRegistration(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}

	t.Run("RequestTxIds callback registration", func(t *testing.T) {
		requestTxIdsFunc := func(ctx CallbackContext, blocking bool, ack, req uint16) ([]TxIdAndSize, error) {
			return nil, nil
		}
		cfg := NewConfig(WithRequestTxIdsFunc(requestTxIdsFunc))
		server := NewServer(getTestProtocolOptions(conn), &cfg)
		assert.NotNil(t, server)
		assert.NotNil(t, cfg.RequestTxIdsFunc)
	})

	t.Run("RequestTxs callback registration", func(t *testing.T) {
		requestTxsFunc := func(ctx CallbackContext, txIds []TxId) ([]TxBody, error) {
			return nil, nil
		}
		cfg := NewConfig(WithRequestTxsFunc(requestTxsFunc))
		server := NewServer(getTestProtocolOptions(conn), &cfg)
		assert.NotNil(t, server)
		assert.NotNil(t, cfg.RequestTxsFunc)
	})
}

func TestClientMessageSending(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	client := NewClient(getTestProtocolOptions(conn), &cfg)

	t.Run("Client can send messages", func(t *testing.T) {
		// Start the client protocol
		client.Start()

		// Send an init message
		initMsg := NewMsgInit()
		err := client.Protocol.SendMessage(initMsg)
		assert.NoError(t, err)

		// Verify message was written to connection
		select {
		case <-conn.writeChan:
			// Message was sent successfully
		case <-time.After(100 * time.Millisecond):
			t.Fatal("timeout waiting for message send")
		}
	})
}

func TestServerMessageHandling(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	server := NewServer(getTestProtocolOptions(conn), &cfg)

	t.Run("Server can be started", func(t *testing.T) {
		server.Start()
		assert.NotNil(t, server)
	})
}

func TestIsConnectionReset(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "connection reset",
			err:      errors.New("connection reset by peer"),
			expected: true,
		},
		{
			name:     "broken pipe",
			err:      errors.New("broken pipe"),
			expected: true,
		},
		{
			name:     "other error",
			err:      errors.New("other error"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isConnectionReset(tt.err))
		})
	}
}
