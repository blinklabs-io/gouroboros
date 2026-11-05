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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

type markerCapability interface{ Marker() string }

// markedAdapter is a caller's adapter that adds one capability over the
// state it wraps.
type markedAdapter struct {
	common.LedgerState
}

func (markedAdapter) Marker() string { return "adapter" }

func (a markedAdapter) UnwrapLedgerState() common.LedgerState {
	return a.LedgerState
}

// findMarker walks one layer at a time, as a package outside gouroboros must,
// and stops at the first state that is not an unwrapper.
func findMarker(ls common.LedgerState) (markerCapability, bool) {
	for ls != nil {
		if m, ok := ls.(markerCapability); ok {
			return m, true
		}
		u, ok := ls.(common.LedgerStateUnwrapper)
		if !ok {
			return nil, false
		}
		ls = u.UnwrapLedgerState()
	}
	return nil, false
}

// TestScopedStateUnwrapsOneLayer checks that a capability held by an adapter
// beneath the transaction-scoped state is still found by a caller's own
// layer-by-layer walk, and that the walk lands on that adapter rather than
// skipping to the innermost state.
func TestScopedStateUnwrapsOneLayer(t *testing.T) {
	adapter := markedAdapter{mockledger.NewLedgerStateBuilder().Build()}
	scoped := common.NewTransactionScopedLedgerState(adapter)
	require.True(t, common.IsTransactionScopedLedgerState(scoped))

	unwrapper, ok := scoped.(common.LedgerStateUnwrapper)
	require.True(t, ok)
	require.Equal(t, common.LedgerState(adapter), unwrapper.UnwrapLedgerState())

	m, ok := findMarker(scoped)
	require.True(t, ok)
	require.Equal(t, "adapter", m.Marker())
}
