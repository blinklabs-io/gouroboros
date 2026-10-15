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
	"bytes"
	"encoding/hex"
	"math"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderTruncationMarker is the text a capped render ends with. The tests
// spell it out rather than import it so a change to the user-visible marker
// is a deliberate test edit.
const renderTruncationMarker = "... (output truncated)"

// nestedDiagnosticInput returns depth single-element containers around 0:
// arrays ([[...0...]]) or maps ({0: {0: ...0...}}).
func nestedDiagnosticInput(kind string, depth int) []byte {
	if kind == "array" {
		return append(bytes.Repeat([]byte{0x81}, depth), 0x00)
	}
	return append(bytes.Repeat([]byte{0xa1, 0x00}, depth), 0x00)
}

// diagnosticRenderer lets one test body run against every renderer that shares
// the output cap. The markers sit differently in each format, so body undoes
// that per format.
type diagnosticRenderer struct {
	name   string
	render func(*cbor.DiagnosticNode, cbor.DiagnosticOptions) string
	// body strips the truncation marker, leaving the rendered prefix.
	body func(string) string
}

// diagnosticRenderers covers the three renderers on DiagnosticNode, in the
// order compact, pretty, hex. Tests that skip compact rely on that order.
var diagnosticRenderers = []diagnosticRenderer{
	{
		name: "compact",
		render: func(n *cbor.DiagnosticNode, o cbor.DiagnosticOptions) string {
			return n.FormatDiagnostic(o)
		},
		body: func(s string) string {
			return strings.TrimSuffix(s, renderTruncationMarker)
		},
	},
	{
		name: "pretty",
		render: func(n *cbor.DiagnosticNode, o cbor.DiagnosticOptions) string {
			return n.FormatDiagnosticPretty(o)
		},
		body: func(s string) string {
			return strings.TrimSuffix(s, "\n"+renderTruncationMarker)
		},
	},
	{
		name: "hex",
		render: func(n *cbor.DiagnosticNode, o cbor.DiagnosticOptions) string {
			return n.FormatHexDump(o)
		},
		body: func(s string) string {
			return strings.TrimSuffix(
				strings.TrimSuffix(s, "\n"),
				renderTruncationMarker,
			)
		},
	},
}

// TestDiagnosticRenderOutputCapBoundary pins the cap to the byte: a render
// that fits is unchanged, and one that does not is a prefix of the full output
// that ends with the marker and never exceeds the cap before it.
//
// An off-by-one here would either truncate output that fits, changing what
// callers see today, or let output run past the limit it promises.
func TestDiagnosticRenderOutputCapBoundary(t *testing.T) {
	t.Parallel()

	// [1, [2, 3], {"a": null}]
	data, err := hex.DecodeString("8301820203a16161f6")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	for _, r := range diagnosticRenderers {
		t.Run(r.name, func(t *testing.T) {
			t.Parallel()

			full := r.render(node, cbor.DiagnosticOptions{})
			require.NotContains(t, full, renderTruncationMarker)

			for _, limit := range []int{
				len(full), len(full) + 1, math.MaxInt,
			} {
				assert.Equal(
					t,
					full,
					r.render(
						node,
						cbor.DiagnosticOptions{MaxOutputBytes: limit},
					),
					"limit %d",
					limit,
				)
			}
			// Zero and negative select the default, which fits.
			for _, limit := range []int{0, -1} {
				assert.Equal(
					t,
					full,
					r.render(
						node,
						cbor.DiagnosticOptions{MaxOutputBytes: limit},
					),
					"limit %d",
					limit,
				)
			}
			for _, limit := range []int{len(full) - 1, len(full) / 2, 1} {
				out := r.render(
					node, cbor.DiagnosticOptions{MaxOutputBytes: limit},
				)
				require.Contains(
					t,
					out,
					renderTruncationMarker,
					"limit %d",
					limit,
				)
				body := r.body(out)
				assert.LessOrEqual(t, len(body), limit, "limit %d", limit)
				assert.True(
					t,
					strings.HasPrefix(full, body),
					"limit %d: %q",
					limit,
					body,
				)
			}
		})
	}
}

// TestDiagnosticRenderBoundedAtMaxNesting renders the deepest value the parser
// admits. Unbounded, the 16 KiB array chain renders 512 MB of pretty output
// and 257 MB of hex, and recopies nested output at every level on the way.
//
// This is the regression test for the quadratic blow-up itself: it fails if
// the default cap stops applying or an explicit MaxOutputBytes is ignored.
// Compact output is linear in the input, so the default leaves it whole.
func TestDiagnosticRenderBoundedAtMaxNesting(t *testing.T) {
	// Not t.Parallel: MaxNestedLevels is a package variable that other tests
	// reassign.
	depth := cbor.MaxNestedLevels
	for _, kind := range []string{"array", "map"} {
		data := nestedDiagnosticInput(kind, depth)
		node, err := cbor.ParseDiagnostic(data)
		require.NoError(t, err, kind)

		for _, r := range diagnosticRenderers {
			t.Run(kind+"/"+r.name+"/default", func(t *testing.T) {
				out := r.render(node, cbor.DiagnosticOptions{})
				// The default cap is 4 MiB plus 256 bytes per input byte,
				// about 8 MiB here. Compact output is linear in the input and
				// fits, so it is rendered whole.
				assert.Less(t, len(out), 16<<20)
				if r.name == "compact" {
					assert.NotContains(t, out, renderTruncationMarker)
					opening := map[string]string{"array": "[", "map": "{0: "}[kind]
					assert.True(
						t,
						strings.HasPrefix(out, strings.Repeat(opening, depth)),
					)
					return
				}
				assert.Contains(t, out, renderTruncationMarker)
			})
			// A caller-supplied cap far below the default is honored exactly.
			t.Run(kind+"/"+r.name+"/explicit", func(t *testing.T) {
				const limit = 1024
				out := r.render(
					node,
					cbor.DiagnosticOptions{MaxOutputBytes: limit},
				)
				assert.Contains(t, out, renderTruncationMarker)
				assert.LessOrEqual(t, len(r.body(out)), limit)
			})
		}
	}
}

