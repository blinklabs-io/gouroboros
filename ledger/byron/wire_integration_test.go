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

package byron_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/stretchr/testify/require"
)

func encodeIndefiniteArray(fields []cbor.RawMessage) []byte {
	encoded := []byte{0x9f}
	for _, field := range fields {
		encoded = append(encoded, field...)
	}
	return append(encoded, 0xff)
}

func TestByronSignedHeaderRejectsArrayEncodedVerificationKey(t *testing.T) {
	blockBytes, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	var block []cbor.RawMessage
	_, err = cbor.Decode(blockBytes, &block)
	require.NoError(t, err)
	if block == nil {
		t.Fatal("expected Byron block fields")
	}
	var header []cbor.RawMessage
	_, err = cbor.Decode(block[0], &header)
	require.NoError(t, err)
	if header == nil {
		t.Fatal("expected Byron header fields")
	}
	var consensus []cbor.RawMessage
	_, err = cbor.Decode(header[3], &consensus)
	require.NoError(t, err)
	if consensus == nil {
		t.Fatal("expected Byron consensus fields")
	}
	var key []byte
	_, err = cbor.Decode(consensus[1], &key)
	require.NoError(t, err)
	keyArray := make([]any, len(key))
	for i, value := range key {
		keyArray[i] = uint64(value)
	}
	consensus[1], err = cbor.Encode(keyArray)
	require.NoError(t, err)
	header[3], err = cbor.Encode(consensus)
	require.NoError(t, err)
	block[0], err = cbor.Encode(header)
	require.NoError(t, err)
	mutatedBlock, err := cbor.Encode(block)
	require.NoError(t, err)

	_, err = byron.NewByronMainBlockFromCbor(mutatedBlock)
	require.Error(t, err)
	require.ErrorContains(t, err, "canonical CBOR byte string")
}

func TestByronSignedHeaderRejectsIndefiniteFixedRecords(t *testing.T) {
	blockBytes, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	var block []cbor.RawMessage
	_, err = cbor.Decode(blockBytes, &block)
	require.NoError(t, err)
	if block == nil {
		t.Fatal("expected Byron block fields")
	}
	var header []cbor.RawMessage
	_, err = cbor.Decode(block[0], &header)
	require.NoError(t, err)
	if header == nil {
		t.Fatal("expected Byron header fields")
	}
	var consensus []cbor.RawMessage
	_, err = cbor.Decode(header[3], &consensus)
	require.NoError(t, err)
	if consensus == nil {
		t.Fatal("expected Byron consensus fields")
	}
	var difficulty []cbor.RawMessage
	_, err = cbor.Decode(consensus[2], &difficulty)
	require.NoError(t, err)
	if difficulty == nil {
		t.Fatal("expected chain difficulty fields")
	}
	var bodyProof []cbor.RawMessage
	_, err = cbor.Decode(header[2], &bodyProof)
	require.NoError(t, err)
	if bodyProof == nil {
		t.Fatal("expected Byron body proof fields")
	}

	tests := []struct {
		name   string
		mutate func(t *testing.T, header, consensus, difficulty, proof []cbor.RawMessage)
	}{
		{
			name:   "main header",
			mutate: func(t *testing.T, _, _, _, _ []cbor.RawMessage) {},
		},
		{
			name: "consensus data",
			mutate: func(t *testing.T, header, consensus, _, _ []cbor.RawMessage) {
				header[3] = encodeIndefiniteArray(consensus)
			},
		},
		{
			name: "chain difficulty",
			mutate: func(t *testing.T, header, consensus, difficulty, _ []cbor.RawMessage) {
				consensus[2] = encodeIndefiniteArray(difficulty)
				header[3], err = cbor.Encode(consensus)
				require.NoError(t, err)
			},
		},
		{
			name: "body proof",
			mutate: func(t *testing.T, header, _, _, proof []cbor.RawMessage) {
				header[2] = encodeIndefiniteArray(proof)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blockFields := append([]cbor.RawMessage(nil), block...)
			headerFields := append([]cbor.RawMessage(nil), header...)
			consensusFields := append([]cbor.RawMessage(nil), consensus...)
			difficultyFields := append([]cbor.RawMessage(nil), difficulty...)
			proofFields := append([]cbor.RawMessage(nil), bodyProof...)
			tc.mutate(t, headerFields, consensusFields, difficultyFields, proofFields)
			if tc.name == "main header" {
				blockFields[0] = encodeIndefiniteArray(headerFields)
			} else {
				blockFields[0], err = cbor.Encode(headerFields)
				require.NoError(t, err)
			}
			mutated, err := cbor.Encode(blockFields)
			require.NoError(t, err)
			_, err = byron.NewByronMainBlockFromCbor(mutated)
			require.Error(t, err)
		})
	}
}

