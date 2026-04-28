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

package localtxsubmission

import (
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// goldenWrongEraByron mirrors the golden bytes captured from
// IntersectMBO/ouroboros-consensus golden test corpus
// (ApplyTxErr_WrongEraByron) — applying a Shelley tx to a Byron ledger.
const goldenWrongEraByron = "828201675368656c6c657982006542" +
	"79726f6e"

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	require.NoError(t, err)
	return b
}

// TestEncodeRejectReason_TypedEraMismatch verifies that an *EraMismatch
// returned from a SubmitTxFunc is wire-encoded as canonical CBOR (matching
// the Haskell golden bytes) rather than its Error() string.
func TestEncodeRejectReason_TypedEraMismatch(t *testing.T) {
	em := &ledger.EraMismatch{
		OtherEra:  ledger.EraInfo{Index: 1, Name: "Shelley"},
		LedgerEra: ledger.EraInfo{Index: 0, Name: "Byron"},
	}
	got, err := encodeRejectReason(em)
	require.NoError(t, err)
	assert.Equal(t, mustHex(t, goldenWrongEraByron), got,
		"typed *EraMismatch must be encoded as canonical Haskell wire bytes")
}

// TestEncodeRejectReason_StringFallback verifies that a plain error is
// CBOR-encoded as a text string — preserving the existing pre-typed
// behaviour so untyped callbacks remain wire-compatible.
func TestEncodeRejectReason_StringFallback(t *testing.T) {
	plainErr := errors.New("something went wrong")
	got, err := encodeRejectReason(plainErr)
	require.NoError(t, err)

	// Decode and check it's a CBOR text string equal to err.Error().
	var decoded string
	_, err = cbor.Decode(got, &decoded)
	require.NoError(t, err)
	assert.Equal(t, "something went wrong", decoded)
}

// TestEncodeRejectReason_WrappedTypedError verifies that errors wrapped
// via fmt.Errorf("%w", ...) still get the typed CBOR encoding via
// errors.As. Without this, callers would have to be careful never to
// wrap typed reject reasons.
func TestEncodeRejectReason_WrappedTypedError(t *testing.T) {
	em := &ledger.EraMismatch{
		OtherEra:  ledger.EraInfo{Index: 1, Name: "Shelley"},
		LedgerEra: ledger.EraInfo{Index: 0, Name: "Byron"},
	}
	wrapped := fmt.Errorf("submission failed: %w", em)
	got, err := encodeRejectReason(wrapped)
	require.NoError(t, err)
	assert.Equal(t, mustHex(t, goldenWrongEraByron), got,
		"wrapped *EraMismatch must still be encoded as canonical CBOR")
}

// TestEncodeRejectReason_RoundTripThroughClientDecoder verifies that the
// bytes produced by encodeRejectReason can be parsed by the client-side
// ledger.NewTxSubmitErrorFromCbor dispatcher, closing the wire-protocol
// loop. Today that loop is broken: the server emits string CBOR for
// every error, so the client always falls through to GenericError.
func TestEncodeRejectReason_RoundTripThroughClientDecoder(t *testing.T) {
	original := &ledger.EraMismatch{
		OtherEra:  ledger.EraInfo{Index: 6, Name: "Conway"},
		LedgerEra: ledger.EraInfo{Index: 5, Name: "Babbage"},
	}
	wireBytes, err := encodeRejectReason(original)
	require.NoError(t, err)

	clientErr, err := ledger.NewTxSubmitErrorFromCbor(wireBytes)
	require.NoError(t, err)
	var em *ledger.EraMismatch
	require.True(t, errors.As(clientErr, &em),
		"client-side decoder must recognise typed reject reason")
	assert.Equal(t, "Conway", em.OtherEra.Name)
	assert.Equal(t, "Babbage", em.LedgerEra.Name)
}
