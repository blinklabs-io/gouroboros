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

package localstatequery

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestServerAcceptsReleaseWithoutCallback checks that MsgRelease, which the
// protocol always allows from Acquired, is accepted by a server with no
// ReleaseFunc. A client releases every state it is done with, so refusing it
// would tear down the connection of every well-behaved client.
func TestServerAcceptsReleaseWithoutCallback(t *testing.T) {
	t.Parallel()
	s := newServerWithVersion(0, nil)
	require.Nil(t, s.config.ReleaseFunc)
	require.NoError(t, s.handleRelease())
}

// TestServerCallsReleaseFunc checks that a configured ReleaseFunc still runs.
func TestServerCallsReleaseFunc(t *testing.T) {
	t.Parallel()
	s := newServerWithVersion(0, nil)
	called := false
	s.config.ReleaseFunc = func(CallbackContext) error {
		called = true
		return nil
	}
	require.NoError(t, s.handleRelease())
	require.True(t, called)
}
