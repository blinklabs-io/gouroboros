// Copyright 2025 Blink Labs Software
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

package shelley_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
)

func TestShelleyTransactionInputToPlutusData(t *testing.T) {
	testTxIdHex := "1639f61ed08f5e489dd64db20f86451a0db06e83d21ea39c73ea0a93b478a370"
	testTxOutputIdx := 2
	testInput := shelley.NewShelleyTransactionInput(
		testTxIdHex,
		testTxOutputIdx,
	)
	testTxId, err := hex.DecodeString(testTxIdHex)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expectedData := data.NewConstr(
		0,
		data.NewByteString(testTxId),
		data.NewInteger(big.NewInt(int64(testTxOutputIdx))),
	)
	tmpData := testInput.ToPlutusData()
	if !reflect.DeepEqual(tmpData, expectedData) {
		t.Fatalf(
			"did not get expected PlutusData\n     got: %#v\n  wanted: %#v",
			tmpData,
			expectedData,
		)
	}
}

// TestShelleyTransactionBody_MarshalCBOR_PreservesWireBytes reproduces
// https://github.com/blinklabs-io/gouroboros/issues/1990: a decoded
// ShelleyTransactionBody carrying a protocol parameter update must
// re-marshal to its preserved wire bytes instead of falling through to
// the generic encoder, which would emit an explicit CBOR null for each
// of the 15 unset ShelleyProtocolParameterUpdate fields.
func TestShelleyTransactionBody_MarshalCBOR_PreservesWireBytes(t *testing.T) {
	genesisHash := common.NewBlake2b224(bytes.Repeat([]byte{0xAB}, 28))
	// Minimal wire-format body: a fee, a TTL, and a protocol param
	// update proposal with only MinFeeA (key 0) populated, as a real
	// node would emit it.
	rawBody := map[uint64]any{
		2: uint64(206245),  // fee
		3: uint64(5000000), // ttl
		6: []any{
			map[common.Blake2b224]any{
				genesisHash: map[uint64]any{0: uint64(44)}, // MinFeeA only
			},
			uint64(4), // epoch
		},
	}
	wireBytes, err := cbor.Encode(rawBody)
	require.NoError(t, err, "unexpected error encoding wire bytes")

	body, err := shelley.NewShelleyTransactionBodyFromCbor(wireBytes)
	require.NoError(t, err, "unexpected error decoding transaction body")
	require.NotNil(t, body.Update, "expected a protocol param update")
	require.Len(
		t,
		body.Update.ProtocolParamUpdates,
		1,
		"expected a single protocol param update",
	)
	update, ok := body.Update.ProtocolParamUpdates[genesisHash]
	require.True(t, ok, "expected update entry for genesis hash %s", genesisHash)
	require.NotNil(t, update.MinFeeA, "expected MinFeeA to be set")
	assert.Equal(t, uint(44), *update.MinFeeA)
	assert.Nil(t, update.MinFeeB, "expected MinFeeB to remain unset")

	marshaled, err := body.MarshalCBOR()
	require.NoError(t, err, "unexpected error marshaling body")
	assert.Equal(
		t,
		wireBytes,
		marshaled,
		"MarshalCBOR() did not return preserved wire bytes",
	)

	// Rebuilding a transaction from the decoded body and witness set --
	// the same pattern ShelleyBlock.Transactions() uses -- must not
	// inflate the encoded transaction size. The witness set fixture uses
	// an indefinite-length VkeyWitnesses array, whose generic
	// (re-)encoding as a Go slice can never reproduce the indefinite-
	// length wire form, so this exercises
	// ShelleyTransactionWitnessSet.MarshalCBOR as well as the body.
	wsBytes, witnessSet := newIndefLengthWitnessSetFixture(t)

	wireTxBytes, err := cbor.Encode(
		[]any{cbor.RawMessage(wireBytes), cbor.RawMessage(wsBytes), nil},
	)
	require.NoError(t, err, "unexpected error encoding wire transaction")

	rebuilt := &shelley.ShelleyTransaction{
		Body:       *body,
		WitnessSet: witnessSet,
	}
	rebuiltBytes := rebuilt.Cbor()
	assert.Equal(
		t,
		wireTxBytes,
		rebuiltBytes,
		"rebuilt transaction CBOR does not match wire bytes",
	)
}

// newIndefLengthWitnessSetFixture builds wire bytes for a
// ShelleyTransactionWitnessSet whose sole VkeyWitnesses entry is encoded
// as an indefinite-length CBOR array, then decodes it. Re-encoding a
// decoded []common.VkeyWitness generically always produces a
// definite-length array, so this fixture's generic encoding necessarily
// differs from its wire bytes -- making it load-bearing for verifying
// that ShelleyTransactionWitnessSet.MarshalCBOR prefers preserved bytes.
func newIndefLengthWitnessSetFixture(
	t *testing.T,
) ([]byte, shelley.ShelleyTransactionWitnessSet) {
	t.Helper()

	vkey := bytes.Repeat([]byte{0x01}, 32)
	sig := bytes.Repeat([]byte{0x02}, 64)
	wsBytes, err := cbor.Encode(map[uint64]any{
		0: cbor.IndefLengthList{[]any{vkey, sig}},
	})
	require.NoError(t, err, "unexpected error encoding witness set")

	var witnessSet shelley.ShelleyTransactionWitnessSet
	_, err = cbor.Decode(wsBytes, &witnessSet)
	require.NoError(t, err, "unexpected error decoding witness set")
	require.Len(t, witnessSet.VkeyWitnesses, 1)
	assert.Equal(t, vkey, witnessSet.VkeyWitnesses[0].Vkey)
	assert.Equal(t, sig, witnessSet.VkeyWitnesses[0].Signature)

	// Sanity check: confirm the fixture actually exercises the preserved-
	// bytes path by verifying generic encoding cannot reproduce it.
	regenerated, err := cbor.EncodeGeneric(&witnessSet)
	require.NoError(t, err, "unexpected error generically encoding witness set")
	assert.NotEqual(
		t,
		wsBytes,
		regenerated,
		"fixture's generic encoding must differ from its indefinite-length wire bytes",
	)

	return wsBytes, witnessSet
}
