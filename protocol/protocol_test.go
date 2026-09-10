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
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

func TestInitialStateTimeoutOptIn(t *testing.T) {
	for _, test := range []struct {
		name        string
		enabled     bool
		timeout     time.Duration
		wantTimeout bool
	}{
		{"disabled by default", false, 20 * time.Millisecond, false},
		{"enabled explicitly", true, 20 * time.Millisecond, true},
		{"enabled with zero timeout", true, 0, false},
		{"enabled with negative timeout", true, -time.Second, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			serverConn, peerConn := net.Pipe()
			defer serverConn.Close()
			defer peerConn.Close()
			m := muxer.New(serverConn)
			m.Start()
			defer m.Stop()
			errorChan := make(chan error, 1)
			state := NewState(1, "Initial")
			p := New(ProtocolConfig{
				ErrorChan: errorChan,
				Muxer:     m,
				Role:      ProtocolRoleServer,
				StateMap: StateMap{state: {
					Agency:  AgencyClient,
					Timeout: test.timeout,
				}},
				InitialState:        state,
				InitialStateTimeout: test.enabled,
			})
			p.Start()
			defer p.Stop()

			if test.wantTimeout {
				select {
				case err := <-errorChan:
					require.NotNil(t, err)
					require.Contains(t, err.Error(), "timeout waiting on transition")
				case <-time.After(time.Second):
					t.Fatal("initial timeout was not enforced")
				}
				return
			}
			select {
			case err := <-errorChan:
				t.Fatalf("unexpected initial timeout: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
		})
	}
}

func TestSubsequentStateTimeoutRemainsActive(t *testing.T) {
	for _, initialStateTimeout := range []bool{false, true} {
		t.Run(fmt.Sprintf("initial timeout enabled=%t", initialStateTimeout), func(t *testing.T) {
			testSubsequentStateTimeoutRemainsActive(t, initialStateTimeout)
		})
	}
}

func testSubsequentStateTimeoutRemainsActive(t *testing.T, initialStateTimeout bool) {
	serverConn, peerConn := net.Pipe()
	defer serverConn.Close()
	defer peerConn.Close()
	m := muxer.New(serverConn)
	m.Start()
	defer m.Stop()
	errorChan := make(chan error, 1)
	initialState := NewState(1, "Initial")
	confirmState := NewState(2, "Confirm")
	p := New(ProtocolConfig{
		ErrorChan: errorChan,
		Muxer:     m,
		Role:      ProtocolRoleServer,
		StateMap: StateMap{
			initialState: {
				Agency: AgencyClient,
				Transitions: []StateTransition{{
					MsgType:  1,
					NewState: confirmState,
				}},
			},
			confirmState: {
				Agency:  AgencyServer,
				Timeout: 20 * time.Millisecond,
			},
		},
		InitialState:        initialState,
		InitialStateTimeout: initialStateTimeout,
	})
	p.Start()
	defer p.Stop()

	msg := &MessageBase{MessageType: 1}
	require.NoError(t, p.transitionState(msg))

	select {
	case err := <-errorChan:
		require.NotNil(t, err)
		require.Contains(t, err.Error(), "timeout waiting on transition")
	case <-time.After(time.Second):
		t.Fatal("subsequent state timeout was not enforced")
	}
}

func TestEnqueueMessageReturnsWhenFullQueueShutsDown(t *testing.T) {
	tests := []struct {
		name     string
		shutdown func(*Protocol)
	}{
		{
			name: "protocol stop",
			shutdown: func(p *Protocol) {
				close(p.stopChan)
			},
		},
		{
			name: "protocol done",
			shutdown: func(p *Protocol) {
				close(p.doneChan)
			},
		},
		{
			name: "muxer done",
			shutdown: func(p *Protocol) {
				close(p.muxerDoneChan)
			},
		},
		{
			name: "receive loop done",
			shutdown: func(p *Protocol) {
				close(p.recvDoneChan)
			},
		},
		{
			name: "send loop done",
			shutdown: func(p *Protocol) {
				close(p.sendDoneChan)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &Protocol{
				stopChan:      make(chan struct{}),
				doneChan:      make(chan struct{}),
				muxerDoneChan: make(chan bool),
				recvDoneChan:  make(chan struct{}),
				sendDoneChan:  make(chan struct{}),
				sendQueueChan: make(chan outboundMessage, 1),
			}
			p.sendQueueChan <- outboundMessage{}

			msg := &MessageBase{}
			msg.SetCbor([]byte{0x80})
			resultChan := make(chan error, 1)
			go func() {
				resultChan <- p.enqueueMessage(
					context.Background(),
					msg,
					nil,
				)
			}()

			require.Eventually(t, func() bool {
				p.pendingBytesMu.Lock()
				defer p.pendingBytesMu.Unlock()
				return p.pendingSendBytes == 1
			}, time.Second, time.Millisecond)

			test.shutdown(p)
			select {
			case err := <-resultChan:
				require.ErrorIs(t, err, ErrProtocolShuttingDown)
			case <-time.After(time.Second):
				t.Fatal("enqueueMessage remained blocked during shutdown")
			}

			p.pendingBytesMu.Lock()
			defer p.pendingBytesMu.Unlock()
			require.Zero(t, p.pendingSendBytes)
		})
	}
}

func TestEnqueueMessageKeepsEncodingOutOfCallerMessage(t *testing.T) {
	p := &Protocol{
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		muxerDoneChan: make(chan bool),
		recvDoneChan:  make(chan struct{}),
		sendDoneChan:  make(chan struct{}),
		sendQueueChan: make(chan outboundMessage, 1),
		config:        ProtocolConfig{Name: "test"},
	}
	msg := &MessageBase{MessageType: 1}

	require.NoError(t, p.SendMessage(msg))
	require.Nil(t, msg.Cbor())
	queued := <-p.sendQueueChan
	require.NotEmpty(t, queued.data)
	require.Same(t, msg, queued.message)
}

func TestSendMessageContextReturnsWhenFullQueueContextEnds(t *testing.T) {
	p := &Protocol{
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		muxerDoneChan: make(chan bool),
		recvDoneChan:  make(chan struct{}),
		sendDoneChan:  make(chan struct{}),
		sendQueueChan: make(chan outboundMessage, 1),
	}
	p.sendQueueChan <- outboundMessage{}

	msg := &MessageBase{}
	msg.SetCbor([]byte{0x80})
	ctx, cancel := context.WithCancel(context.Background())
	resultChan := make(chan error, 1)
	go func() {
		resultChan <- p.SendMessageContext(ctx, msg)
	}()

	require.Eventually(t, func() bool {
		p.pendingBytesMu.Lock()
		defer p.pendingBytesMu.Unlock()
		return p.pendingSendBytes == 1
	}, time.Second, time.Millisecond)
	cancel()

	select {
	case err := <-resultChan:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("SendMessageContext remained blocked after cancellation")
	}
	p.pendingBytesMu.Lock()
	defer p.pendingBytesMu.Unlock()
	require.Zero(t, p.pendingSendBytes)
}

func TestWaitForMessageDeliveryPrefersReportedResult(t *testing.T) {
	deliveryErr := errors.New("write failed")

	tests := []struct {
		name     string
		shutdown func(*Protocol)
	}{
		{
			name: "protocol stop",
			shutdown: func(p *Protocol) {
				close(p.stopChan)
			},
		},
		{
			name: "protocol done",
			shutdown: func(p *Protocol) {
				close(p.doneChan)
			},
		},
		{
			name: "muxer done",
			shutdown: func(p *Protocol) {
				close(p.muxerDoneChan)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for range 100 {
				deliveryChan := make(chan error, 1)
				deliveryChan <- deliveryErr
				p := &Protocol{
					stopChan:      make(chan struct{}),
					doneChan:      make(chan struct{}),
					muxerDoneChan: make(chan bool),
				}
				test.shutdown(p)

				require.ErrorIs(
					t,
					p.waitForMessageDelivery(deliveryChan),
					deliveryErr,
				)
			}
		})
	}
}

func TestWaitForMessageDeliveryReportsShutdownWithoutResult(t *testing.T) {
	p := &Protocol{
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		muxerDoneChan: make(chan bool),
	}
	close(p.stopChan)

	require.ErrorIs(
		t,
		p.waitForMessageDelivery(make(chan error, 1)),
		ErrProtocolShuttingDown,
	)
}

func TestIsDone(t *testing.T) {
	stateIdle := NewState(1, "Idle")
	stateDone := NewState(2, "Done")
	stateWorking := NewState(3, "Working")

	stateMap := StateMap{
		stateIdle: StateMapEntry{
			Agency: AgencyClient,
		},
		stateDone: StateMapEntry{
			Agency: AgencyNone,
		},
		stateWorking: StateMapEntry{
			Agency: AgencyServer,
		},
	}

	t.Run("returns false when protocol is active and in working state", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateWorking,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		require.False(t, p.IsDone(), "IsDone() should return false when in a non-terminal working state")
	})

	t.Run("returns false when in initial state (for client Stop behavior)", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateIdle,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		// IsDone should return false for initial state - client Stop() should still send Done
		require.False(t, p.IsDone(), "IsDone() should return false when in initial state (client needs to send Done)")
	})

	t.Run("returns true when done channel is closed", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateWorking,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		close(doneChan)

		require.True(t, p.IsDone(), "IsDone() should return true when doneChan is closed")
	})

	t.Run("returns true when in AgencyNone state (Done state)", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateDone,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		require.True(t, p.IsDone(), "IsDone() should return true when in AgencyNone (Done) state")
	})

	t.Run("returns true consistently after done channel closed", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateWorking,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		close(doneChan)

		for i := range 3 {
			require.True(t, p.IsDone(), "IsDone() call %d should return true", i+1)
		}
	})
}

