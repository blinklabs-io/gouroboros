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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestExtractTransactionOffsetsReturnsInvalidTransactions(t *testing.T) {
	block := []any{
		[]any{}, // header
		[]any{map[uint64]any{}, map[uint64]any{}},
		[]any{map[uint64]any{}, map[uint64]any{}},
		map[uint64]any{}, // metadata
		[]uint{1},        // invalid_transactions
	}
	blockCbor, err := cbor.Encode(block)
	require.NoError(t, err)

	offsets, err := common.ExtractTransactionOffsets(blockCbor)
	require.NoError(t, err)
	require.Len(t, offsets.Transactions, 2)
	require.Equal(t, []uint{1}, offsets.InvalidTransactions)
}

func TestExtractTransactionOffsetsPreservesDuplicateInvalidTransactions(t *testing.T) {
	block := []any{
		[]any{}, // header
		[]any{map[uint64]any{}, map[uint64]any{}},
		[]any{map[uint64]any{}, map[uint64]any{}},
		map[uint64]any{},
		cbor.NewSetType([]uint{1, 1}, false),
	}
	blockCbor, err := cbor.Encode(block)
	require.NoError(t, err)

	offsets, err := common.ExtractTransactionOffsets(blockCbor)
	require.NoError(t, err)
	require.Equal(t, []uint{1, 1}, offsets.InvalidTransactions)
}
