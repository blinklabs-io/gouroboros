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

package protocol

import (
	"errors"
	"fmt"
	"math"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// panicValue is the value every injected panic in this file carries, so that a
// contained panic can be matched back to the handler that raised it.
const panicValue = "injected handler panic"

// errInjectedHandler is an ordinary handler failure, used to confirm that
// containing panics did not change what a non-panicking failure produces.
var errInjectedHandler = errors.New("injected handler error")

// panicTestState is the state every protocol in this file starts in. The
// client holds agency, so a server-role protocol is immediately ready to
// receive.
var (
	panicTestInitialState = NewState(1, "Initial")
	panicTestNextState    = NewState(2, "Next")
)

// startPanicTestProtocol wires a server-role protocol to a real muxer over an
// in-memory connection and starts it. The returned function delivers one
// message to the protocol's read loop the way the muxer would, so the message
// travels the whole readLoop -> recvLoop -> handler path rather than being
// handed straight to a handler.
func startPanicTestProtocol(
	t *testing.T,
	config ProtocolConfig,
) (*Protocol, chan error, func()) {
	t.Helper()

	serverConn, peerConn := net.Pipe()
	t.Cleanup(func() { _ = serverConn.Close() })
	t.Cleanup(func() { _ = peerConn.Close() })

	m := muxer.New(serverConn)
	m.Start()
	t.Cleanup(m.Stop)

	errorChan := make(chan error, 1)
	config.ErrorChan = errorChan
	config.Muxer = m
	config.Role = ProtocolRoleServer
	config.Name = "paniced"
	config.InitialState = panicTestInitialState
	if config.StateMap == nil {
		config.StateMap = StateMap{
			panicTestInitialState: {
				Agency: AgencyClient,
				Transitions: []StateTransition{{
					MsgType:  1,
					NewState: panicTestNextState,
				}},
			},
			panicTestNextState: {Agency: AgencyServer},
		}
	}
	if config.MessageFromCborFunc == nil {
		config.MessageFromCborFunc = func(
			msgType uint,
			data []byte,
		) (Message, error) {
			if msgType > math.MaxUint8 {
				return nil, fmt.Errorf(
					"message type out of range: %d",
					msgType,
				)
			}
			msg := &MessageBase{MessageType: uint8(msgType)}
			msg.SetCbor(data)
			return msg, nil
		}
	}
	if config.MessageHandlerFunc == nil {
		config.MessageHandlerFunc = func(Message) error { return nil }
	}

	p := New(config)
	p.Start()
	t.Cleanup(p.Stop)

	sendMessage := func() {
		// A one-element CBOR array holding message type 1, which is what
		// readLoop's type sniffing expects of any mini-protocol message.
		segment := muxer.NewSegment(0, []byte{0x81, 0x01}, false)
		require.NotNil(t, segment)
		select {
		case p.muxerRecvChan <- segment:
		case <-time.After(time.Second):
			t.Error("protocol never accepted the peer's message")
		}
	}
	return p, errorChan, sendMessage
}

// requireContainedPanic asserts that the consumer was handed a contained panic
// naming the goroutine it was raised on and the frame that raised it, rather
// than losing the process to it.
func requireContainedPanic(
	t *testing.T,
	errorChan <-chan error,
	where string,
	frame string,
) {
	t.Helper()
	select {
	case err := <-errorChan:
		require.Error(t, err)
		require.ErrorIs(t, err, ErrHandlerPanic)
		require.ErrorContains(t, err, panicValue)
		require.ErrorContains(t, err, "paniced: "+where)
		// Naming the responsible frame is what keeps a fault diagnosable
		// once the process no longer dumps a stack of its own, and it is
		// the only thing that tells a consumer whether the panic came from
		// its own callback or from this library's decoding.
		require.ErrorContains(t, err, frame)
	case <-time.After(5 * time.Second):
		t.Fatal(
			"no error reached the protocol's error channel; the panic was " +
				"neither contained nor reported",
		)
	}
}

// requireProtocolStopped asserts the connection was torn down, which is the
// disposition a protocol violation or decode error already produces.
func requireProtocolStopped(t *testing.T, p *Protocol) {
	t.Helper()
	select {
	case <-p.DoneChan():
	case <-time.After(5 * time.Second):
		t.Fatal(
			"protocol kept running after a contained panic; a handler that " +
				"did not complete must not leave the connection live",
		)
	}
}

func TestMessageHandlerPanicFailsConnection(t *testing.T) {
	p, errorChan, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		MessageHandlerFunc: func(Message) error {
			panic(panicValue)
		},
	})
	sendMessage()
	requireContainedPanic(
		t,
		errorChan,
		"receive loop",
		"TestMessageHandlerPanicFailsConnection",
	)
	requireProtocolStopped(t, p)
}

// TestMessageDecodePanicFailsConnection covers the read loop, where a peer's
// raw bytes reach a decoder: the message-type sniffing this package does
// itself and the mini-protocol's own message construction both run there,
// unwinding the read goroutine rather than any consumer frame.
func TestMessageDecodePanicFailsConnection(t *testing.T) {
	p, errorChan, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		MessageFromCborFunc: func(uint, []byte) (Message, error) {
			panic(panicValue)
		},
	})
	sendMessage()
	requireContainedPanic(
		t,
		errorChan,
		"read loop",
		"TestMessageDecodePanicFailsConnection",
	)
	requireProtocolStopped(t, p)
}

