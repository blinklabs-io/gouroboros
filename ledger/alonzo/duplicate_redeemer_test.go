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

package alonzo_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAlonzoRedeemersDuplicateKeyLastValueWins covers the list-form witness
// encoding used before Conway. Cardano ledger decodes this list into a Map,
// whose last value wins when a key occurs more than once.
func TestAlonzoRedeemersDuplicateKeyLastValueWins(t *testing.T) {
	// [(mint, 0, 1), (mint, 0, 2), (mint, 0, 3)]
	const raw = "83840100018201018401000282010184010003820101"
	var redeemers alonzo.AlonzoRedeemers
	require.NoError(t, redeemers.UnmarshalCBOR(mustDecodeHex(t, raw)))

	value := redeemers.Value(0, common.RedeemerTagMint)
	assert.Equal(t, "03", hex.EncodeToString(value.Data.Cbor()))

	var entries []common.RedeemerValue
	for key, entry := range redeemers.Iter() {
		assert.Equal(t, common.RedeemerTagMint, key.Tag)
		assert.Equal(t, uint32(0), key.Index)
		entries = append(entries, entry)
	}
	assert.Len(t, entries, 1)
	if len(entries) == 1 {
		assert.Equal(t, "03", hex.EncodeToString(entries[0].Data.Cbor()))
	}
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()
	data, err := hex.DecodeString(value)
	require.NoError(t, err)
	return data
}
