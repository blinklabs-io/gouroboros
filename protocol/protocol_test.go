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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
				resultChan <- p.enqueueMessage(msg, nil)
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
