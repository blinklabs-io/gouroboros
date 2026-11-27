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

package messagesubmission

import (
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// TestWaitsTheReferenceTimesAreTimed checks every state in which dmq-node
// times the side that waits for a reply: a non-blocking request for IDs and
// a request for messages have shortWait, 10 s, in V1 and V2
// (DMQ/Protocol/SigSubmission/Codec.hs timeLimitsSigSubmission,
// SigSubmissionV2/Codec.hs timeLimitsSigSubmissionV2). A peer that never
// answers such a request must not hold the requester forever.
func TestWaitsTheReferenceTimesAreTimed(t *testing.T) {
	const shortWait = 10 * time.Second
	for version, stateMap := range map[string]protocol.StateMap{
		"V1": stateMapV1,
		"V2": stateMapV2,
	} {
		for _, state := range []protocol.State{
			protocolStateMessageIdsNonBlk,
			protocolStateMessages,
		} {
			timeout := stateMap[state].Timeout
			require.Positive(t, timeout, "%s %s is untimed", version, state)
			require.GreaterOrEqual(
				t,
				timeout,
				shortWait,
				"%s %s times the peer out before the reference does",
				version,
				state,
			)
		}
	}
	require.Equal(t, shortWait, NewConfig().MessageIdsNonblockingTimeout)
	require.Equal(t, shortWait, normalizeConfig(&Config{}).MessageIdsNonblockingTimeout)
}
