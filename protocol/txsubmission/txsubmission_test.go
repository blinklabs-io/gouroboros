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
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testAddr struct{}

func (a testAddr) Network() string { return "test" }
func (a testAddr) String() string  { return "test-addr" }

type testConn struct {
	writeChan chan []byte
	closed    bool
	closeChan chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
}

func newTestConn() *testConn {
	return &testConn{
		writeChan: make(chan []byte, 100),
		closeChan: make(chan struct{}),
	}
}

func (c *testConn) Read(b []byte) (n int, err error) { return 0, nil }
func (c *testConn) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		close(c.closeChan)
		c.closed = true
	})
	return nil
}
func (c *testConn) LocalAddr() net.Addr                { return testAddr{} }
func (c *testConn) RemoteAddr() net.Addr               { return testAddr{} }
func (c *testConn) SetDeadline(t time.Time) error      { return nil }
func (c *testConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *testConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *testConn) Write(b []byte) (n int, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, io.EOF
	}
	select {
	case c.writeChan <- b:
		return len(b), nil
	case <-c.closeChan:
		return 0, io.EOF
	}
}

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
	conn := newTestConn()
	defer conn.Close()
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
func TestCallbackRegistration(t *testing.T) {
	conn := newTestConn()
	defer conn.Close()

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
	conn := newTestConn()
	defer conn.Close()
	cfg := NewConfig()
	client := NewClient(getTestProtocolOptions(conn), &cfg)

	t.Run("Client can send messages", func(t *testing.T) {
		client.Start()
		defer client.Stop()

		err := client.SendMessage(NewMsgInit())
		require.NoError(t, err)

		select {
		case msg := <-conn.writeChan:
			assert.NotEmpty(t, msg, "expected message to be written")
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for message send")
		}
	})
}

func TestServerMessageHandling(t *testing.T) {
	conn := newTestConn()
	defer conn.Close()
	cfg := NewConfig()
	server := NewServer(getTestProtocolOptions(conn), &cfg)

	t.Run("Server can be started", func(t *testing.T) {
		server.Start()
		defer server.Stop()
		assert.NotNil(t, server)
	})
}