// TestDiagnosticRenderDefaultCapSparesOrdinaryDepth guards the other side of
// the cap: a default that was too small would truncate legitimate output.
// Nesting far deeper than any Cardano structure must still render whole, which
// is what keeps the cap from changing results for ordinary callers.
func TestDiagnosticRenderDefaultCapSparesOrdinaryDepth(t *testing.T) {
	t.Parallel()

	// 1,000 levels is far deeper than any Cardano structure and still renders
	// about 2 MB of pretty output, inside the default floor.
	node, err := cbor.ParseDiagnostic(nestedDiagnosticInput("array", 1000))
	require.NoError(t, err)
	for _, r := range diagnosticRenderers[1:] {
		out := r.render(node, cbor.DiagnosticOptions{})
		assert.NotContains(t, out, renderTruncationMarker, r.name)
		assert.Greater(t, len(out), 1<<20, r.name)
	}
}

// TestGetPathToOffsetCopiesNoPrefixes checks that GetPathToOffset does work
// linear in the depth. The path is a single descent, so it needs no copy at
// each level; copying the growing prefix made it quadratic. The path is not
// rendered, so the output cap does not help here and the allocation is
// measured directly. Both arrays and maps are covered because each builds its
// path segments through a different branch.
func TestGetPathToOffsetCopiesNoPrefixes(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter, and
	// MaxNestedLevels is reassigned by other tests.
	depth := cbor.MaxNestedLevels
	for _, kind := range []string{"array", "map"} {
		data := nestedDiagnosticInput(kind, depth)
		node, err := cbor.ParseDiagnostic(data)
		require.NoError(t, err, kind)

		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		path := node.GetPathToOffset(len(data) - 1)
		runtime.ReadMemStats(&after)

		// The explicit nil check also tells nilaway that indexing is safe;
		// it does not model require.Len as a guarantee.
		if path == nil {
			t.Fatalf("%s: no path to the deepest offset", kind)
		}
		require.Len(t, path, depth, kind)
		if kind == "array" {
			assert.Equal(t, "[0]", path[0])
		} else {
			assert.Equal(t, "0", path[0])
		}
		// Copying the path at every level allocated about 2 GB here.
		assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(32<<20), kind)
	}
}

// TestCardanoFormattersHonorOutputCap checks that the Cardano-aware formatters
// obey the same MaxOutputBytes as the generic renderers. They share
// DiagnosticOptions and indent every nesting level, so without the cap they
// grow quadratically with depth in the same way. A transaction must be an
// array at the root, hence the extra wrapper byte in its input.
func TestCardanoFormattersHonorOutputCap(t *testing.T) {
	// Not t.Parallel: MaxNestedLevels is reassigned by other tests.
	deep := nestedDiagnosticInput("array", cbor.MaxNestedLevels-1)
	formatters := map[string]struct {
		format func([]byte, cbor.DiagnosticOptions) (string, error)
		input  []byte
	}{
		"plutus": {cbor.FormatPlutusData, deep},
		"tx": {
			cbor.FormatTransactionDiagnostic,
			append([]byte{0x81}, deep...),
		},
	}
	for name, f := range formatters {
		// With no MaxOutputBytes the size-derived default must still stop it.
		t.Run(name+"/default", func(t *testing.T) {
			out, err := f.format(f.input, cbor.DiagnosticOptions{})
			require.NoError(t, err)
			assert.Less(t, len(out), 16<<20)
			assert.True(t, strings.HasSuffix(out, renderTruncationMarker))
		})
		// An explicit cap is honored exactly; the marker may add one line.
		t.Run(name+"/explicit", func(t *testing.T) {
			out, err := f.format(
				f.input,
				cbor.DiagnosticOptions{MaxOutputBytes: 1024},
			)
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(out, "\n"+renderTruncationMarker))
			assert.LessOrEqual(t, len(out), 1024+len(renderTruncationMarker)+1)
		})
	}

	// The exact boundary again, for the Cardano writer: a cap equal to the
	// output leaves it whole, one byte less truncates it.
	t.Run("boundary", func(t *testing.T) {
		// [1, [2, 3]]
		small, err := hex.DecodeString("8201820203")
		require.NoError(t, err)
		full, err := cbor.FormatPlutusData(small, cbor.DiagnosticOptions{})
		require.NoError(t, err)
		same, err := cbor.FormatPlutusData(
			small, cbor.DiagnosticOptions{MaxOutputBytes: len(full)},
		)
		require.NoError(t, err)
		assert.Equal(t, full, same)
		cut, err := cbor.FormatPlutusData(
			small, cbor.DiagnosticOptions{MaxOutputBytes: len(full) - 1},
		)
		require.NoError(t, err)
		assert.True(t, strings.HasSuffix(cut, renderTruncationMarker))
	})
}

