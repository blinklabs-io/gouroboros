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
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// nestedAllScript returns an "all" script nested depth levels deep, each level
// holding exactly one child, ending in an invalid-before leaf. Each level
// costs three bytes and two CBOR nesting levels.
func nestedAllScript(depth int, indefinite bool) []byte {
	var out []byte
	for range depth {
		if indefinite {
			out = append(out, 0x82, 0x01, 0x9f)
		} else {
			out = append(out, 0x82, 0x01, 0x81)
		}
	}
	out = append(out, 0x82, 0x04, 0x00)
	if indefinite {
		for range depth {
			out = append(out, 0xff)
		}
	}
	return out
}

func decodeNativeScriptTime(t *testing.T, data []byte) time.Duration {
	t.Helper()
	best := time.Duration(1<<63 - 1)
	for range 5 {
		var script common.NativeScript
		start := time.Now()
		require.NoError(t, script.UnmarshalCBOR(data))
		if d := time.Since(start); d < best {
			best = d
		}
	}
	return best
}

// Decoding must visit each byte a constant number of times. Quadratic decode
// grows 16x when the depth grows 4x; linear decode grows about 4x. The bound
// sits between them with a wide margin, and the best of several runs on each
// side filters scheduler noise.
func TestNativeScriptDecodeScalesLinearlyWithDepth(t *testing.T) {
	t.Parallel()
	const baseDepth = 1500
	small := decodeNativeScriptTime(t, nestedAllScript(baseDepth, false))
	large := decodeNativeScriptTime(t, nestedAllScript(baseDepth*4, false))
	if small <= 0 {
		t.Skipf("clock cannot resolve decode of depth %d", baseDepth)
	}
	ratio := float64(large) / float64(small)
	require.Lessf(
		t,
		ratio,
		10.0,
		"decode time grew %.1fx for 4x depth (%s -> %s); linear is ~4x, quadratic ~16x",
		ratio,
		small,
		large,
	)
}

func TestNativeScriptDeepNestingRoundTrip(t *testing.T) {
	t.Parallel()
	for _, indefinite := range []bool{false, true} {
		const depth = 3000
		raw := nestedAllScript(depth, indefinite)
		var script common.NativeScript
		require.NoError(t, script.UnmarshalCBOR(raw))
		require.Equal(t, raw, script.Cbor())
		marshaled, err := script.MarshalCBOR()
		require.NoError(t, err)
		require.Equal(t, raw, marshaled)
		require.Equal(
			t,
			common.ScriptHash(common.Blake2b224Hash(
				append([]byte{common.ScriptRefTypeNativeScript}, raw...),
			)),
			script.Hash(),
		)

		// Every node keeps exactly its own bytes, which are a suffix-framed
		// slice of the parent's.
		node := script
		offset := 0
		step := 3
		for i := range depth {
			all, ok := node.Item().(*common.NativeScriptAll)
			require.Truef(t, ok, "level %d is %T", i, node.Item())
			require.Len(t, all.Scripts, 1)
			if indefinite {
				require.Equal(t, raw[offset:len(raw)-i], node.Cbor())
			} else {
				require.Equal(t, raw[offset:], node.Cbor())
			}
			node = all.Scripts[0]
			offset += step
		}
		leaf, ok := node.Item().(*common.NativeScriptInvalidBefore)
		require.True(t, ok)
		require.Equal(t, uint(4), leaf.Type)
		require.Equal(t, uint64(0), leaf.Slot)
		require.Equal(t, []byte{0x82, 0x04, 0x00}, node.Cbor())
	}
}

func TestNativeScriptNestingAtCborLimit(t *testing.T) {
	t.Parallel()
	// A script level uses two nesting levels, so this is the deepest chain
	// the CBOR decoder accepts.
	atLimit := nestedAllScript(cbor.MaxNestedLevels/2-1, false)
	var script common.NativeScript
	require.NoError(t, script.UnmarshalCBOR(atLimit))
	require.Equal(t, atLimit, script.Cbor())

	tooDeep := nestedAllScript(cbor.MaxNestedLevels/2, false)
	var rejected common.NativeScript
	require.Error(t, rejected.UnmarshalCBOR(tooDeep))
}
