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

package shelley_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func nativeScriptThresholdCbor(t *testing.T, threshold int64) []byte {
	t.Helper()
	raw, err := cbor.Encode(common.NativeScriptNofK{
		Type:    3,
		N:       threshold,
		Scripts: []common.NativeScript{},
	})
	require.NoError(t, err)
	return raw
}

func TestShelleyNativeScriptNofKMatchesSignedThresholdSemantics(t *testing.T) {
	for _, tc := range []struct {
		threshold int64
		want      bool
	}{
		{threshold: -1, want: true},
		{threshold: 0, want: true},
		{threshold: 1, want: false},
	} {
		var script common.NativeScript
		require.NoError(t, script.UnmarshalCBOR(nativeScriptThresholdCbor(t, tc.threshold)))
		require.NoError(t, common.ValidatePreAllegraNativeScripts([]common.NativeScript{script}))
		require.Equal(t, tc.want, script.Evaluate(0, 0, 0, nil))
	}
}

func TestShelleyTransactionAndBlockAcceptSignedNativeScriptNofK(
	t *testing.T,
) {
	negative := nativeScriptThresholdCbor(t, -1)
	var negativeScript common.NativeScript
	require.NoError(t, negativeScript.UnmarshalCBOR(negative))
	allWithNegativeChild, err := cbor.Encode(common.NativeScriptAll{
		Type:    1,
		Scripts: []common.NativeScript{negativeScript},
	})
	require.NoError(t, err)
	var nestedScript common.NativeScript
	require.NoError(t, nestedScript.UnmarshalCBOR(allWithNegativeChild))
	require.True(t, nestedScript.Evaluate(0, 0, 0, nil))

	scripts := [][]byte{
		negative,
		nativeScriptThresholdCbor(t, 0),
		nativeScriptThresholdCbor(t, 1),
		allWithNegativeChild,
	}
	for _, scriptCbor := range scripts {
		witnessSet := map[uint]any{1: []cbor.RawMessage{scriptCbor}}
		transactionCbor, err := cbor.Encode([]any{
			map[uint]any{3: uint64(1)}, witnessSet, nil,
		})
		require.NoError(t, err)
		var transaction shelley.ShelleyTransaction
		require.NoError(t, transaction.UnmarshalCBOR(transactionCbor))

		body := map[uint]any{3: uint64(1)}
		blockCbor, err := cbor.Encode([]any{
			nil,
			[]any{body},
			[]any{witnessSet},
			map[uint]any{},
		})
		require.NoError(t, err)
		var block shelley.ShelleyBlock
		require.NoError(t, block.UnmarshalCBOR(blockCbor))
	}
}

func TestShelleyTransactionAndBlockRejectPostShelleyConstructors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		script any
	}{
		{
			name:   "invalid before at slot zero",
			script: common.NativeScriptInvalidBefore{Type: 4, Slot: 0},
		},
		{
			name:   "invalid hereafter matching ttl",
			script: common.NativeScriptInvalidHereafter{Type: 5, Slot: 1000},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scriptCbor, err := cbor.Encode(tc.script)
			require.NoError(t, err)
			var script common.NativeScript
			require.NoError(t, script.UnmarshalCBOR(scriptCbor))
			require.True(t, script.Evaluate(1000, 0, 500, nil))
			allWithChild, err := cbor.Encode(common.NativeScriptAll{
				Type:    1,
				Scripts: []common.NativeScript{script},
			})
			require.NoError(t, err)

			for _, rawScript := range [][]byte{scriptCbor, allWithChild} {
				witnessSet := map[uint]any{1: []cbor.RawMessage{rawScript}}
				transactionCbor, err := cbor.Encode([]any{
					map[uint]any{3: uint64(500)}, witnessSet, nil,
				})
				require.NoError(t, err)
				var transaction shelley.ShelleyTransaction
				require.Error(t, transaction.UnmarshalCBOR(transactionCbor))

				blockCbor, err := cbor.Encode([]any{
					nil,
					[]any{map[uint]any{3: uint64(500)}},
					[]any{witnessSet},
					map[uint]any{},
				})
				require.NoError(t, err)
				var block shelley.ShelleyBlock
				require.Error(t, block.UnmarshalCBOR(blockCbor))
			}
		})
	}
}
