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
	"errors"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeErrorErrorMessageFull(t *testing.T) {
	e := &cbor.DecodeError{
		Message:  "invalid array length",
		Offset:   42,
		Expected: "array of length 4-8",
		Found:    "array of length 2",
	}
	assert.Equal(
		t,
		"cbor: invalid array length at offset 42: expected array of length 4-8, found array of length 2",
		e.Error(),
	)
}

func TestDecodeErrorErrorMessageWithoutFound(t *testing.T) {
	e := &cbor.DecodeError{
		Message:  "truncated input",
		Offset:   10,
		Expected: "more bytes",
	}
	assert.Equal(
		t,
		"cbor: truncated input at offset 10: expected more bytes",
		e.Error(),
	)
}

func TestDecodeErrorErrorMessageMinimal(t *testing.T) {
	e := &cbor.DecodeError{
		Message: "boom",
		Offset:  3,
	}
	assert.Equal(t, "cbor: boom at offset 3", e.Error())
}

func TestDecodeErrorUnwrap(t *testing.T) {
	cause := errors.New("inner")
	e := &cbor.DecodeError{Message: "outer", Cause: cause}
	assert.Same(t, cause, errors.Unwrap(e))
	assert.True(t, errors.Is(e, cause))
}

func TestNewDecodeErrorTruncatesRawBytes(t *testing.T) {
	raw := make([]byte, 64)
	for i := range raw {
		raw[i] = byte(i)
	}
	e := cbor.NewDecodeError(
		"oversized",
		0,
		"",
		"",
		"",
		raw,
		nil,
		nil,
	)
	require.NotNil(t, e)
	assert.LessOrEqual(t, len(e.RawBytes), 16)
	// Defensive copy: mutating the original must not affect the error.
	raw[0] = 0xff
	assert.Equal(t, byte(0), e.RawBytes[0])
}

func TestDecodeErrorWithDiagnostic(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)

	e := &cbor.DecodeError{
		Message:  "invalid array length",
		Offset:   0,
		Context:  "decoding transaction body",
		Expected: "array of length 4-8",
		Found:    "array of length 3",
	}
	out := e.WithDiagnostic(data, cbor.DiagnosticOptions{ShowOffsets: true})
	assert.Contains(t, out, "cbor: invalid array length at offset 0")
	assert.Contains(t, out, "Context: decoding transaction body")
	assert.Contains(t, out, "Expected: array of length 4-8")
	assert.Contains(t, out, "Found: array of length 3")
	assert.Contains(t, out, "Raw bytes at offset 0-4: 83 01 02 03")
	assert.Contains(t, out, "Diagnostic:")
	assert.True(
		t,
		strings.Contains(out, "[") && strings.Contains(out, "]"),
		"expected diagnostic to show array brackets, got: %s",
		out,
	)
}

func TestDecodeErrorWithDiagnosticPrefersAttachedNode(t *testing.T) {
	data, err := hex.DecodeString("83010203")
	require.NoError(t, err)
	node, err := cbor.ParseDiagnostic(data)
	require.NoError(t, err)

	e := &cbor.DecodeError{
		Message:    "boom",
		Offset:     2,
		RawBytes:   []byte{0x02},
		Diagnostic: node,
	}
	out := e.WithDiagnostic(nil, cbor.DiagnosticOptions{})
	assert.Contains(t, out, "cbor: boom at offset 2")
	assert.Contains(t, out, "Raw bytes at offset 2-3: 02")
	assert.Contains(t, out, "Diagnostic:")
}

func TestDecodeErrorWithDiagnosticHandlesUnparseableData(t *testing.T) {
	e := &cbor.DecodeError{Message: "x", Offset: 0}
	out := e.WithDiagnostic([]byte{0xff, 0xff}, cbor.DiagnosticOptions{})
	assert.Contains(t, out, "cbor: x at offset 0")
}
