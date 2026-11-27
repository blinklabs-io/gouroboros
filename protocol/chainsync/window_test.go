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

package chainsync

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestWindowBlockNumberMatchesDecodedBlock(t *testing.T) {
	t.Parallel()
	block := hexDecode(string(readFile(
		"testdata/shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
	)))
	decoded, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeShelley,
		block,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	want := decoded.BlockNumber()

	got, ok := blockBlockNumber(ledger.BlockTypeShelley, block)
	require.True(t, ok)
	require.Equal(t, want, got)

	wrapped, err := NewWrappedHeader(ledger.BlockHeaderTypeShelley, 0, block)
	require.NoError(t, err)
	got, ok = headerBlockNumber(ledger.BlockTypeShelley, wrapped.HeaderCbor())
	require.True(t, ok)
	require.Equal(t, want, got)
}

func TestWindowBlockNumberUnmeasured(t *testing.T) {
	t.Parallel()
	byron := hexDecode(string(readFile(
		"testdata/byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
	)))
	_, ok := blockBlockNumber(ledger.BlockTypeByronMain, byron)
	require.False(t, ok, "Byron block")

	enc := func(v any) []byte {
		data, err := cbor.Encode(v)
		require.NoError(t, err)
		return data
	}
	for name, data := range map[string][]byte{
		"empty":          nil,
		"not an array":   enc(uint64(7)),
		"body not array": enc([]any{uint64(7), []byte{}}),
		"number a bytes": enc([]any{[]any{[]byte{1}}, []byte{}}),
		"truncated":      enc([]any{[]any{uint64(1) << 40}})[:4],
	} {
		_, ok := headerBlockNumber(ledger.BlockTypeConway, data)
		require.False(t, ok, name)
	}
	for _, n := range []uint64{0, 23, 24, 255, 256, 65535, 65536, 1 << 32, 1<<64 - 1} {
		got, ok := headerBlockNumber(
			ledger.BlockTypeConway,
			enc([]any{[]any{n, uint64(1)}, []byte{}}),
		)
		require.True(t, ok)
		require.Equal(t, n, got)
	}
}
