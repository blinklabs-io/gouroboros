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

// TestDefaultTimeoutIsReferenceShortWait checks the default bound on
// non-blocking ID and object requests against the reference's
// timeLimitsObjectDiffusion, which gives those states shortWait (10s).
func TestDefaultTimeoutIsReferenceShortWait(t *testing.T) {
	t.Parallel()
	require.Equal(t, 10*time.Second, DefaultTimeout)
	cfg := NewConfig()
	require.Equal(t, 10*time.Second, cfg.Timeout)
}
