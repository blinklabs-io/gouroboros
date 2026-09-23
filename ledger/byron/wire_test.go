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

package byron

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func encodeRawArray(t *testing.T, fields ...cbor.RawMessage) []byte {
	t.Helper()
	encoded, err := cbor.Encode(fields)
	require.NoError(t, err)
	return encoded
}

func TestByronTransactionAttributesRequireReferenceMapShape(t *testing.T) {
	tests := []struct {
		name       string
		attributes string
		wantError  bool
	}{
		{name: "sorted Word8 map of bytes", attributes: "a20041000140"},
		{name: "non-shortest map length preserved", attributes: "b801004100"},
		{name: "not a map", attributes: "00", wantError: true},
		{name: "indefinite map", attributes: "bf004100ff", wantError: true},
		{name: "out of order keys", attributes: "a201400040", wantError: true},
		{name: "duplicate keys", attributes: "a200400040", wantError: true},
		{name: "key outside Word8", attributes: "a119010040", wantError: true},
		{name: "non-byte value", attributes: "a10000", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			attributes, err := hex.DecodeString(tc.attributes)
			require.NoError(t, err)
			body := encodeRawArray(t,
				cbor.RawMessage{0x80}, cbor.RawMessage{0x80}, attributes,
			)
			var decoded ByronTransactionBody
			err = decoded.UnmarshalCBOR(body)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, attributes, []byte(decoded.Attributes))
			require.True(t, bytes.Equal(body, decoded.Cbor()))
			require.Equal(t, common.Blake2b256Hash(body), decoded.Id())
		})
	}
}

func TestByronVerificationKeyRequiresCanonicalByteString(t *testing.T) {
	key := make([]byte, VerificationKeySize)
	canonical, err := cbor.Encode(key)
	require.NoError(t, err)
	require.NoError(t, requireByronVerificationKey(canonical, "key"))

	array := make([]any, VerificationKeySize)
	for i := range array {
		array[i] = uint64(0)
	}
	arrayEncoding, err := cbor.Encode(array)
	require.NoError(t, err)
	require.Error(t, requireByronVerificationKey(arrayEncoding, "key"))

	nonShortest := append([]byte{0x59, 0x00, VerificationKeySize}, key...)
	require.Error(t, requireByronVerificationKey(nonShortest, "key"))
}

func TestByronTransactionPayloadRequiresIndefiniteList(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire cbor.RawMessage
		bad  bool
	}{
		{name: "definite empty list", wire: cbor.RawMessage{0x80}, bad: true},
		{name: "indefinite empty list", wire: cbor.RawMessage{0x9f, 0xff}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := encodeRawArray(t,
				tc.wire,
				cbor.RawMessage{0x80},
				cbor.RawMessage{0x9f, 0xff},
				cbor.RawMessage{0x82, 0x80, 0x9f, 0xff},
			)
			var decoded ByronMainBlockBody
			err := decoded.UnmarshalCBOR(body)
			if tc.bad {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestByronArrayFieldsRejectIndefiniteAndWrongArity(t *testing.T) {
	_, err := byronArrayFields([]byte{0x9f, 0x01, 0xff}, "record")
	require.Error(t, err)
	_, err = byronArrayFields([]byte{0x82, 0x00, 0x01, 0x02}, "record")
	require.Error(t, err)
	fields, err := byronArrayFields([]byte{0x82, 0x00, 0x01}, "record")
	require.NoError(t, err)
	require.Len(t, fields, 2)
}

func TestByronFixedRecordsRejectIndefiniteArrays(t *testing.T) {
	var body ByronTransactionBody
	require.Error(t, body.UnmarshalCBOR([]byte{0x9f, 0x80, 0x80, 0xa0, 0xff}))

	var transaction ByronTransaction
	require.Error(t, transaction.UnmarshalCBOR([]byte{0x9f, 0x80, 0x80, 0xff}))

	var output ByronTransactionOutput
	require.Error(t, output.UnmarshalCBOR([]byte{0x9f, 0x40, 0x00, 0xff}))

	var update ByronUpdateProposalBlockVersionMod
	require.Error(t, update.UnmarshalCBOR([]byte{0x9f, 0xff}))
}

func TestByronProtocolParameterUpdateRejectsIndefiniteStrings(t *testing.T) {
	fields := make([]cbor.RawMessage, 14)
	for index := range fields {
		fields[index] = cbor.RawMessage{0x80}
	}
	fields[0] = cbor.RawMessage{0x7f, 0x61, 'a', 0xff}
	var update ByronUpdateProposalBlockVersionMod
	err := update.UnmarshalCBOR(encodeRawArray(t, fields...))
	require.ErrorContains(t, err, "indefinite-length CBOR string")
}

func TestByronOrdinaryStringsRequireDefiniteFraming(t *testing.T) {
	require.NoError(t, requireByronByteString([]byte{0x41, 0x00}, "bytes"))
	require.Error(t, requireByronByteString([]byte{0x5f, 0x41, 0x00, 0xff}, "bytes"))
	require.Error(t, requireByronByteString([]byte{0x41, 0x00, 0x00}, "bytes"))
	require.NoError(t, requireByronTextString([]byte{0x61, 'a'}, "text"))
	require.Error(t, requireByronTextString([]byte{0x7f, 0x61, 'a', 0xff}, "text"))
}

func TestByronItemScannerSkipsSimpleValuesAndFloats(t *testing.T) {
	for _, raw := range [][]byte{
		{0x81, 0xf8, 0x20},
		{0x81, 0xf9, 0x3c, 0x00},
		{0x81, 0xfa, 0x3f, 0x80, 0x00, 0x00},
		{0x81, 0xfb, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	} {
		require.NoError(t, validateByronDefiniteStrings(raw))
	}
}
