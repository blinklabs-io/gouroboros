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
	"runtime"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
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

// decodeNativeScriptTime returns the time to decode data repeat times.
func decodeNativeScriptTime(
	t *testing.T,
	data []byte,
	repeat int,
) time.Duration {
	t.Helper()
	// Collect first so a cycle triggered by an earlier measurement does not
	// land inside this one.
	runtime.GC()
	start := time.Now()
	for range repeat {
		var script common.NativeScript
		require.NoError(t, script.UnmarshalCBOR(data))
	}
	return time.Since(start)
}

// Decoding must visit each byte a constant number of times. The test decodes
// a shallow script factor times and a script factor times deeper once, so
// both sides process the same number of bytes: quadratic decode takes factor
// times longer per byte on the deep one, linear decode only the 2-4x that
// deep recursion costs in stack growth and cache locality. The bound sits
// between the two. Interleaving
// the two sides and keeping the fastest of several rounds of each cancels
// clock-speed drift and scheduler noise, and the test is not parallel so
// sibling tests do not compete with the timed runs.
func TestNativeScriptDecodeScalesLinearlyWithDepth(t *testing.T) {
	const baseDepth = 500
	const factor = 16
	shallow := nestedAllScript(baseDepth, false)
	deep := nestedAllScript(baseDepth*factor, false)
	small := time.Duration(1<<63 - 1)
	large := small
	for range 7 {
		small = min(small, decodeNativeScriptTime(t, shallow, factor))
		large = min(large, decodeNativeScriptTime(t, deep, 1))
	}
	if small <= 0 {
		t.Skipf("clock cannot resolve decode of depth %d", baseDepth)
	}
	ratio := float64(large) / float64(small)
	require.Lessf(
		t,
		ratio,
		7.0,
		"decoding %dx deeper took %.1fx as long per byte (%s -> %s); "+
			"linear is ~1x, quadratic ~%dx",
		factor,
		ratio,
		small,
		large,
		factor,
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

// A child list and each child must be plain CBOR arrays. The reference ledger
// decodes them with decodeStrictSeq and the Timelock sum decoder, neither of
// which accepts a tag in front of the list or the child.
func TestNativeScriptRejectsTaggedChildren(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string][]byte{
		"set tag on child list": {0x82, 0x01, 0xd9, 0x01, 0x02, 0x80},
		"self-describe tag on child list": {
			0x82, 0x01, 0xd9, 0xd9, 0xf7, 0x81, 0x82, 0x04, 0x00,
		},
		"self-describe tag on child": {
			0x82, 0x01, 0x81, 0xd9, 0xd9, 0xf7, 0x82, 0x04, 0x00,
		},
		"self-describe tag on indefinite child": {
			0x82, 0x01, 0x9f, 0xd9, 0xd9, 0xf7, 0x82, 0x04, 0x00, 0xff,
		},
		"tag on n-of-k child list": {
			0x83, 0x03, 0x01, 0xd9, 0x01, 0x02, 0x81, 0x82, 0x04, 0x00,
		},
	} {
		var script common.NativeScript
		assert.Errorf(t, script.UnmarshalCBOR(raw), "%s: %x", name, raw)
	}
}

func TestNativeScriptOversizedChildListLengthsStayBounded(t *testing.T) {
	// Each level is [1, <list declaring 2^31-1 children>] holding only the
	// next level, so the input is truncated and must be rejected without
	// reserving memory for the declared lengths.
	const depth = 2000
	var data []byte
	for range depth {
		data = append(data, 0x82, 0x01, 0x9a, 0x7f, 0xff, 0xff, 0xff)
	}
	data = append(data, 0x82, 0x04, 0x00)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	var script common.NativeScript
	err := script.UnmarshalCBOR(data)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	allocated := after.TotalAlloc - before.TotalAlloc
	require.Less(
		t,
		allocated,
		uint64(16<<20),
		"decoding a %d-byte script allocated %d bytes",
		len(data),
		allocated,
	)
}