func TestIsInTerminalOrIdleState(t *testing.T) {
	stateIdle := NewState(1, "Idle")
	stateDone := NewState(2, "Done")
	stateWorking := NewState(3, "Working")

	stateMap := StateMap{
		stateIdle: StateMapEntry{
			Agency: AgencyClient,
		},
		stateDone: StateMapEntry{
			Agency: AgencyNone,
		},
		stateWorking: StateMapEntry{
			Agency: AgencyServer,
		},
	}

	t.Run("returns false when protocol is active and in working state", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateWorking,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		require.False(t, p.IsInTerminalOrIdleState(), "IsInTerminalOrIdleState() should return false when in a non-terminal working state")
	})

	t.Run("returns true when done channel is closed", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateWorking,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		close(doneChan)

		require.True(t, p.IsInTerminalOrIdleState(), "IsInTerminalOrIdleState() should return true when doneChan is closed")
	})

	t.Run("returns true when in AgencyNone state (Done state)", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateDone,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		require.True(t, p.IsInTerminalOrIdleState(), "IsInTerminalOrIdleState() should return true when in AgencyNone (Done) state")
	})

	t.Run("returns true when in initial state (no messages exchanged)", func(t *testing.T) {
		doneChan := make(chan struct{})
		p := &Protocol{
			doneChan:     doneChan,
			currentState: stateIdle,
			config: ProtocolConfig{
				InitialState: stateIdle,
				StateMap:     stateMap,
			},
		}

		require.True(t, p.IsInTerminalOrIdleState(), "IsInTerminalOrIdleState() should return true when in initial state (no messages exchanged)")
	})
}
