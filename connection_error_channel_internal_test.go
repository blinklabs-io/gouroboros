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

package ouroboros

import (
	"errors"
	"net"
	"testing"
	"time"

	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

const errorForwardingTestTimeout = 5 * time.Second

type injectableReadErrorConn struct {
	net.Conn
	injectedError chan error
	errorReturned chan struct{}
}

func newInjectableReadErrorConn(conn net.Conn) *injectableReadErrorConn {
	return &injectableReadErrorConn{
		Conn:          conn,
		injectedError: make(chan error, 1),
		errorReturned: make(chan struct{}),
	}
}

func (c *injectableReadErrorConn) Read(buf []byte) (int, error) {
	type readResult struct {
		count int
		err   error
	}
	resultChan := make(chan readResult, 1)
	go func() {
		count, err := c.Conn.Read(buf)
		resultChan <- readResult{count: count, err: err}
	}()
	select {
	case err := <-c.injectedError:
		_ = c.Conn.Close()
		<-resultChan
		close(c.errorReturned)
		return 0, err
	case result := <-resultChan:
		return result.count, result.err
	}
}

func (c *injectableReadErrorConn) injectReadError(err error) {
	c.injectedError <- err
}

func newErrorForwardingTestConnection(
	t *testing.T,
	errorChan chan error,
) (*Connection, *injectableReadErrorConn) {
	t.Helper()
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryHandshakeNtCResponse,
		},
	)
	transport := newInjectableReadErrorConn(mockConn)
	conn, err := New(
		WithConnection(transport),
		WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
		WithErrorChan(errorChan),
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
		_ = mockConn.Close()
	})
	return conn, transport
}

func requireChannelSignal[T any](
	t *testing.T,
	signal <-chan T,
	message string,
) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(errorForwardingTestTimeout):
		t.Fatal(message)
	}
}

func requireInjectedReadError(
	t *testing.T,
	conn *injectableReadErrorConn,
	err error,
) {
	t.Helper()
	conn.injectReadError(err)
	requireChannelSignal(
		t,
		conn.errorReturned,
		"injected read error was not returned",
	)
}

func requireUndrainedErrorDoesNotBlockShutdown(
	t *testing.T,
	conn *Connection,
	errorChan <-chan error,
) {
	t.Helper()
	select {
	case <-conn.doneChan:
	case <-time.After(errorForwardingTestTimeout):
		// Release an implementation with a blocking send so the failed
		// regression does not leave teardown goroutines behind.
		select {
		case <-errorChan:
		case <-time.After(errorForwardingTestTimeout):
			t.Fatal("error forwarding did not reach the caller channel")
		}
		requireChannelSignal(
			t,
			conn.doneChan,
			"error delivery did not initiate connection shutdown after resuming",
		)
		conn.shutdown()
		t.Fatal("undrained error channel prevented error-triggered shutdown")
	}

	// A second call blocks in sync.Once until the shutdown initiated above
	// has waited for all forwarding goroutines.
	shutdownDone := make(chan struct{})
	go func() {
		conn.shutdown()
		close(shutdownDone)
	}()
	select {
	case <-shutdownDone:
		return
	case <-time.After(errorForwardingTestTimeout):
		t.Fatal("connection shutdown did not wait for error forwarding")
	}
}

func TestMuxerErrorForwardingCancelsDuringShutdown(t *testing.T) {
	errorChan := make(chan error)
	conn, transport := newErrorForwardingTestConnection(t, errorChan)
	requireInjectedReadError(t, transport, errors.New("muxer failure"))
	requireUndrainedErrorDoesNotBlockShutdown(t, conn, errorChan)
}

func TestProtocolErrorForwardingCancelsDuringShutdown(t *testing.T) {
	errorChan := make(chan error)
	conn, _ := newErrorForwardingTestConnection(t, errorChan)
	forwardingStarted := make(chan struct{})
	go func() {
		conn.protoErrorChan <- errors.New("protocol failure")
		close(forwardingStarted)
	}()
	requireChannelSignal(
		t,
		forwardingStarted,
		"connection did not consume a protocol error",
	)
	requireUndrainedErrorDoesNotBlockShutdown(t, conn, errorChan)
}

