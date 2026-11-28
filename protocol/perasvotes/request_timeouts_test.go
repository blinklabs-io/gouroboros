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

package perasvotes

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNonBlockingAndObjectRequestTimeouts checks the waits the reference
// times with shortWait (10 s) besides the blocking request: the client's wait
// for the reply to a non-blocking request for IDs and to a request for
// objects (ObjectDiffusion/Codec.hs timeLimitsObjectDiffusion). The server
// installs the same map and waits only in Init and Idle, where the reference
// waits forever, so neither is timed.
func TestNonBlockingAndObjectRequestTimeouts(t *testing.T) {
	stateMap := clientStateMap(normalizeConfig(nil))
	require.Equal(t, 10*time.Second, stateMap[StateObjectIDsNonBlocking].Timeout)
	require.Equal(t, 10*time.Second, stateMap[StateObjects].Timeout)
	require.Zero(t, stateMap[StateInit].Timeout)
	require.Zero(t, stateMap[StateIdle].Timeout)
}
