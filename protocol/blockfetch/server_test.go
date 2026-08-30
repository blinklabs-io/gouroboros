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

package blockfetch

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServerStateMapUsesInstanceTimeouts(t *testing.T) {
	cfg := &Config{
		BatchStartTimeout: 13 * time.Second,
		BlockTimeout:      17 * time.Second,
	}

	got := serverStateMap(cfg)
	require.Equal(t, 13*time.Second, got[StateBusy].Timeout)
	require.Equal(t, 17*time.Second, got[StateStreaming].Timeout)
	require.Equal(t, BusyTimeout, StateMap[StateBusy].Timeout)
	require.Equal(t, StreamingTimeout, StateMap[StateStreaming].Timeout)
}

func TestServerStateMapsAreIndependent(t *testing.T) {
	first := serverStateMap(&Config{BlockTimeout: 2 * time.Second})
	second := serverStateMap(&Config{BlockTimeout: 3 * time.Second})

	require.Equal(t, 2*time.Second, first[StateStreaming].Timeout)
	require.Equal(t, 3*time.Second, second[StateStreaming].Timeout)
}
