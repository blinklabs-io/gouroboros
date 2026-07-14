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

package localtxsubmission

import "testing"

// The LocalTxSubmission client has agency in the Idle state and may terminate
// the protocol by sending MsgDone (per the mini-protocol spec). Without an
// Idle->Done transition, gouroboros rejects cardano-cli's clean shutdown with
// "message *localtxsubmission.MsgDone not allowed in current protocol state
// Idle" and tears down the whole node-to-client connection (dingo issue #2788).
func TestStateMapIdleAllowsDone(t *testing.T) {
	entry, ok := StateMap[stateIdle]
	if !ok {
		t.Fatal("StateMap has no entry for the Idle state")
	}
	found := false
	for _, transition := range entry.Transitions {
		if transition.MsgType != MessageTypeDone {
			continue
		}
		found = true
		if transition.NewState != stateDone {
			t.Fatalf(
				"Idle MsgDone transitions to %s, want Done",
				transition.NewState,
			)
		}
	}
	if !found {
		t.Fatal(
			"Idle state does not allow MsgDone; cardano-cli cannot cleanly " +
				"close the protocol (issue #2788)",
		)
	}
}