func TestByronMainHeaderRejectsIndefiniteStrings(t *testing.T) {
	blockBytes, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	var block []cbor.RawMessage
	_, err = cbor.Decode(blockBytes, &block)
	require.NoError(t, err)
	if block == nil {
		t.Fatal("expected Byron block fields")
	}
	var header []cbor.RawMessage
	_, err = cbor.Decode(block[0], &header)
	require.NoError(t, err)
	if header == nil {
		t.Fatal("expected Byron header fields")
	}
	var extraData []cbor.RawMessage
	_, err = cbor.Decode(header[4], &extraData)
	require.NoError(t, err)
	if extraData == nil {
		t.Fatal("expected Byron extra data fields")
	}
	var extraProof []byte
	_, err = cbor.Decode(extraData[3], &extraProof)
	require.NoError(t, err)
	var softwareVersion []cbor.RawMessage
	_, err = cbor.Decode(extraData[1], &softwareVersion)
	require.NoError(t, err)
	if softwareVersion == nil {
		t.Fatal("expected Byron software version fields")
	}
	var appName string
	_, err = cbor.Decode(softwareVersion[0], &appName)
	require.NoError(t, err)

	tests := []struct {
		name      string
		mutate    func(t *testing.T, extra, software []cbor.RawMessage)
		wantError string
	}{
		{
			name: "extra proof",
			mutate: func(t *testing.T, extra, _ []cbor.RawMessage) {
				encoded, err := cbor.Encode(cbor.IndefLengthByteString{extraProof})
				require.NoError(t, err)
				extra[3] = encoded
			},
			wantError: "indefinite-length CBOR string",
		},
		{
			name: "application name",
			mutate: func(t *testing.T, _, software []cbor.RawMessage) {
				first, err := cbor.Encode(appName[:1])
				require.NoError(t, err)
				second, err := cbor.Encode(appName[1:])
				require.NoError(t, err)
				encoded := append([]byte{0x7f}, first...)
				encoded = append(encoded, second...)
				encoded = append(encoded, 0xff)
				software[0] = encoded
			},
			wantError: "indefinite-length CBOR string",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			headerFields := append([]cbor.RawMessage(nil), header...)
			extraFields := append([]cbor.RawMessage(nil), extraData...)
			softwareFields := append([]cbor.RawMessage(nil), softwareVersion...)
			tc.mutate(t, extraFields, softwareFields)
			if tc.name == "application name" {
				var err error
				extraFields[1], err = cbor.Encode(softwareFields)
				require.NoError(t, err)
			}
			headerFields[4], err = cbor.Encode(extraFields)
			require.NoError(t, err)
			blockFields := append([]cbor.RawMessage(nil), block...)
			blockFields[0], err = cbor.Encode(headerFields)
			require.NoError(t, err)
			mutated, err := cbor.Encode(blockFields)
			require.NoError(t, err)
			_, err = byron.NewByronMainBlockFromCbor(mutated)
			require.Error(t, err)
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}
