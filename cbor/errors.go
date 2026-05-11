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
	"strings"
)

// maxRawBytesPreview limits how many problematic bytes are captured in a
// DecodeError to keep error messages bounded.
const maxRawBytesPreview = 16

// DecodeError represents a CBOR decoding error with byte position and
// diagnostic context.
type DecodeError struct {
	Message    string
	Offset     int
	Context    string
	Expected   string
	Found      string
	RawBytes   []byte
	Diagnostic *DiagnosticNode
	Cause      error
}

// NewDecodeError constructs a DecodeError, copying RawBytes defensively and
// truncating to maxRawBytesPreview.
func NewDecodeError(
	message string,
	offset int,
	context, expected, found string,
	rawBytes []byte,
	diagnostic *DiagnosticNode,
	cause error,
) *DecodeError {
	var raw []byte
	if len(rawBytes) > 0 {
		n := len(rawBytes)
		if n > maxRawBytesPreview {
			n = maxRawBytesPreview
		}
		raw = make([]byte, n)
		copy(raw, rawBytes[:n])
	}
	return &DecodeError{
		Message:    message,
		Offset:     offset,
		Context:    context,
		Expected:   expected,
		Found:      found,
		RawBytes:   raw,
		Diagnostic: diagnostic,
		Cause:      cause,
	}
}

// Error returns a single-line summary of the decode error.
func (e *DecodeError) Error() string {
	if e == nil {
		return ""
	}
	switch {
	case e.Expected != "" && e.Found != "":
		return fmt.Sprintf(
			"cbor: %s at offset %d: expected %s, found %s",
			e.Message,
			e.Offset,
			e.Expected,
			e.Found,
		)
	case e.Expected != "":
		return fmt.Sprintf(
			"cbor: %s at offset %d: expected %s",
			e.Message,
			e.Offset,
			e.Expected,
		)
	default:
		return fmt.Sprintf(
			"cbor: %s at offset %d",
			e.Message,
			e.Offset,
		)
	}
}

// Unwrap returns the underlying cause, if any.
func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// WithDiagnostic returns a multi-line error message with diagnostic context.
// If data is non-nil and the error has no Diagnostic node attached, the data
// is parsed to produce one. opts is forwarded to FormatDiagnosticPretty.
func (e *DecodeError) WithDiagnostic(data []byte, opts DiagnosticOptions) string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(e.Error())
	if e.Context != "" {
		b.WriteString("\nContext: ")
		b.WriteString(e.Context)
	}
	if e.Expected != "" {
		b.WriteString("\nExpected: ")
		b.WriteString(e.Expected)
	}
	if e.Found != "" {
		b.WriteString("\nFound: ")
		b.WriteString(e.Found)
	}
	raw := e.RawBytes
	if len(raw) == 0 && len(data) > 0 && e.Offset >= 0 && e.Offset < len(data) {
		end := e.Offset + maxRawBytesPreview
		if end > len(data) {
			end = len(data)
		}
		raw = data[e.Offset:end]
	}
	if len(raw) > 0 {
		fmt.Fprintf(
			&b,
			"\nRaw bytes at offset %d-%d: %s",
			e.Offset,
			e.Offset+len(raw),
			formatHexBytes(raw, 0),
		)
	}
	diag := e.Diagnostic
	if diag == nil && len(data) > 0 {
		if parsed, perr := ParseDiagnostic(data); perr == nil {
			diag = parsed
		}
	}
	if diag != nil {
		b.WriteString("\n\nDiagnostic:\n")
		b.WriteString(diag.FormatDiagnosticPretty(opts))
	}
	return b.String()
}