// TestDiagnosticRenderSkipsLeavesPastCap checks that MaxOutputBytes bounds
// transient memory as well as output: a byte or text string too large for the
// cap must not be hex-encoded, quoted, or formatted before the cap is checked.
// Each 8 MiB leaf would otherwise allocate tens of megabytes to produce output
// that is then discarded. The ShowHex case matters separately because the hex
// suffix is built from the whole node even when the body fits.
func TestDiagnosticRenderSkipsLeavesPastCap(t *testing.T) {
	// Not t.Parallel: reads the process-wide allocation counter.
	const size = 8 << 20
	const limit = 1024
	leaves := map[string][]byte{
		"bytes": append(
			[]byte{0x5a, 0x00, 0x80, 0x00, 0x00},
			make([]byte, size)...),
		"text": append(
			[]byte{0x7a, 0x00, 0x80, 0x00, 0x00},
			bytes.Repeat([]byte("a"), size)...,
		),
	}
	allocated := func(f func()) uint64 {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		f()
		runtime.ReadMemStats(&after)
		return after.TotalAlloc - before.TotalAlloc
	}
	for name, data := range leaves {
		node, err := cbor.ParseDiagnostic(data)
		require.NoError(t, err, name)

		for _, r := range diagnosticRenderers[:2] {
			var out string
			grew := allocated(func() {
				out = r.render(
					node, cbor.DiagnosticOptions{MaxOutputBytes: limit},
				)
			})
			assert.Contains(t, out, renderTruncationMarker, name+"/"+r.name)
			assert.Less(t, grew, uint64(1<<20), name+"/"+r.name)
		}

		// FormatPlutusData parses its input itself, which costs several copies
		// of the leaf, so compare against the cost of a plain parse: only the
		// rendering must stay small.
		parsed := allocated(func() {
			_, err = cbor.ParseDiagnostic(data)
		})
		require.NoError(t, err, name)
		var out string
		grew := allocated(func() {
			out, err = cbor.FormatPlutusData(
				data, cbor.DiagnosticOptions{MaxOutputBytes: limit},
			)
		})
		require.NoError(t, err, name)
		assert.Contains(t, out, renderTruncationMarker, name+"/cardano")
		assert.Less(t, grew, parsed+(1<<20), name+"/cardano")
	}

	// The source text can fit while its escaped representation does not.
	// Check the quoted length before allocating it.
	escapedText := append(
		[]byte{0x7a, 0x00, 0x08, 0x00, 0x00},
		make([]byte, 512<<10)...,
	)
	node, err := cbor.ParseDiagnostic(escapedText)
	require.NoError(t, err)
	grew := allocated(func() {
		out := node.FormatDiagnostic(
			cbor.DiagnosticOptions{MaxOutputBytes: 600 << 10},
		)
		assert.Contains(t, out, renderTruncationMarker)
	})
	assert.Less(t, grew, uint64(1<<20))

	// A truncated byte string keeps the body tiny, so only the ShowHex suffix
	// is large enough to hit the cap.
	node, err = cbor.ParseDiagnostic(leaves["bytes"])
	require.NoError(t, err)
	var out string
	grew = allocated(func() {
		out = node.FormatDiagnostic(cbor.DiagnosticOptions{
			MaxByteLength:  4,
			ShowHex:        true,
			MaxOutputBytes: limit,
		})
	})
	assert.Contains(t, out, renderTruncationMarker)
	assert.Less(t, grew, uint64(1<<20))
}

// TestCardanoFormatterStopsAtOversizedLeaf checks that a leaf too large for the
// cap ends the whole render. The Cardano writer once rendered each leaf through
// a separate capped writer and copied the result in, which embedded a marker
// in the middle of the output and carried on rendering the elements after it.
func TestCardanoFormatterStopsAtOversizedLeaf(t *testing.T) {
	t.Parallel()

	// [1, h'00...00' (4096 bytes), 7777]
	data := append([]byte{0x83, 0x01, 0x59, 0x10, 0x00}, make([]byte, 4096)...)
	data = append(data, 0x19, 0x1e, 0x61)

	out, err := cbor.FormatPlutusData(
		data, cbor.DiagnosticOptions{MaxOutputBytes: 256},
	)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(out, "\n"+renderTruncationMarker), out)
	assert.Equal(
		t, 1, strings.Count(out, renderTruncationMarker),
		"one marker, at the end",
	)
	assert.NotContains(t, out, "7777", "rendering stopped at the leaf")
}
