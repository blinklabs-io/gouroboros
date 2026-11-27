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

	"github.com/stretchr/testify/require"
)

// TestBlockingIDRequestTimeoutFollowsReference checks the limit the Server
// puts on the peer's answer to a blocking request for IDs. In V1, which is
// tx-submission, dmq-node's outbound waits until it has an ID and has no
// empty answer, and the reference limit is waitForever
// (DMQ/Protocol/SigSubmission/Codec.hs), so a timer there drops a peer that
// merely has nothing new. In V2 the outbound answers with MsgReplyNoSigIds
// after 17 s and the reference limit is 20 s, so a limit stays.
// The engine arms a state's timer only while the peer has agency, and the
// Server installs these state maps unchanged.
func TestBlockingIDRequestTimeoutFollowsReference(t *testing.T) {
	require.Equal(
		t,
		time.Duration(0),
		stateMapV1[protocolStateMessageIdsBlock].Timeout,
		"V1 blocking request must be untimed",
	)
	v2 := stateMapV2[protocolStateMessageIdsBlock].Timeout
	require.Greater(t, v2, 17*time.Second, "V2 limit below the outbound's answer")
}
