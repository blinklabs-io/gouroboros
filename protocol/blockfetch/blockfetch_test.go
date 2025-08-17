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
package blockfetch

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
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

func TestNewBlockFetch(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	bf := New(getTestProtocolOptions(conn), &cfg)
	assert.NotNil(t, bf.Client)
	assert.NotNil(t, bf.Server)
}

func TestConfigOptions(t *testing.T) {
	t.Run("Default config", func(t *testing.T) {
		cfg := NewConfig()
		assert.Equal(t, 5*time.Second, cfg.BatchStartTimeout)
		assert.Equal(t, 60*time.Second, cfg.BlockTimeout)
	})

	t.Run("Custom config", func(t *testing.T) {
		cfg := NewConfig(
			WithBatchStartTimeout(10*time.Second),
			WithBlockTimeout(30*time.Second),
			WithRecvQueueSize(100),
		)
		assert.Equal(t, 10*time.Second, cfg.BatchStartTimeout)
		assert.Equal(t, 30*time.Second, cfg.BlockTimeout)
		assert.Equal(t, 100, cfg.RecvQueueSize)
	})
}

func TestConnectionErrorHandling(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	bf := New(getTestProtocolOptions(conn), &cfg)

	t.Run("Non-EOF error when not done", func(t *testing.T) {
		err := bf.HandleConnectionError(errors.New("test error"))
		assert.Error(t, err)
	})

	t.Run("EOF error when not done", func(t *testing.T) {
		err := bf.HandleConnectionError(io.EOF)
		assert.Error(t, err)
	})

	t.Run("Connection reset error", func(t *testing.T) {
		err := bf.HandleConnectionError(errors.New("connection reset by peer"))
		assert.Error(t, err)
	})
}

func TestCallbackRegistration(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}

	t.Run("Block callback registration", func(t *testing.T) {
		blockFunc := func(ctx CallbackContext, slot uint, block ledger.Block) error {
			return nil
		}
		cfg := NewConfig(WithBlockFunc(blockFunc))
		client := NewClient(getTestProtocolOptions(conn), &cfg)
		assert.NotNil(t, client)
		assert.NotNil(t, cfg.BlockFunc)
	})

	t.Run("RequestRange callback registration", func(t *testing.T) {
		requestRangeFunc := func(ctx CallbackContext, start, end common.Point) error {
			return nil
		}
		cfg := NewConfig(WithRequestRangeFunc(requestRangeFunc))
		server := NewServer(getTestProtocolOptions(conn), &cfg)
		assert.NotNil(t, server)
		assert.NotNil(t, cfg.RequestRangeFunc)
	})
}

func TestClientMessageSending(t *testing.T) {
	conn := &testConn{writeChan: make(chan []byte, 1)}
	cfg := NewConfig()
	client := NewClient(getTestProtocolOptions(conn), &cfg)

	t.Run("Client can send messages", func(t *testing.T) {
		// Start the client protocol
		client.Start()

		// Send a done message
		err := client.Protocol.SendMessage(NewMsgClientDone())
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

	})
}
