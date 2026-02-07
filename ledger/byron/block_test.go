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

package byron_test

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
)

func TestByronBlock_CborRoundTrip_UsingCborEncode(t *testing.T) {
	hexStr := strings.TrimSpace(testdata.ByronBlockHex)

	// Decode the hex string into CBOR bytes
	dataBytes, err := hex.DecodeString(hexStr)
	if err != nil {
		t.Fatalf(
			"Failed to decode Byron block hex string into CBOR bytes: %v",
			err,
		)
	}

	// Deserialize CBOR bytes into ByronMainBlock struct
	var block byron.ByronMainBlock
	err = block.UnmarshalCBOR(dataBytes)
	if err != nil {
		t.Fatalf("Failed to unmarshal CBOR data into ByronBlock: %v", err)
	}
	// Reset stored CBOR to nil
	block.SetCbor(nil)

	// Re-encode using the cbor Encode function
	encoded, err := cbor.Encode(block)
	if err != nil {
		t.Fatalf(
			"Failed to marshal ByronBlock using custom encode function: %v",
			err,
		)
	}
	if len(encoded) == 0 {
		t.Fatal("Custom encoded CBOR from ByronBlock is nil or empty")
	}

	// Ensure the original and re-encoded CBOR bytes are identical
	if !bytes.Equal(dataBytes, encoded) {
		t.Errorf(
			"Custom CBOR round-trip mismatch for Byron block\nOriginal CBOR (hex): %x\nCustom Encoded CBOR (hex): %x",
			dataBytes,
			encoded,
		)

		// Check from which byte it differs
		diffIndex := -1
		for i := 0; i < len(dataBytes) && i < len(encoded); i++ {
			if dataBytes[i] != encoded[i] {
				diffIndex = i
				break
			}
		}
		if diffIndex != -1 {
			t.Logf("First mismatch at byte index: %d", diffIndex)
			t.Logf(
				"Original byte: 0x%02x, Re-encoded byte: 0x%02x",
				dataBytes[diffIndex],
				encoded[diffIndex],
			)
		} else {
			t.Logf("Length mismatch: original length = %d, re-encoded length = %d", len(dataBytes), len(encoded))
		}
	}
}

func TestByronTransaction_Utxorpc(t *testing.T) {
	// Decode the hex string to bytes
	blockBytes, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	assert.NoError(t, err)

	block, err := byron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	assert.NoError(t, err)
	assert.NotNil(t, block)

	txs := block.Transactions()
	assert.Greater(
		t,
		len(txs),
		0,
		"Expected at least one transaction in the block",
	)

	for _, tx := range txs {
		byronTx, ok := tx.(*byron.ByronTransaction)
		assert.True(t, ok, "Transaction should be of type *ByronTransaction")

		assert.NotNil(
			t,
			byronTx.Twit,
			"Byron witness list (Twit) should not be nil",
		)
		assert.Greater(
			t,
			len(byronTx.Twit),
			0,
			"Byron witness list (Twit) should not be empty",
		)
		assert.NotNil(
			t,
			byronTx.Witnesses(),
			"Witnesses() should not return nil",
		)

		utxoTx, err := byronTx.Utxorpc()
		assert.NoError(t, err)
		assert.NotNil(t, utxoTx, "Utxorpc() should not return nil")

		txHash := byronTx.Hash()
		assert.NotEmpty(
			t,
			txHash.String(),
			"Transaction hash should not be empty",
		)

		inputs := byronTx.Inputs()
		assert.Equal(
			t,
			len(inputs),
			len(byronTx.Consumed()),
			"Consumed inputs should match Inputs() length",
		)

		produced := byronTx.Produced()
		assert.Equal(
			t,
			len(produced),
			len(byronTx.Outputs()),
			"Produced should match Outputs() length",
		)

		for _, utxo := range produced {
			rpcOut, err := utxo.Output.Utxorpc()
			assert.NoError(t, err)
			assert.NotNil(t, rpcOut, "Utxorpc output should not be nil")
			coin := rpcOut.Coin
			assert.NotNil(t, coin, "Coin should not be nil")
			if coin.GetInt() != 0 {
				assert.Greater(
					t,
					coin.GetInt(),
					int64(0),
					"Coin amount should be greater than 0",
				)
			} else {
				assert.Greater(
					t,
					new(big.Int).SetBytes(coin.GetBigUInt()).Cmp(big.NewInt(0)),
					0,
					"Coin amount should be greater than 0",
				)
			}
			assert.NotEmpty(
				t,
				rpcOut.Address,
				"Address bytes should not be empty",
			)
		}
	}
}

func BenchmarkByronBlockDeserialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		var block byron.ByronMainBlock
		err := block.UnmarshalCBOR(blockCbor)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkByronBlockSerialization(b *testing.B) {
	blockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ByronBlockHex))
	if err != nil {
		b.Fatal(err)
	}
	var block byron.ByronMainBlock
	err = block.UnmarshalCBOR(blockCbor)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		_ = block.Cbor()
	}
}
