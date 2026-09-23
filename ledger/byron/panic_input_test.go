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
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"math"

	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// nullHeaderMainBlockCbor rewrites a real Byron main block so its header is
// CBOR null, leaving the body and extra body data untouched. Splicing a known
// good block keeps every other decode check satisfied, so a decode that still
// succeeds can only have accepted the null header.
func nullHeaderMainBlockCbor(t *testing.T) []byte {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	var parts []cbor.RawMessage
	_, err = cbor.Decode(raw, &parts)
	require.NoError(t, err)
	require.Len(t, parts, 3)
	parts[0] = cbor.RawMessage{0xf6}
	spliced, err := cbor.Encode(parts)
	require.NoError(t, err)
	return spliced
}

// A Byron main block arrives from a remote peer, so a header the decoder
// leaves nil is dereferenced by every accessor on the block. The
// epoch-boundary decode already rejects this shape.
func TestByronMainBlockRejectsNullHeader(t *testing.T) {
	t.Parallel()

	data := nullHeaderMainBlockCbor(t)

	var block byron.ByronMainBlock
	err := block.UnmarshalCBOR(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")

	// The body-proof check stands in for the missing guard only while it
	// runs; a caller that skips it must still not receive a nil header.
	_, err = byron.NewByronMainBlockFromCbor(
		data,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing header")
}

// A valid block must keep decoding, so the guard cannot be satisfied by
// rejecting everything.
func TestByronMainBlockAcceptsRealHeader(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(raw)
	require.NoError(t, err)
	require.NotNil(t, block.BlockHeader)
}

func TestNewByronTransactionInputRejectsBadArguments(t *testing.T) {
	t.Parallel()

	validHash := strings.Repeat("ab", 32)

	type inputCase struct {
		name string
		hash string
		idx  int
	}
	cases := []inputCase{
		{name: "non-hex hash", hash: "not-hex", idx: 0},
		// A hash of any other length reached the slice-to-array
		// conversion, which panics rather than truncating.
		{name: "short hash", hash: strings.Repeat("ab", 31), idx: 0},
		{name: "long hash", hash: strings.Repeat("ab", 33), idx: 0},
		{name: "empty hash", hash: "", idx: 0},
		{name: "negative index", hash: validHash, idx: -1},
	}
	// math.MaxUint32+1 is not representable as an int where int is 32 bits,
	// so the case is built at run time and omitted on those GOARCHs, where
	// the bound it probes cannot be reached.
	if math.MaxInt > math.MaxUint32 {
		aboveUint32 := int64(math.MaxUint32) + 1
		cases = append(cases, inputCase{
			name: "index above uint32",
			hash: validHash,
			idx:  int(aboveUint32),
		})
	}
	for _, test := range cases {
		require.NotPanics(t, func() {
			_, err := byron.NewByronTransactionInput(test.hash, test.idx)
			require.Error(t, err, test.name)
		}, test.name)
	}

	input, err := byron.NewByronTransactionInput(validHash, 3)
	require.NoError(t, err)
	require.Equal(t, uint32(3), input.OutputIndex)
	require.Equal(t, validHash, input.Id().String())
}

// Produced builds its inputs from the already-typed transaction hash rather
// than round-tripping it through hex, so the identity it yields is asserted
// here: a UTxO identified by the wrong hash or index is a consensus fault,
// not a formatting one.
func TestByronTransactionProducedInputIdentity(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(raw)
	require.NoError(t, err)

	txs := block.Transactions()
	require.NotEmpty(t, txs)
	for _, tx := range txs {
		produced := tx.Produced()
		require.Len(t, produced, len(tx.Outputs()))
		for idx, utxo := range produced {
			require.Equal(t, tx.Hash(), utxo.Id.Id())
			require.Equal(t, uint32(idx), utxo.Id.Index())
		}
	}
}

func nonShortestOutputAmountBody(
	t *testing.T,
	body []byte,
	amount uint64,
) []byte {
	t.Helper()
	var bodyFields []cbor.RawMessage
	_, err := cbor.Decode(body, &bodyFields)
	require.NoError(t, err)
	require.Len(t, bodyFields, 3)
	var outputs []cbor.RawMessage
	_, err = cbor.Decode(bodyFields[1], &outputs)
	require.NoError(t, err)
	require.NotEmpty(t, outputs)
	var outputFields []cbor.RawMessage
	_, err = cbor.Decode(outputs[0], &outputFields)
	require.NoError(t, err)
	require.Len(t, outputFields, 2)

	encodedAmount := make([]byte, 9)
	encodedAmount[0] = 0x1b
	binary.BigEndian.PutUint64(encodedAmount[1:], amount)
	encodedOutput := append([]byte{0x82}, outputFields[0]...)
	encodedOutput = append(encodedOutput, encodedAmount...)
	outputsWire := bodyFields[1]
	firstOutput := bytes.Index(outputsWire, outputs[0])
	require.NotEqual(t, -1, firstOutput)
	updatedOutputs := make([]byte, 0, len(outputsWire)+len(encodedOutput))
	updatedOutputs = append(updatedOutputs, outputsWire[:firstOutput]...)
	updatedOutputs = append(updatedOutputs, encodedOutput...)
	updatedOutputs = append(
		updatedOutputs,
		outputsWire[firstOutput+len(outputs[0]):]...,
	)
	encodedBody := []byte{0x83}
	for i, part := range bodyFields {
		if i == 1 {
			part = updatedOutputs
		}
		encodedBody = append(encodedBody, part...)
	}
	return encodedBody
}

func TestByronTransactionProducedInputUsesReferenceId(t *testing.T) {
	t.Parallel()

	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	require.NoError(t, err)
	block, err := byron.NewByronMainBlockFromCbor(raw)
	require.NoError(t, err)
	txs := block.Transactions()
	require.NotEmpty(t, txs)

	byronTx, ok := txs[0].(*byron.ByronTransaction)
	require.True(t, ok)
	originalBody := byronTx.Body.Cbor()
	require.NotEmpty(t, originalBody)
	require.Equal(t, originalBody, referenceBodyCBOR(t, &byronTx.Body))
	nonShortestBody := nonShortestOutputAmountBody(
		t,
		originalBody,
		1,
	)
	var body byron.ByronTransactionBody
	_, err = cbor.Decode(nonShortestBody, &body)
	require.NoError(t, err)

	tx := byron.ByronTransaction{Body: body}
	produced := tx.Produced()
	require.NotEmpty(t, produced)
	referenceId := tx.Id()
	wireId := tx.Body.WireHash()
	require.NotEqual(t, referenceId, wireId)
	canonicalWire := referenceBodyCBOR(t, &tx.Body)
	var canonicalBody byron.ByronTransactionBody
	_, err = cbor.Decode(canonicalWire, &canonicalBody)
	require.NoError(t, err)
	canonicalTx := byron.ByronTransaction{Body: canonicalBody}
	require.Equal(t, referenceId, canonicalTx.Id())
	require.Equal(t, referenceId, canonicalTx.Body.WireHash())
	require.Equal(t, referenceId, produced[0].Id.Id())

	sameBlockSpend := byron.ByronTransactionInput{
		TxId:        produced[0].Id.Id(),
		OutputIndex: produced[0].Id.Index(),
	}
	require.Equal(t, referenceId, sameBlockSpend.Id())
	require.NotEqual(t, wireId, sameBlockSpend.Id())
}
