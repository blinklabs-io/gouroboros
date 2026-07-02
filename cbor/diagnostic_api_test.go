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

package cbor_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiagnosticNotation pins the formatter output to RFC 8949 § 8 diagnostic
// notation for the major types. Each case parses a well-known CBOR encoding
// and asserts that the compact formatter matches the canonical rendering.
func TestDiagnosticNotation(t *testing.T) {
	cases := []struct {
		name string
		hex  string
		want string
	}{
		{"uint", "0a", "10"},
		{"nint", "29", "-10"},
		{"bytes", "44deadbeef", "h'deadbeef'"},
		{"text", "6548656c6c6f", "\"Hello\""},
		{"array", "83010203", "[1, 2, 3]"},
		{"map", "a2616101616202", "{\"a\": 1, \"b\": 2}"},
		{"tag", "c074323031332d30332d32315432303a30343a30305a", "0(\"2013-03-21T20:04:00Z\")"},
		{"true", "f5", "true"},
		{"false", "f4", "false"},
		{"null", "f6", "null"},
		{"indef array", "9f0102ff", "[1, 2]"},
		{"indef bytes", "5f4201024103ff", "(_ h'0102', h'03')"},
		{"indef text", "7f61616162ff", "(_ \"a\", \"b\")"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(tc.hex)
			require.NoError(t, err)
			result, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			got := result.Root.FormatDiagnostic(cbor.DiagnosticOptions{})
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestByteOffsetTracking verifies that every node in the parsed tree has
// an Offset/Length that exactly indexes its CBOR-encoded slice within the
// original input. This is the foundational guarantee callers rely on when
// pointing users at byte ranges from an error.
func TestByteOffsetTracking(t *testing.T) {
	// {"a": [1, 2, 3], "b": h'cafe'}
	data, err := hex.DecodeString("a26161830102036162" + "42cafe")
	require.NoError(t, err)
	result, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)

	root := result.Root
	assert.Equal(t, 0, root.Offset)
	assert.Equal(t, len(data), root.Length)
	assert.Equal(t, cbor.DiagTypeMap, root.Type)
	require.Len(t, root.Children, 4)

	// Walk every node and check the raw bytes at [Offset:Offset+Length]
	// match the node's RawBytes copy.
	var walk func(n *cbor.DiagnosticNode)
	walk = func(n *cbor.DiagnosticNode) {
		require.True(t, n.Offset >= 0)
		require.True(t, n.Offset+n.Length <= len(data))
		assert.Equal(t, data[n.Offset:n.Offset+n.Length], n.RawBytes,
			"raw bytes mismatch at offset %d", n.Offset)
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(root)

	// GetNodeAtOffset should land on the inner array element at the right
	// position. The array [1, 2, 3] starts at offset 3 (after a2 61 61);
	// element 1 is at offset 4.
	elem0 := root.GetNodeAtOffset(4)
	require.NotNil(t, elem0)
	assert.Equal(t, cbor.DiagTypeUint, elem0.Type)
	assert.Equal(t, uint64(1), elem0.Value)
}

// TestCardanoLabels asserts that the Cardano-aware formatters emit the
// expected field labels for a representative transaction and block. Body
// keys (inputs/outputs/fee) and block positions (header /
// transaction_bodies / invalid_transactions) are pinned so a label-table
// rename or re-indexing gets caught immediately.
func TestCardanoLabels(t *testing.T) {
	// tx = [body, witness_set, true, null] with body = {0:[],1:[],2:100}
	body := "a3" + "00" + "80" + "01" + "80" + "02" + "1864"
	tx := "84" + body + "a0" + "f5" + "f6"
	txData, err := hex.DecodeString(tx)
	require.NoError(t, err)

	result, err := cbor.DiagnoseTransaction(txData, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	require.NotNil(t, result.Root)
	assert.Equal(t, len(txData), result.Statistics.TotalBytes)
	assert.Empty(t, result.Warnings, "well-formed tx should have no warnings")

	out, err := cbor.FormatTransactionDiagnostic(txData, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	for _, want := range []string{
		"transaction [", "body: {", "witness_set: {",
		"is_valid: true", "auxiliary_data: null",
		"/ inputs /", "/ outputs /", "/ fee /",
	} {
		assert.Contains(t, out, want)
	}

	// inner Dijkstra block = [header, [], [], {}, [], null, null]
	inner := "87" + "8100" + "80" + "80" + "a0" + "80" + "f6" + "f6"
	full := "82" + "07" + inner // [era=7, inner]
	blockData, err := hex.DecodeString(full)
	require.NoError(t, err)

	bResult, err := cbor.DiagnoseBlock(blockData, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	assert.Empty(t, bResult.Warnings, "well-formed block should have no warnings")

	bOut, err := cbor.FormatBlockDiagnostic(blockData, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	for _, want := range []string{
		"block [", "/ era 7 /", "header:",
		"transaction_bodies: []",
		"transaction_witnesses: []",
		"auxiliary_data_set: {}",
		"invalid_transactions: []",
		"leios_cert: null",
		"peras_cert: null",
	} {
		assert.Contains(t, bOut, want)
	}
}

// TestMalformedInput covers each rejected-input category the diagnostic
// pipeline guards: empty buffer, truncated headers, oversized collection
// counts, trailing data after the first item, mismatched tx/block shape.
func TestMalformedInput(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		_, err := cbor.Diagnose(nil, cbor.DiagnosticOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty input")
	})

	t.Run("truncated array header", func(t *testing.T) {
		// 0x98 expects a 1-byte length to follow; we provide none.
		_, err := cbor.Diagnose([]byte{0x98}, cbor.DiagnosticOptions{})
		require.Error(t, err)
	})

	t.Run("oversized collection", func(t *testing.T) {
		// 0x9b followed by uint64 max — must be rejected as out of range.
		data, hexErr := hex.DecodeString("9bffffffffffffffff")
		require.NoError(t, hexErr)
		_, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds int32 range")
	})

	t.Run("trailing data", func(t *testing.T) {
		// uint 1 followed by an unrelated uint 2 in the same buffer.
		_, err := cbor.Diagnose([]byte{0x01, 0x02}, cbor.DiagnosticOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trailing CBOR data")
	})

	t.Run("non-array transaction", func(t *testing.T) {
		// {} can't be a tx.
		_, err := cbor.DiagnoseTransaction([]byte{0xa0}, cbor.DiagnosticOptions{})
		require.Error(t, err)
	})

	t.Run("tx with bad is_valid", func(t *testing.T) {
		// 4-element tx where element 2 is uint 100 instead of bool.
		body := "a1" + "02" + "1864"
		tx := "84" + body + "a0" + "1864" + "a0"
		data, hexErr := hex.DecodeString(tx)
		require.NoError(t, hexErr)
		result, err := cbor.DiagnoseTransaction(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		require.NotEmpty(t, result.Warnings)
		assert.Contains(t, result.Warnings[0], "not a bool")
	})

	t.Run("non-array block body", func(t *testing.T) {
		// Bare uint can't be a block.
		_, err := cbor.DiagnoseBlock([]byte{0x01}, cbor.DiagnosticOptions{})
		require.Error(t, err)
	})

	t.Run("oversized input", func(t *testing.T) {
		// 16 MiB + 1 byte of padding past a valid 1-byte item. The
		// length check fires before parsing, so the contents don't have
		// to be valid CBOR.
		data := make([]byte, 16*1024*1024+1)
		_, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds diagnostic limit")
	})

	t.Run("oversized transaction warning", func(t *testing.T) {
		// 17 KiB definite byte string wrapped as a 2-elem tx-ish array.
		// Larger than max_tx_size; DiagnoseTransaction must warn but
		// still parse — diagnostics on out-of-spec bytes is the point.
		body := append([]byte{0x59, 0x42, 0x00}, make([]byte, 0x4200)...)
		tx := append([]byte{0x82}, body...)
		tx = append(tx, 0xa0)
		result, err := cbor.DiagnoseTransaction(tx, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		require.NotEmpty(t, result.Warnings)
		var found bool
		for _, w := range result.Warnings {
			if strings.Contains(w, "max_tx_size") {
				found = true
				break
			}
		}
		assert.True(t, found, "expected max_tx_size warning, got %v", result.Warnings)
	})

	t.Run("block with non-tag-24 outer tag", func(t *testing.T) {
		// tag(99) wrapping a 5-element inner array. The diagnostic API
		// must reject the unknown wrapper rather than silently unwrapping.
		// 0xd8 0x63 = tag(99); inner = 85 8100 80 80 a0 80.
		inner := "85" + "8100" + "80" + "80" + "a0" + "80"
		data, hexErr := hex.DecodeString("d863" + inner)
		require.NoError(t, hexErr)
		_, err := cbor.DiagnoseBlock(data, cbor.DiagnosticOptions{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported block outer tag")
	})

	t.Run("block with overflow era", func(t *testing.T) {
		// [era=u64-max, inner] — the era id is impossible but the inner
		// block must still be unwrapped and validated. Pre-fix this
		// returned a bogus "block body has 2 fields" warning.
		// 0x82 1b ff ff ff ff ff ff ff ff <inner>
		inner := "85" + "8100" + "80" + "80" + "a0" + "80"
		data, hexErr := hex.DecodeString(
			"82" + "1bffffffffffffffff" + inner,
		)
		require.NoError(t, hexErr)
		result, err := cbor.DiagnoseBlock(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		var sawEraWarning bool
		var sawFieldsWarning bool
		for _, w := range result.Warnings {
			if strings.Contains(w, "unexpected era id") {
				sawEraWarning = true
			}
			if strings.Contains(w, "fields") {
				sawFieldsWarning = true
			}
		}
		assert.True(t, sawEraWarning,
			"expected era-id warning, got %v", result.Warnings)
		assert.False(t, sawFieldsWarning,
			"inner should have been unwrapped — no fields warning expected; got %v",
			result.Warnings)
	})

	t.Run("block with extra fields", func(t *testing.T) {
		// 6-element inner array — neither Conway's 5-field shape nor
		// Dijkstra's 7-field shape.
		inner := "86" + "8100" + "80" + "80" + "a0" + "80" + "80"
		data, hexErr := hex.DecodeString(inner)
		require.NoError(t, hexErr)
		result, err := cbor.DiagnoseBlock(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		require.NotEmpty(t, result.Warnings)
		assert.Contains(t, result.Warnings[0], "6 fields")
	})
}

// TestDiagnosticStats covers the stats walker: element counts include
// every node, byte/text string sizes are summed correctly for both
// definite and indefinite forms (the latter must not double-count the
// parent + chunk children), and MaxDepth tracks the deepest nesting.
func TestDiagnosticStats(t *testing.T) {
	t.Run("definite strings", func(t *testing.T) {
		// {"abc": h'deadbeef'} — text(3) + bytes(4) → sizes 3 and 4.
		data, err := hex.DecodeString("a16361626344deadbeef")
		require.NoError(t, err)
		r, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		assert.Equal(t, len(data), r.Statistics.TotalBytes)
		assert.Equal(t, 3, r.Statistics.TextStringSize)
		assert.Equal(t, 4, r.Statistics.ByteStringSize)
		// map + key + value = 3 elements
		assert.Equal(t, 3, r.Statistics.ElementCount)
		assert.Equal(t, 1, r.Statistics.MaxDepth)
	})

	t.Run("indefinite bytes not double-counted", func(t *testing.T) {
		// (_ h'0102', h'03') — 2-byte chunk + 1-byte chunk = 3 total.
		// Without the indefinite guard, the parent's concatenated value
		// (3 bytes) plus each chunk (2+1) would tally 6.
		data, err := hex.DecodeString("5f4201024103ff")
		require.NoError(t, err)
		r, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		assert.Equal(
			t, 3, r.Statistics.ByteStringSize,
			"indefinite byte string chunks should sum to 3, not 6",
		)
		// 1 parent + 2 chunks = 3 element count
		assert.Equal(t, 3, r.Statistics.ElementCount)
	})

	t.Run("indefinite text not double-counted", func(t *testing.T) {
		// (_ "a", "b") — 1 + 1 = 2 chars total, must not be 4.
		data, err := hex.DecodeString("7f61616162ff")
		require.NoError(t, err)
		r, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		assert.Equal(t, 2, r.Statistics.TextStringSize)
	})

	t.Run("max depth", func(t *testing.T) {
		// [[[1]]] — three nested arrays.
		data, err := hex.DecodeString("81818101")
		require.NoError(t, err)
		r, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		assert.Equal(t, 3, r.Statistics.MaxDepth)
		assert.Equal(t, 4, r.Statistics.ElementCount) // 3 arrays + 1 leaf
	})
}

// TestDiagnoseTransactionShapes pins the warning behaviour for the four
// valid transaction shapes (2, 3, 4 elements; 4 with a non-bool index 2)
// and the two invalid ones (1 element, 5+ elements). Pre-Alonzo
// transactions in particular must NOT produce warnings.
func TestDiagnoseTransactionShapes(t *testing.T) {
	cases := []struct {
		name       string
		hex        string
		wantWarn   bool
		wantSubstr string
	}{
		{
			name:     "2 elements (body+ws)",
			hex:      "82" + "a0" + "a0",
			wantWarn: false,
		},
		{
			name:     "3 elements (pre-Alonzo)",
			hex:      "83" + "a0" + "a0" + "a0",
			wantWarn: false,
		},
		{
			name:     "4 elements with bool is_valid",
			hex:      "84" + "a0" + "a0" + "f5" + "a0",
			wantWarn: false,
		},
		{
			name:       "1 element (too short)",
			hex:        "81" + "a0",
			wantWarn:   true,
			wantSubstr: "1 elements",
		},
		{
			name:       "5 elements (too long)",
			hex:        "85" + "a0" + "a0" + "a0" + "a0" + "a0",
			wantWarn:   true,
			wantSubstr: "5 elements",
		},
		{
			name:       "4 elements, non-bool is_valid",
			hex:        "84" + "a0" + "a0" + "1864" + "a0",
			wantWarn:   true,
			wantSubstr: "not a bool",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := hex.DecodeString(tc.hex)
			require.NoError(t, err)
			r, err := cbor.DiagnoseTransaction(data, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			if tc.wantWarn {
				require.NotEmpty(t, r.Warnings)
				assert.Contains(t, r.Warnings[0], tc.wantSubstr)
			} else {
				assert.Empty(t, r.Warnings, "got unexpected warnings: %v", r.Warnings)
			}
		})
	}
}

// TestDiagnoseAddressAnnotationOnResultRoot proves DiagnoseTransaction
// mutates the same node the caller can see via result.Root — not a stale
// copy or shadowed pointer. A stub formatter labels any 29-byte string;
// the assertion walks result.Root afterwards and finds the Comment.
func TestDiagnoseAddressAnnotationOnResultRoot(t *testing.T) {
	cbor.RegisterAddressFormatter(func(b []byte) (string, bool) {
		if len(b) == 29 {
			return "addr_stub", true
		}
		return "", false
	})
	t.Cleanup(func() { cbor.RegisterAddressFormatter(nil) })

	addr := strings.Repeat("aa", 29)
	// tx = [body, witness_set] where body = {0: h'aa..aa'}
	body := "a1" + "00" + "581d" + addr
	tx := "82" + body + "a0"
	data, err := hex.DecodeString(tx)
	require.NoError(t, err)

	r, err := cbor.DiagnoseTransaction(data, cbor.DiagnosticOptions{})
	require.NoError(t, err)
	require.NotNil(t, r.Root)

	// Walk result.Root and find the bytes node — its Comment must carry
	// the annotation written by DiagnoseTransaction.
	var found string
	var walk func(n *cbor.DiagnosticNode)
	walk = func(n *cbor.DiagnosticNode) {
		if n.Type == cbor.DiagTypeBytes && n.Comment != "" {
			found = n.Comment
		}
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(r.Root)
	assert.Equal(t, "addr_stub", found,
		"AnnotateAddresses must mutate the node reachable via result.Root")
}

// TestRoundTrip walks a parsed tree's compact diagnostic notation, ensures
// the original bytes can be re-parsed cleanly, and checks GetPathToOffset
// returns the same path before and after a re-parse. We don't try to
// reconstruct CBOR from diagnostic notation (that would require a separate
// decoder) — instead we round-trip the *parse*, which is the property the
// public API guarantees.
func TestRoundTrip(t *testing.T) {
	hexInputs := []string{
		"83010203",       // [1,2,3]
		"a2616101616202", // {"a":1,"b":2}
		"a164626f6479a1676f75747075747381a167616464726573734101", // nested address fixture
		"d901028101", // 258([1])
		"9f0102ff",   // indefinite [1,2]
	}
	for _, h := range hexInputs {
		t.Run(h, func(t *testing.T) {
			data, err := hex.DecodeString(h)
			require.NoError(t, err)

			r1, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			notation1 := r1.Root.FormatDiagnostic(cbor.DiagnosticOptions{})

			// Re-parse the original bytes and check formatted output is
			// stable (parse → format → re-parse → re-format).
			r2, err := cbor.Diagnose(data, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			notation2 := r2.Root.FormatDiagnostic(cbor.DiagnosticOptions{})
			assert.Equal(t, notation1, notation2)

			// Statistics must agree between parses.
			assert.Equal(t, r1.Statistics, r2.Statistics)

			// Path lookup is offset-stable across parses.
			for off := range data {
				p1 := r1.Root.GetPathToOffset(off)
				p2 := r2.Root.GetPathToOffset(off)
				assert.Equal(t, p1, p2, "path mismatch at offset %d", off)
			}
		})
	}
}

// TestLargeBlock exercises the diagnostic pipeline on real mainnet block
// fixtures across every era. Each block must parse, populate stats, and
// produce non-empty diagnostic notation — slow paths and big-collection
// edges only show up at production size.
func TestLargeBlock(t *testing.T) {
	for _, tb := range testdata.GetTestBlocks() {
		t.Run(tb.Name, func(t *testing.T) {
			result, err := cbor.Diagnose(tb.Cbor, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			require.NotNil(t, result.Root)
			assert.Equal(t, len(tb.Cbor), result.Statistics.TotalBytes)
			assert.Greater(t, result.Statistics.ElementCount, 0)
			assert.Greater(t, result.Statistics.MaxDepth, 0)

			// First, exercise the full formatter without any caps to make
			// sure formatting on real bytes doesn't blow up.
			full := result.Root.FormatDiagnostic(cbor.DiagnosticOptions{})
			assert.NotEmpty(t, full)

			// Then isolate MaxDepth: with only the depth cap set, the only
			// way "..." can appear is via depth truncation. We pick a
			// depth shallower than the recorded MaxDepth so truncation
			// must fire.
			require.Greater(t, result.Statistics.MaxDepth, 1,
				"real block should have depth > 1")
			depthOnly := result.Root.FormatDiagnostic(cbor.DiagnosticOptions{
				MaxDepth: 1,
			})
			assert.True(t, strings.Contains(depthOnly, "..."),
				"MaxDepth=1 should produce depth-truncation markers")
		})
	}
}

// BenchmarkDiagnosticParse measures the per-block ParseDiagnostic cost on
// every era's fixture. Reports allocs/op so a regression in tree-building
// memory shows up alongside latency.
func BenchmarkDiagnosticParse(b *testing.B) {
	for _, tb := range testdata.GetTestBlocks() {
		b.Run(tb.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tb.Cbor)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := cbor.ParseDiagnostic(tb.Cbor)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDiagnosticFormat measures FormatDiagnostic on a pre-parsed
// tree per era. Parsing is done once outside the timed loop so the
// benchmark isolates the formatter.
func BenchmarkDiagnosticFormat(b *testing.B) {
	opts := cbor.DiagnosticOptions{
		MaxDepth:      6,
		MaxArrayItems: 8,
		MaxByteLength: 32,
	}
	for _, tb := range testdata.GetTestBlocks() {
		node, err := cbor.ParseDiagnostic(tb.Cbor)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(tb.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = node.FormatDiagnostic(opts)
			}
		})
	}
}

// BenchmarkFullBlockDiagnosis exercises the end-to-end Diagnose path
// (parse + stats walk) on each era's block fixture.
func BenchmarkFullBlockDiagnosis(b *testing.B) {
	opts := cbor.DiagnosticOptions{}
	for _, tb := range testdata.GetTestBlocks() {
		b.Run(tb.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(tb.Cbor)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := cbor.Diagnose(tb.Cbor, opts)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
