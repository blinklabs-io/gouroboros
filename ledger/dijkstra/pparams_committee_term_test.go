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

package dijkstra_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommitteeMaxTermLengthNilReceiver pins the explicit nil-safe override.
// Without it the promoted ConwayProtocolParameters method is reached through
// the embedded field of a nil *DijkstraProtocolParameters, which dereferences
// the nil outer pointer and panics.
func TestCommitteeMaxTermLengthNilReceiver(t *testing.T) {
	t.Parallel()
	var pp *dijkstra.DijkstraProtocolParameters
	var provider common.CommitteeMaxTermLengthProvider = pp
	require.NotPanics(t, func() {
		limit, ok := provider.CommitteeMaxTermLength()
		assert.False(t, ok)
		assert.Zero(t, limit)
	})
}

func TestCommitteeMaxTermLengthReportsConwayLimit(t *testing.T) {
	t.Parallel()
	pp := &dijkstra.DijkstraProtocolParameters{}
	pp.CommitteeTermLimit = 73
	limit, ok := pp.CommitteeMaxTermLength()
	assert.True(t, ok)
	assert.Equal(t, uint64(73), limit)
}
