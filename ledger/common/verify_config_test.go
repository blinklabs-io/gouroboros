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
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationErrorErrorIncludesCborContext(t *testing.T) {
	e := &common.ValidationError{
		Type:        common.ValidationErrorTypeBodyHash,
		Message:     "hash mismatch",
		ByteOffset:  128,
		CborContext: "block/body/tx[3]",
		Diagnostic:  "[...]",
	}
	got := e.Error()
	assert.Contains(t, got, "body_hash")
	assert.Contains(t, got, "hash mismatch")
	assert.Contains(t, got, "block/body/tx[3]")
	assert.Contains(t, got, "@offset 128")
}

func TestValidationErrorErrorOffsetOnly(t *testing.T) {
	e := &common.ValidationError{
		Type:       common.ValidationErrorTypeTransaction,
		Message:    "decode failed",
		ByteOffset: 7,
		Cause:      errors.New("inner"),
	}
	got := e.Error()
	assert.Contains(t, got, "@offset 7")
	assert.Contains(t, got, "(inner)")
}

func TestValidationErrorErrorWithoutCborFieldsUnchanged(t *testing.T) {
	e := &common.ValidationError{
		Type:    common.ValidationErrorTypeProtocol,
		Message: "bad",
	}
	assert.Equal(t, "protocol: bad", e.Error())
}

func TestValidationErrorWithDiagnostic(t *testing.T) {
	diag := "[\n  0,\n  1\n]"
	e := &common.ValidationError{
		Type:        common.ValidationErrorTypeBodyHash,
		Message:     "hash mismatch",
		ByteOffset:  128,
		CborContext: "block/body/tx[3]",
		Diagnostic:  diag,
	}
	got := e.WithDiagnostic()
	// Single-line summary remains intact.
	assert.Contains(t, got, "body_hash")
	assert.Contains(t, got, "hash mismatch")
	assert.Contains(t, got, "block/body/tx[3]")
	assert.Contains(t, got, "@offset 128")
	// Diagnostic block must follow.
	assert.Contains(t, got, "\n\nDiagnostic:\n")
	assert.Contains(t, got, diag)
}

func TestValidationErrorWithDiagnosticEmpty(t *testing.T) {
	e := &common.ValidationError{
		Type:    common.ValidationErrorTypeProtocol,
		Message: "bad",
	}
	// No Diagnostic stored — WithDiagnostic must equal Error().
	assert.Equal(t, e.Error(), e.WithDiagnostic())
}

// TestBlockBodySizeFromCbor verifies the body size is the sum of the raw
// CBOR lengths of every top-level block array element after the header,
// covering both the pre-Dijkstra (5-element) and Dijkstra (2-element)
// block shapes.
func TestBlockBodySizeFromCbor(t *testing.T) {
	t.Run("pre-Dijkstra 5-element block", func(t *testing.T) {
		header := []byte{0x43, 0x01, 0x02, 0x03} // bstr(3): header placeholder
		txBodies := []byte{0x80}                 // empty array
		txWitnesses := []byte{0x80}              // empty array
		auxData := []byte{0xa0}                  // empty map
		invalidTxs := []byte{0x81, 0x00}         // [0]
		blockCbor, err := cbor.Encode([]cbor.RawMessage{
			header,
			txBodies,
			txWitnesses,
			auxData,
			invalidTxs,
		})
		require.NoError(t, err)

		size, err := common.BlockBodySizeFromCbor(blockCbor)
		require.NoError(t, err)
		expected := uint64(
			len(txBodies) + len(txWitnesses) + len(auxData) + len(invalidTxs),
		)
		assert.Equal(t, expected, size)
	})

	t.Run("Dijkstra 2-element block", func(t *testing.T) {
		header := []byte{0x43, 0x01, 0x02, 0x03} // bstr(3): header placeholder
		body := []byte{0x82, 0x01, 0x02}         // [1, 2]: body placeholder
		blockCbor, err := cbor.Encode(
			[]cbor.RawMessage{header, body},
		)
		require.NoError(t, err)

		size, err := common.BlockBodySizeFromCbor(blockCbor)
		require.NoError(t, err)
		assert.Equal(t, uint64(len(body)), size)
	})

	t.Run("malformed CBOR returns error", func(t *testing.T) {
		_, err := common.BlockBodySizeFromCbor([]byte{0xff})
		assert.Error(t, err)
	})

	t.Run("too few elements returns error", func(t *testing.T) {
		blockCbor, err := cbor.Encode([]cbor.RawMessage{{0x01}})
		require.NoError(t, err)
		_, err = common.BlockBodySizeFromCbor(blockCbor)
		assert.Error(t, err)
	})
}
