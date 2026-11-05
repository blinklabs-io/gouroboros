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

package localstatequery_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A UTxO query answers from the ledger state, which still holds outputs
// created before Babbage whose address carries bytes past its payload. One
// such output must not fail the whole result.
func TestUTxOResultsAcceptPreBabbageAddressTrailingBytes(t *testing.T) {
	header := byte(common.AddressTypeKeyNone<<4) | byte(common.AddressNetworkMainnet)
	payload := append([]byte{header}, bytes.Repeat([]byte{0x11}, common.AddressHashSize)...)
	junk := append(append([]byte{}, payload...), bytes.Repeat([]byte{0xee}, 21)...)
	clean := append([]byte{header}, bytes.Repeat([]byte{0x22}, common.AddressHashSize)...)
	junkRef := []any{bytes.Repeat([]byte{0xaa}, 32), 0}
	cleanRef := []any{bytes.Repeat([]byte{0xbb}, 32), 1}
	result, err := cbor.Encode([]any{map[*[]any][]any{
		&junkRef:  {junk, uint64(1_000_000)},
		&cleanRef: {clean, uint64(2_000_000)},
	}})
	require.NoError(t, err)

	check := func(t *testing.T, got map[localstatequery.UtxoId]common.TransactionOutput) {
		t.Helper()
		require.Len(t, got, 2)
		for id, out := range got {
			addr, err := out.Address().Bytes()
			require.NoError(t, err)
			switch id.Idx {
			case 0:
				assert.Equal(t, payload, addr)
				assert.Len(t, out.Address().TrailingBytes(), 21)
			case 1:
				assert.Equal(t, clean, addr)
			}
		}
	}

	t.Run("whole", func(t *testing.T) {
		var r localstatequery.UTxOWholeResult
		_, err := cbor.Decode(result, &r)
		require.NoError(t, err)
		got := map[localstatequery.UtxoId]common.TransactionOutput{}
		for k, v := range r.Results {
			got[k] = v
		}
		check(t, got)
	})
	t.Run("by tx in", func(t *testing.T) {
		var r localstatequery.UTxOByTxInResult
		_, err := cbor.Decode(result, &r)
		require.NoError(t, err)
		got := map[localstatequery.UtxoId]common.TransactionOutput{}
		for k, v := range r.Results {
			got[k] = v
		}
		check(t, got)
	})
}

func TestUTxOResultsRejectDuplicateRefs(t *testing.T) {
	header := byte(common.AddressTypeKeyNone<<4) | byte(common.AddressNetworkMainnet)
	addr := append([]byte{header}, bytes.Repeat([]byte{0x11}, common.AddressHashSize)...)
	out, err := cbor.Encode([]any{addr, uint64(1)})
	require.NoError(t, err)
	key, err := cbor.Encode([]any{bytes.Repeat([]byte{0xaa}, 32), 0})
	require.NoError(t, err)
	// A one-element array holding a two-entry map whose keys repeat.
	data := []byte{0x81, 0xa2}
	for range 2 {
		data = append(data, key...)
		data = append(data, out...)
	}
	var r localstatequery.UTxOWholeResult
	_, err = cbor.Decode(data, &r)
	require.Error(t, err)
}