func TestConnectionsDoNotCloseCallerErrorChannel(t *testing.T) {
	errorChan := make(chan error, 1)
	connections := make([]*Connection, 2)
	for i := range connections {
		var err error
		connections[i], err = New(WithErrorChan(errorChan))
		require.NoError(t, err)
	}

	require.NotPanics(t, func() {
		for _, conn := range connections {
			conn.shutdown()
		}
	})

	wantErr := errors.New("caller still owns channel")
	require.NotPanics(t, func() {
		errorChan <- wantErr
	})
	require.ErrorIs(t, <-errorChan, wantErr)
}

func TestConnectionClosesInternalErrorChannel(t *testing.T) {
	conn, err := New()
	require.NoError(t, err)
	errorChan := conn.ErrorChan()
	conn.shutdown()

	select {
	case _, ok := <-errorChan:
		require.False(t, ok, "internally-created error channel should close")
	case <-time.After(errorForwardingTestTimeout):
		t.Fatal("internally-created error channel remained open")
	}
}

func TestCloseWaitsForErrorForwardersBeforeCallerClosesChannel(t *testing.T) {
	errorChan := make(chan error, 1)
	conn := &Connection{
		errorChan:      errorChan,
		doneChan:       make(chan any),
		connClosedChan: make(chan struct{}),
	}
	go func() {
		<-conn.doneChan
		conn.shutdown()
	}()

	forwarderStarted := make(chan struct{})
	releaseForwarder := make(chan struct{})
	forwarderDone := make(chan struct{})
	conn.waitGroup.Go(func() {
		defer close(forwarderDone)
		close(forwarderStarted)
		<-releaseForwarder
		conn.forwardError(errors.New("late connection error"))
	})
	requireChannelSignal(
		t,
		forwarderStarted,
		"error forwarder did not start",
	)

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- conn.Close()
	}()
	requireChannelSignal(t, conn.doneChan, "connection shutdown did not start")
	select {
	case <-conn.connClosedChan:
		close(releaseForwarder)
		err := <-closeResult
		conn.shutdown()
		require.NoError(t, err)
		t.Fatal("shutdown signaled completion before error forwarding stopped")
	case <-time.After(100 * time.Millisecond):
		// Shutdown must remain blocked while the tracked forwarder is blocked.
	}

	close(releaseForwarder)
	select {
	case err := <-closeResult:
		require.NoError(t, err)
	case <-time.After(errorForwardingTestTimeout):
		t.Fatal("Close did not return after error forwarding stopped")
	}
	select {
	case <-forwarderDone:
	default:
		t.Fatal("Close returned before the error forwarder finished")
	}
	require.NotPanics(t, func() {
		close(errorChan)
	})
}

func TestConnectionErrorForwardingDeliversNormally(t *testing.T) {
	testCases := []struct {
		name    string
		trigger func(
			*testing.T,
			*Connection,
			*injectableReadErrorConn,
			error,
		)
		wantErr string
	}{
		{
			name: "muxer",
			trigger: func(
				t *testing.T,
				_ *Connection,
				transport *injectableReadErrorConn,
				err error,
			) {
				requireInjectedReadError(t, transport, err)
			},
			wantErr: "muxer error: delivery failure",
		},
		{
			name: "protocol",
			trigger: func(
				_ *testing.T,
				conn *Connection,
				_ *injectableReadErrorConn,
				err error,
			) {
				conn.protoErrorChan <- err
			},
			wantErr: "protocol error: delivery failure",
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			errorChan := make(chan error, 1)
			conn, transport := newErrorForwardingTestConnection(t, errorChan)
			testCase.trigger(
				t,
				conn,
				transport,
				errors.New("delivery failure"),
			)

			select {
			case err := <-errorChan:
				require.EqualError(t, err, testCase.wantErr)
			case <-time.After(errorForwardingTestTimeout):
				t.Fatalf("%s error was not delivered", testCase.name)
			}
		})
	}
}
