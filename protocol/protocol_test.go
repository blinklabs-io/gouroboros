// Copyright 2025 Blink Labs Software
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
	"testing"
)

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

		if p.IsDone() {
			t.Error("IsDone() should return false when in a non-terminal working state")
		}
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
		if p.IsDone() {
			t.Error("IsDone() should return false when in initial state (client needs to send Done)")
		}
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

		if !p.IsDone() {
			t.Error("IsDone() should return true when doneChan is closed")
		}
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

		if !p.IsDone() {
			t.Error("IsDone() should return true when in AgencyNone (Done) state")
		}
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

		for i := 0; i < 3; i++ {
			if !p.IsDone() {
				t.Errorf("IsDone() call %d should return true", i+1)
			}
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

		if p.isInTerminalOrIdleState() {
			t.Error("isInTerminalOrIdleState() should return false when in a non-terminal working state")
		}
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

		if !p.isInTerminalOrIdleState() {
			t.Error("isInTerminalOrIdleState() should return true when doneChan is closed")
		}
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

		if !p.isInTerminalOrIdleState() {
			t.Error("isInTerminalOrIdleState() should return true when in AgencyNone (Done) state")
		}
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

		if !p.isInTerminalOrIdleState() {
			t.Error("isInTerminalOrIdleState() should return true when in initial state (no messages exchanged)")
		}
	})
}
