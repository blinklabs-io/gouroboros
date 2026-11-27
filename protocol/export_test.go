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

// ClassificationConfigForTest returns the state map and initial state the
// protocol was built with, for tests in package protocol_test.
func (p *Protocol) ClassificationConfigForTest() (StateMap, State) {
	return p.config.StateMap, p.config.InitialState
}

// SetCurrentStateForTest moves the protocol to s without a transition.
func (p *Protocol) SetCurrentStateForTest(s State) {
	p.currentStateMu.Lock()
	p.currentState = s
	p.currentStateMu.Unlock()
}

// SetPendingPipelinedRequestsForTest records n pipelined requests written to
// the wire whose deferred transitions have not yet applied.
func (p *Protocol) SetPendingPipelinedRequestsForTest(n int) {
	p.pendingBytesMu.Lock()
	p.pendingPipelinedRequests = n
	p.pendingBytesMu.Unlock()
}