func TestStateTransitionMatchPanicFailsConnection(t *testing.T) {
	p, errorChan, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		StateMap: StateMap{
			panicTestInitialState: {
				Agency: AgencyClient,
				Transitions: []StateTransition{{
					MsgType: 1,
					MatchFunc: func(any, Message) bool {
						panic(panicValue)
					},
					NewState: panicTestNextState,
				}},
			},
			panicTestNextState: {Agency: AgencyServer},
		},
	})
	sendMessage()
	requireContainedPanic(
		t,
		errorChan,
		"state loop",
		"TestStateTransitionMatchPanicFailsConnection",
	)
	requireProtocolStopped(t, p)
}

// TestStateTimeoutPanicFailsConnection covers the state loop's dynamic
// timeout callback, which runs on the goroutine that owns every state
// transition and transition timer.
func TestStateTimeoutPanicFailsConnection(t *testing.T) {
	p, errorChan, _ := startPanicTestProtocol(t, ProtocolConfig{
		InitialStateTimeout: true,
		StateMap: StateMap{
			panicTestInitialState: {
				Agency:      AgencyClient,
				TimeoutFunc: func() time.Duration { panic(panicValue) },
			},
		},
	})
	requireContainedPanic(
		t,
		errorChan,
		"state loop",
		"TestStateTimeoutPanicFailsConnection",
	)
	requireProtocolStopped(t, p)
}

// TestMessageHandlingUnaffectedByContainment covers the case that must not
// change: an ordinary message is handled, nothing is reported, and the
// connection stays up.
func TestMessageHandlingUnaffectedByContainment(t *testing.T) {
	handled := make(chan Message, 1)
	p, errorChan, sendMessage := startPanicTestProtocol(t, ProtocolConfig{
		MessageHandlerFunc: func(msg Message) error {
			handled <- msg
			return nil
		},
	})
	sendMessage()

	select {
	case msg := <-handled:
		require.Equal(t, uint8(1), msg.Type())
	case <-time.After(5 * time.Second):
		t.Fatal("the message was never handled")
	}
	select {
	case err := <-errorChan:
		t.Fatalf("normal message handling reported an error: %v", err)
	case <-p.DoneChan():
		t.Fatal("normal message handling stopped the protocol")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestNonPanicFailuresUnchanged covers each failure that already had a defined
// error, confirming the consumer still sees exactly that error and not a
// contained panic.
func TestNonPanicFailuresUnchanged(t *testing.T) {
	for _, test := range []struct {
		name    string
		config  ProtocolConfig
		wantErr error
		wantMsg string
	}{
		{
			name: "handler error",
			config: ProtocolConfig{
				MessageHandlerFunc: func(Message) error {
					return errInjectedHandler
				},
			},
			wantErr: errInjectedHandler,
		},
		{
			name: "decode error",
			config: ProtocolConfig{
				MessageFromCborFunc: func(uint, []byte) (Message, error) {
					return nil, errInjectedHandler
				},
			},
			wantErr: errInjectedHandler,
		},
		{
			name: "message not allowed in current state",
			config: ProtocolConfig{
				StateMap: StateMap{
					panicTestInitialState: {Agency: AgencyClient},
				},
			},
			wantMsg: "not allowed in current protocol state",
		},
		{
			name: "no matching transition",
			config: ProtocolConfig{
				StateMap: StateMap{
					panicTestInitialState: {
						Agency: AgencyClient,
						Transitions: []StateTransition{{
							MsgType:   1,
							MatchFunc: func(any, Message) bool { return false },
							NewState:  panicTestNextState,
						}},
					},
					panicTestNextState: {Agency: AgencyServer},
				},
			},
			wantMsg: "not allowed in current protocol state",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, errorChan, sendMessage := startPanicTestProtocol(
				t,
				test.config,
			)
			sendMessage()
			select {
			case err := <-errorChan:
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrHandlerPanic)
				if test.wantErr != nil {
					require.ErrorIs(t, err, test.wantErr)
				}
				if test.wantMsg != "" {
					require.ErrorContains(t, err, test.wantMsg)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the expected error was never reported")
			}
			requireProtocolStopped(t, p)
		})
	}
}

// TestRecoverLoopReportsAndStops covers the backstop on its own, for the send
// loop, which has no peer-driven path a test can make panic: it muxes messages
// that were already encoded by the caller's goroutine. The containment is
// still worth having there, because an escaping panic kills the process rather
// than just that loop.
func TestRecoverLoopReportsAndStops(t *testing.T) {
	errorChan := make(chan error, 1)
	p := New(ProtocolConfig{
		ErrorChan:    errorChan,
		Name:         "paniced",
		InitialState: panicTestInitialState,
		StateMap: StateMap{
			panicTestInitialState: {Agency: AgencyClient},
		},
	})

	func() {
		defer p.recoverLoop("send loop")
		panic(panicValue)
	}()

	requireContainedPanic(
		t,
		errorChan,
		"send loop",
		"TestRecoverLoopReportsAndStops",
	)
	require.True(t, p.IsStopping())
}

// TestRecoverLoopIgnoresNormalReturn confirms the backstop reports nothing
// when the guarded function simply returns.
func TestRecoverLoopIgnoresNormalReturn(t *testing.T) {
	errorChan := make(chan error, 1)
	p := New(ProtocolConfig{
		ErrorChan:    errorChan,
		Name:         "paniced",
		InitialState: panicTestInitialState,
	})

	func() { defer p.recoverLoop("send loop") }()

	select {
	case err := <-errorChan:
		t.Fatalf("a normal return was reported as a panic: %v", err)
	default:
	}
	require.False(t, p.IsStopping())
}
