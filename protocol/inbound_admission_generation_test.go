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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInboundAdmissionGenerationDrift checks how admission treats a state
// generation that moved between a message's snapshot and its admission, for a
// protocol that does not pipeline. A drift of one is the previous message's
// own transition only when the peer was already talking in the snapshot's
// state; after a snapshot in a state where we held agency, the message started
// arriving before the peer had agency and is refused.
func TestInboundAdmissionGenerationDrift(t *testing.T) {
	idle := NewState(1, "Idle")
	busy := NewState(2, "Busy")
	streaming := NewState(3, "Streaming")
	stateMap := StateMap{
		idle:      {Agency: AgencyClient},
		busy:      {Agency: AgencyServer},
		streaming: {Agency: AgencyServer},
	}
	tests := []struct {
		name              string
		snapshotState     State
		snapshotGen       uint64
		currentState      State
		currentGeneration uint64
		want              inboundAdmissionResult
	}{
		{
			name:              "same generation",
			snapshotState:     busy,
			snapshotGen:       2,
			currentState:      busy,
			currentGeneration: 2,
			want:              inboundAdmissionGranted,
		},
		{
			name:              "one step after a peer-agency snapshot",
			snapshotState:     busy,
			snapshotGen:       1,
			currentState:      streaming,
			currentGeneration: 2,
			want:              inboundAdmissionGranted,
		},
		{
			name:              "one step after a local-agency snapshot",
			snapshotState:     idle,
			snapshotGen:       1,
			currentState:      busy,
			currentGeneration: 2,
			want:              inboundAdmissionDenied,
		},
		{
			name:              "two steps after a peer-agency snapshot",
			snapshotState:     busy,
			snapshotGen:       1,
			currentState:      streaming,
			currentGeneration: 3,
			want:              inboundAdmissionDenied,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &Protocol{
				config: ProtocolConfig{
					Name:     "test",
					Role:     ProtocolRoleClient,
					StateMap: stateMap,
				},
				currentState:           test.currentState,
				currentStateGeneration: test.currentGeneration,
			}
			state, admission, result := p.reserveInboundAdmission(
				1,
				test.snapshotState,
				test.snapshotGen,
				false,
			)
			require.Equal(t, test.want, result)
			require.Equal(t, test.currentState, state)
			require.False(t, admission.reservedPipelinedReply)
			require.Equal(
				t,
				test.want == inboundAdmissionGranted,
				p.peerAgencyReserved,
				"only an admitted message reserves peer agency",
			)
		})
	}
}
