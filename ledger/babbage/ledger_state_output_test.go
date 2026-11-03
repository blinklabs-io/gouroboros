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

package babbage_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// junkAddressOutput returns a legacy output whose mainnet enterprise address
// is followed by 21 bytes past its payload, as some pre-Babbage outputs on
// mainnet carry, and the same address in the map output form.
func junkAddressOutput(t *testing.T) (payload, legacy, mapForm []byte) {
	t.Helper()
	header := byte(common.AddressTypeKeyNone<<4) | byte(common.AddressNetworkMainnet)
	payload = append([]byte{header}, bytes.Repeat([]byte{0x11}, common.AddressHashSize)...)
	addr := append(append([]byte{}, payload...), bytes.Repeat([]byte{0xee}, 21)...)
	legacy, err := cbor.Encode([]any{addr, uint64(1_000_000)})
	require.NoError(t, err)
	mapForm, err = cbor.Encode(map[uint]any{0: addr, 1: uint64(1_000_000)})
	require.NoError(t, err)
	return payload, legacy, mapForm
}

func TestNewBabbageTransactionOutputFromCborLenientCropsAddress(t *testing.T) {
	payload, legacy, _ := junkAddressOutput(t)
	_, err := babbage.NewBabbageTransactionOutputFromCbor(legacy)
	require.ErrorContains(t, err, "21 unexpected trailing byte(s)",
		"a transaction output from Babbage on must keep rejecting it")

	out, err := babbage.NewBabbageTransactionOutputFromCborLenient(legacy)
	require.NoError(t, err)
	got, err := out.Address().Bytes()
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	assert.Len(t, out.Address().TrailingBytes(), 21)
	assert.Equal(t, uint64(1_000_000), out.Amount().Uint64())
	assert.Equal(t, legacy, out.Cbor())
}

func TestNewBabbageTransactionOutputFromCborLenientRejectsMapForm(t *testing.T) {
	_, _, mapForm := junkAddressOutput(t)
	_, err := babbage.NewBabbageTransactionOutputFromCborLenient(mapForm)
	require.ErrorContains(t, err, "21 unexpected trailing byte(s)")
}
