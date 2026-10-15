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

package cbor

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	// The default output cap is a fixed floor plus a multiple of the encoded
	// size of the rendered node. Indentation and the fixed-width hex columns
	// make a nested item cost far more output than input, so a cap that
	// ignored the input would either truncate large legitimate dumps or admit
	// hundreds of megabytes from a few kilobytes of deeply nested input.
	diagnosticOutputBaseBytes         = 4 << 20
	diagnosticOutputBytesPerInputByte = 256

	diagnosticTruncationMarker = "... (output truncated)"
)

// outputLimit returns the output cap for rendering n.
func (o DiagnosticOptions) outputLimit(n *DiagnosticNode) int {
	if o.MaxOutputBytes > 0 {
		return o.MaxOutputBytes
	}
	if n.Length > (math.MaxInt-diagnosticOutputBaseBytes)/
		diagnosticOutputBytesPerInputByte {
		return math.MaxInt
	}
	return diagnosticOutputBaseBytes +
		diagnosticOutputBytesPerInputByte*max(n.Length, 0)
}

// diagnosticWriter accumulates rendered output up to a byte cap. Each write is
// all or nothing: the first one that would pass the cap marks the writer
// truncated and every later write is a no-op, so callers can keep walking and
// stop at their next convenient check without growing the output.
type diagnosticWriter struct {
	b         strings.Builder
	limit     int
	truncated bool
}

func newDiagnosticWriter(
	opts DiagnosticOptions,
	n *DiagnosticNode,
) *diagnosticWriter {
	return &diagnosticWriter{limit: opts.outputLimit(n)}
}

// fits reports whether n more bytes fit under the cap. When they do not it
// marks the writer truncated, so a caller can skip building a value that write
// would refuse anyway instead of allocating it first.
func (w *diagnosticWriter) fits(n int) bool {
	if w.truncated {
		return false
	}
	if n < 0 || n > w.limit-w.b.Len() {
		w.truncated = true
		return false
	}
	return true
}

// write appends s and reports whether rendering may continue.
func (w *diagnosticWriter) write(s string) bool {
	if !w.fits(len(s)) {
		return false
	}
	w.b.WriteString(s)
	return true
}

// writeRepeat appends n copies of s, checking the cap before allocating.
func (w *diagnosticWriter) writeRepeat(s string, n int) bool {
	if w.truncated {
		return false
	}
	if n <= 0 || s == "" {
		return true
	}
	if n > (w.limit-w.b.Len())/len(s) {
		w.truncated = true
		return false
	}
	w.b.WriteString(strings.Repeat(s, n))
	return true
}

// printf formats and appends. It builds the whole string before the cap is
// checked, so callers whose arguments can be large check fits first.
func (w *diagnosticWriter) printf(format string, args ...any) bool {
	if w.truncated {
		return false
	}
	return w.write(fmt.Sprintf(format, args...))
}

// hexSize returns the size of n bytes written as hex digits plus extra bytes
// of surrounding text, or -1 when that overflows. fits treats -1 as too large.
func hexSize(n int, extra int) int {
	if n > (math.MaxInt-extra)/2 {
		return -1
	}
	return 2*n + extra
}

// quotedStringSize returns the number of bytes strconv.Quote will produce,
// or -1 if that size cannot be represented as an int.
func quotedStringSize(s string) int {
	ret := 2 // surrounding quotes
	for len(s) > 0 {
		r, width := utf8.DecodeRuneInString(s)
		var size int
		switch {
		case width == 1 && r == utf8.RuneError:
			size = 4 // \\xNN
		case r == '"' || r == '\\':
			size = 2
		case strconv.IsPrint(r):
			size = width
		case r == '\a' || r == '\b' || r == '\f' || r == '\n' ||
			r == '\r' || r == '\t' || r == '\v':
			size = 2
		case r < ' ' || r == 0x7f:
			size = 4 // \\xNN
		case r < 0x10000:
			size = 6 // \\uNNNN
		default:
			size = 10 // \\UNNNNNNNN
		}
		if ret > math.MaxInt-size {
			return -1
		}
		ret += size
		s = s[width:]
	}
	return ret
}

// writeHexSuffix appends "  / <hex> /" for raw without building the hex when
// it would not fit.
func (w *diagnosticWriter) writeHexSuffix(raw []byte) {
	if w.fits(hexSize(len(raw), len("  /  /"))) {
		w.printf("  / %x /", raw)
	}
}

// result returns the output, ending it with the truncation marker after sep
// when the cap was hit.
func (w *diagnosticWriter) result(sep string) string {
	if !w.truncated {
		return w.b.String()
	}
	return w.b.String() + sep + diagnosticTruncationMarker
}
