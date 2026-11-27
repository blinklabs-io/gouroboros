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

package conway_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/cbor"
	internaltest "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
)

// maxRejectMapAllocBytes is far below the megabytes needed to hold every pair of
// an oversized map.
const maxRejectMapAllocBytes = 256 * 1024

func TestConwayProtocolParameterUpdateRejectsUnknownKeysBeforeAllocating(t *testing.T) {
	// Not t.Parallel: heap allocation is measured with a process-wide counter.
	data := internaltest.MapOfUnknownKeys(internaltest.OversizedMapPairs)
	var err error
	allocated := internaltest.AllocatedBytes(func() {
		err = (&conway.ConwayProtocolParameterUpdate{}).UnmarshalCBOR(data)
	})
	require.Error(t, err)
	assert.Less(
		t,
		allocated,
		uint64(maxRejectMapAllocBytes),
		"conway parameter update allocated before checking its keys",
	)
}

func TestConwayTransactionPparamUpdateRejectsInvalidProposalsBeforeAllocating(t *testing.T) {
	// Not t.Parallel: heap allocation is measured with a process-wide counter.
	data := internaltest.ParamUpdateProposals(internaltest.OversizedProposalPairs)
	var update conway.ConwayTransactionPparamUpdate
	var err error
	allocated := internaltest.AllocatedBytes(func() {
		_, err = cbor.Decode(data, &update)
	})
	require.Error(t, err)
	assert.Less(
		t,
		allocated,
		uint64(maxRejectMapAllocBytes),
		"conway update proposals allocated before checking their values",
	)
}
