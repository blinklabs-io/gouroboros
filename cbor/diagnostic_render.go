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
	"strings"
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

// write appends s and reports whether rendering may continue.
func (w *diagnosticWriter) write(s string) bool {
	if w.truncated {
		return false
	}
	if len(s) > w.limit-w.b.Len() {
		w.truncated = true
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

func (w *diagnosticWriter) printf(format string, args ...any) bool {
	if w.truncated {
		return false
	}
	return w.write(fmt.Sprintf(format, args...))
}

// result returns the output, ending it with the truncation marker after sep
// when the cap was hit.
func (w *diagnosticWriter) result(sep string) string {
	if !w.truncated {
		return w.b.String()
	}
	return w.b.String() + sep + diagnosticTruncationMarker
}
