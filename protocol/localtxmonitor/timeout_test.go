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

package localtxmonitor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDefaultConfigHasNoTimeouts pins the specification's "No timeouts" for
// this node-to-client protocol: the reference defines no time limits for it,
// so a default timer would tear down the connection to a node that is
// working as intended. A caller may still opt in with the timeout options.
func TestDefaultConfigHasNoTimeouts(t *testing.T) {
	t.Parallel()
	cfg := NewConfig()
	require.Zero(t, cfg.AcquireTimeout)
	require.Zero(t, cfg.QueryTimeout)
	for state, entry := range StateMap {
		require.Zero(t, entry.Timeout, "state %s", state)
		require.Nil(t, entry.TimeoutFunc, "state %s", state)
	}
}
