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

package conway

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConwayRedeemersIter(t *testing.T) {
	testRedeemers := ConwayRedeemers{
		Redeemers: map[common.RedeemerKey]common.RedeemerValue{
			{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			}: {
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
			{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			}: {
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
			{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			}: {
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
	}
	expectedOrder := []struct {
		Key   common.RedeemerKey
		Value common.RedeemerValue
	}{
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagSpend,
				Index: 4,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 0,
					Steps:  4444,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 0,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  0,
				},
			},
		},
		{
			Key: common.RedeemerKey{
				Tag:   common.RedeemerTagMint,
				Index: 2,
			},
			Value: common.RedeemerValue{
				ExUnits: common.ExUnits{
					Memory: 1111,
					Steps:  2222,
				},
			},
		},
	}
	iterIdx := 0
	for key, val := range testRedeemers.Iter() {
		expected := expectedOrder[iterIdx]
		if !reflect.DeepEqual(key, expected.Key) {
			t.Fatalf(
				"did not get expected key: got %#v, wanted %#v",
				key,
				expected.Key,
			)
		}
		if !reflect.DeepEqual(val, expected.Value) {
			t.Fatalf(
				"did not get expected value: got %#v, wanted %#v",
				val,
				expected.Value,
			)
		}
		iterIdx++
	}
}

// Transaction taken from https://cexplorer.io/tx/6e6e15e39da0b8283b6c6d10b88b29adcac12e67edcf502c84cd2adb38a68880
// HASH: 6e6e15e39da0b8283b6c6d10b88b29adcac12e67edcf502c84cd2adb38a68880

const conwayTxHex = `84a40081825820b267f34cbf10a995565482742f328537ba4ccc767ea3ec99d4a443b5c71ae70101018182584c82d818584283581c4473be071285b7c1ac840cfaef878d95e078bf67fb859d587d488128a101581e581c5981b261ab5ccd11065b4b22cb3d6ba5938430480014d42742714dfb001a103e5cad1a0afcf06d021a00028ab9031a0b532b80a10081825820ca3554458c006d1ef04f33c13688b06c032d7c80adaf2967a01f1159c8058c65584016ce288ac2aca9ec373f3f84fd3a37c762e0ab244f68b2fd1c59525f24bbd5afc82ccbc67fa3eeeaaa7df2e212eae795b48e7803e6c7ec49b6b14bde6b84550af5f6`

func TestConwayTx_CborRoundTrip_UsingCborEncode(t *testing.T) {
	txHex := strings.TrimSpace(conwayTxHex)
	orig, err := hex.DecodeString(txHex)
	if err != nil {
		t.Fatalf("failed to decode Conway tx hex into CBOR bytes: %v", err)
	}

	var tx ConwayTransaction
	if err := tx.UnmarshalCBOR(orig); err != nil {
		t.Fatalf("failed to unmarshal CBOR into ConwayTransaction: %v", err)
	}

	enc, err := cbor.Encode(tx)
	if err != nil {
		t.Fatalf("failed to encode ConwayTransaction via cbor.Encode: %v", err)
	}
	if len(enc) == 0 {
		t.Fatal("encoded tx CBOR is empty")
	}

	if !bytes.Equal(orig, enc) {
		t.Errorf(
			"CBOR round-trip mismatch\noriginal: %x\nencoded : %x",
			orig,
			enc,
		)

		i := -1
		for j := 0; j < len(orig) && j < len(enc); j++ {
			if orig[j] != enc[j] {
				i = j
				break
			}
		}
		if i != -1 {
			t.Logf(
				"first diff at byte %d: orig=0x%02x enc=0x%02x",
				i,
				orig[i],
				enc[i],
			)
		} else {
			t.Logf("length mismatch: orig=%d enc=%d", len(orig), len(enc))
		}
	}
}

func TestConwayTx_Utxorpc(t *testing.T) {
	txHex := strings.TrimSpace(conwayTxHex)
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		t.Fatalf("failed to decode Conway tx hex: %v", err)
	}

	tx, err := NewConwayTransactionFromCbor(txBytes)
	if err != nil {
		t.Fatalf("failed to parse Conway tx: %v", err)
	}

	utxoTx, err := tx.Utxorpc()
	if err != nil {
		t.Fatalf("failed to convert Conway tx to utxorpc: %v", err)
	}

	expHash := tx.Id().Bytes()
	if !bytes.Equal(utxoTx.Hash, expHash) {
		t.Errorf(
			"tx hash mismatch\nexpected: %x\nactual  : %x",
			expHash,
			utxoTx.Hash,
		)
	}

	if got, want := len(utxoTx.Inputs), len(tx.Inputs()); got != want {
		t.Errorf("inputs count mismatch: got %d want %d", got, want)
	}
	if got, want := len(utxoTx.Outputs), len(tx.Outputs()); got != want {
		t.Errorf("outputs count mismatch: got %d want %d", got, want)
	}
}

func TestConwayTransactionBodyRejectsDuplicateTaggedInputs(t *testing.T) {
	input := testConwayShelleyInput()
	bodyCbor, err := cbor.Encode(map[uint]any{
		0: cbor.NewSetType([]shelley.ShelleyTransactionInput{input, input}, true),
	})
	assert.NoError(t, err)

	var body ConwayTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	assert.ErrorContains(t, err, "duplicate member in set")
}

func TestConwayTransactionBodyRejectsDuplicateMultiAssetKeys(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "mint duplicate policy",
			body: append(
				[]byte{0xa1, 0x09},
				testDuplicatePolicyMultiAssetCbor(0x11)...,
			),
		},
		{
			name: "mint duplicate asset name",
			body: append(
				[]byte{0xa1, 0x09},
				testDuplicateAssetNameMultiAssetCbor(0x22)...,
			),
		},
		{
			name: "output duplicate asset name",
			body: append(
				[]byte{0xa1, 0x01, 0x81},
				testBabbageOutputWithAssetsCbor(
					t,
					testDuplicateAssetNameMultiAssetCbor(0x33),
				)...,
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body ConwayTransactionBody
			err := body.UnmarshalCBOR(tt.body)
			assert.ErrorContains(t, err, "duplicate map key")
		})
	}
}

func TestConwayTransactionBodyRejectsDuplicateTaggedSetFields(t *testing.T) {
	input := testConwayShelleyInput()
	var signer common.Blake2b224
	signer[0] = 1
	tests := []struct {
		name  string
		field uint
		value any
	}{
		{
			name:  "collateral",
			field: 13,
			value: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input, input},
				true,
			),
		},
		{
			name:  "required signers",
			field: 14,
			value: cbor.NewSetType([]common.Blake2b224{signer, signer}, true),
		},
		{
			name:  "reference inputs",
			field: 18,
			value: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input, input},
				true,
			),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyCbor, err := cbor.Encode(map[uint]any{
				tt.field: tt.value,
			})
			assert.NoError(t, err)

			var body ConwayTransactionBody
			err = body.UnmarshalCBOR(bodyCbor)
			assert.ErrorContains(t, err, "duplicate member in set")
		})
	}
}

// Conway (protocol versions 9-11) tolerates duplicate vkey witnesses:
// cardano-ledger decodes them via Set.fromList and only begins rejecting
// duplicates at protocol version 12 (Dijkstra). Rejecting at decode in Conway
// wrongly rejects valid historical blocks (issue #1853).
func TestConwayWitnessSetToleratesDuplicateTaggedVkeyWitness(t *testing.T) {
	dupCbor := []byte{
		0xa1,             // map(1)
		0x00,             // key: 0  (VkeyWitnesses field)
		0xd9, 0x01, 0x02, // tag(258) - CBOR set
		0x82,                         // array(2)
		0x82, 0x41, 0x01, 0x41, 0x02, // VkeyWitness{[0x01], [0x02]}
		0x82, 0x41, 0x01, 0x41, 0x02, // duplicate
	}

	var ws ConwayTransactionWitnessSet
	err := ws.UnmarshalCBOR(dupCbor)
	assert.NoError(t, err,
		"Conway must tolerate duplicate vkey witnesses (dedup at decode, matching cardano-ledger pv 9-11)")
}

func TestConwayWitnessSetToleratesDuplicateTaggedWitnessSetFields(t *testing.T) {
	tests := []struct {
		name   string
		field  byte
		member []byte
	}{
		{
			name:  "bootstrap witnesses",
			field: 0x02,
			member: []byte{
				0x84,                   // BootstrapWitness
				0x41, 0x01, 0x41, 0x02, // public key, signature
				0x41, 0x03, 0x41, 0x04, // chain code, attributes
			},
		},
		{
			name:   "native scripts",
			field:  0x01,
			member: []byte{0x82, 0x00, 0x41, 0x01}, // pubkey script
		},
		{
			name:   "plutus data",
			field:  0x04,
			member: []byte{0x00}, // integer datum
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ws ConwayTransactionWitnessSet
			err := ws.UnmarshalCBOR(
				duplicateTaggedWitnessSetFieldCbor(tt.field, tt.member),
			)
			assert.NoError(t, err)
		})
	}
}

// Plutus script sets reject duplicates from protocol version 9 (cardano-ledger
// scriptDecoderV9), so Conway must still reject them at decode.
func TestConwayWitnessSetRejectsDuplicateTaggedPlutusV1Script(t *testing.T) {
	dupCbor := []byte{
		0xa1,             // map(1)
		0x03,             // key: 3  (WsPlutusV1Scripts field)
		0xd9, 0x01, 0x02, // tag(258) - CBOR set
		0x82,       // array(2)
		0x41, 0x01, // PlutusV1Script [0x01]
		0x41, 0x01, // duplicate
	}

	var ws ConwayTransactionWitnessSet
	err := ws.UnmarshalCBOR(dupCbor)
	assert.ErrorContains(t, err, "duplicate member in set")
}

func TestConwayWitnessSetRejectsDuplicateTaggedPlutusV2AndV3Scripts(t *testing.T) {
	tests := []struct {
		name  string
		field byte
	}{
		{name: "Plutus V2", field: 0x06},
		{name: "Plutus V3", field: 0x07},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ws ConwayTransactionWitnessSet
			err := ws.UnmarshalCBOR(
				duplicateTaggedWitnessSetFieldCbor(
					tt.field,
					[]byte{0x41, 0x01},
				),
			)
			assert.ErrorContains(t, err, "duplicate member in set")
		})
	}
}

func duplicateTaggedWitnessSetFieldCbor(field byte, member []byte) []byte {
	ret := []byte{
		0xa1,             // map(1)
		field,            // witness set field key
		0xd9, 0x01, 0x02, // tag(258) - CBOR set
		0x82, // array(2)
	}
	ret = append(ret, member...)
	ret = append(ret, member...)
	return ret
}

func testConwayShelleyInput() shelley.ShelleyTransactionInput {
	var txId common.Blake2b256
	txId[0] = 1
	return shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: 0,
	}
}

func testDuplicatePolicyMultiAssetCbor(policyByte byte) []byte {
	policy := bytes.Repeat([]byte{policyByte}, common.Blake2b224Size)
	ret := []byte{0xa2, 0x58, 0x1c}
	ret = append(ret, policy...)
	ret = append(ret, 0xa1, 0x41, 0xaa, 0x01, 0x58, 0x1c)
	ret = append(ret, policy...)
	ret = append(ret, 0xa1, 0x41, 0xbb, 0x02)
	return ret
}

func testDuplicateAssetNameMultiAssetCbor(policyByte byte) []byte {
	policy := bytes.Repeat([]byte{policyByte}, common.Blake2b224Size)
	ret := []byte{0xa1, 0x58, 0x1c}
	ret = append(ret, policy...)
	ret = append(ret, 0xa2, 0x41, 0xcc, 0x01, 0x41, 0xcc, 0x09)
	return ret
}

func testBabbageOutputWithAssetsCbor(t *testing.T, assets []byte) []byte {
	t.Helper()
	addr, err := common.NewAddressFromBytes(
		test.DecodeHexString(
			"40000000000000000000000000000000000000000000000000000000008198bd431b03",
		),
	)
	assert.NoError(t, err)
	addrCbor, err := cbor.Encode(addr)
	assert.NoError(t, err)

	ret := []byte{0xa2, 0x00}
	ret = append(ret, addrCbor...)
	ret = append(ret, 0x01, 0x82, 0x01)
	ret = append(ret, assets...)
	return ret
}

func TestConwayTx_WithReferenceInputs_CborRoundTrip(t *testing.T) {
	// Test CBOR round-trip for transactions with reference inputs (CIP-0031)

	// Create a transaction with reference inputs
	tx := &ConwayTransaction{}
	tx.Body.TxInputs = NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{
			shelley.NewShelleyTransactionInput(
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				0,
			),
		},
	)
	tx.Body.TxReferenceInputs = cbor.NewSetType(
		[]shelley.ShelleyTransactionInput{
			shelley.NewShelleyTransactionInput(
				"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
				1,
			),
		},
		false,
	)
	tx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			OutputAddress: func() common.Address {
				addr, _ := common.NewAddressFromBytes(
					test.DecodeHexString(
						"40000000000000000000000000000000000000000000000000000000008198bd431b03",
					),
				)
				return addr
			}(),
			OutputAmount: mary.MaryTransactionOutputValue{
				Amount: 1000000,
			},
		},
	}
	tx.Body.TxFee = 1000
	tx.Body.Ttl = 1000000

	// Encode to CBOR
	cborData, err := cbor.Encode(tx)
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded ConwayTransaction
	_, err = cbor.Decode(cborData, &decoded)
	assert.NoError(t, err)

	// Verify reference inputs are preserved
	refInputs := decoded.Body.TxReferenceInputs.Items()
	assert.Len(t, refInputs, 1)
	refInput := refInputs[0]
	assert.Equal(
		t,
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		hex.EncodeToString(refInput.Id().Bytes()),
	)
	assert.Equal(t, uint32(1), refInput.Index())

	// Verify byte-identical round-trip
	reEncoded, err := cbor.Encode(&decoded)
	assert.NoError(t, err)
	assert.Equal(
		t,
		cborData,
		reEncoded,
		"CBOR round-trip should be byte-identical",
	)
}

func TestConwayTx_WithReferenceScripts_CborRoundTrip(t *testing.T) {
	// Test CBOR round-trip for transactions with reference scripts (CIP-0033)

	// Create a transaction with reference scripts in outputs
	tx := &ConwayTransaction{}
	tx.Body.TxInputs = NewConwayTransactionInputSet(
		[]shelley.ShelleyTransactionInput{
			shelley.NewShelleyTransactionInput(
				"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
				0,
			),
		},
	)
	tx.Body.TxOutputs = []babbage.BabbageTransactionOutput{
		{
			OutputAddress: func() common.Address {
				addr, _ := common.NewAddressFromBytes(
					test.DecodeHexString(
						"40000000000000000000000000000000000000000000000000000000008198bd431b03",
					),
				)
				return addr
			}(),
			OutputAmount: mary.MaryTransactionOutputValue{
				Amount: 1000000,
			},
			// Include a reference script (CIP-0033)
			TxOutScriptRef: &common.ScriptRef{
				Type:   1, // PlutusV1
				Script: &common.PlutusV1Script{0x01, 0x02, 0x03, 0x04},
			},
		},
	}
	tx.Body.TxFee = 1000
	tx.Body.Ttl = 1000000

	// Encode to CBOR
	cborData, err := cbor.Encode(tx)
	assert.NoError(t, err)
	assert.NotEmpty(t, cborData)

	// Decode back from CBOR
	var decoded ConwayTransaction
	_, err = cbor.Decode(cborData, &decoded)
	assert.NoError(t, err)

	// Verify reference script is preserved
	assert.Len(t, decoded.Body.TxOutputs, 1)
	output := decoded.Body.TxOutputs[0]
	assert.NotNil(t, output.TxOutScriptRef)
	assert.Equal(t, uint(1), output.TxOutScriptRef.Type)
	if plutusScript, ok := output.TxOutScriptRef.Script.(common.PlutusV1Script); ok {
		expectedScript := common.PlutusV1Script{0x01, 0x02, 0x03, 0x04}
		assert.Equal(t, expectedScript, plutusScript)
	} else {
		t.Fatalf("expected PlutusV1Script, got %T", output.TxOutScriptRef.Script)
	}

	// Verify byte-identical round-trip
	reEncoded, err := cbor.Encode(&decoded)
	assert.NoError(t, err)
	assert.Equal(
		t,
		cborData,
		reEncoded,
		"CBOR round-trip should be byte-identical",
	)
}

// TestConwayRedeemersDuplicateKeyLenient verifies that a Redeemers map carrying
// a duplicate (tag, index) key — which cardano-node accepts and strict decoding
// rejects — is decoded leniently (last-wins) instead of failing the block.
// See gouroboros #1860.
func TestConwayRedeemersDuplicateKeyLenient(t *testing.T) {
	// Redeemers map with two entries under the same key [Spend(0), index 0]:
	//   { [0,0]: [0,[1,2]], [0,0]: [0,[3,4]] }
	// Plutus data is the bare integer 0; the value's second element is
	// ExUnits [mem, steps].
	dupCbor := []byte{
		0xA2,             // map(2)
		0x82, 0x00, 0x00, // key [Spend, 0]
		0x82, 0x00, 0x82, 0x01, 0x02, // value [datum=0, exUnits=[1,2]]
		0x82, 0x00, 0x00, // key [Spend, 0] (duplicate)
		0x82, 0x00, 0x82, 0x03, 0x04, // value [datum=0, exUnits=[3,4]]
	}
	// Strict decode rejects the duplicate key (the pre-fix behavior that
	// wedged preview sync).
	var strict map[common.RedeemerKey]common.RedeemerValue
	_, strictErr := cbor.Decode(dupCbor, &strict)
	assert.Error(t, strictErr)
	assert.True(
		t,
		cbor.IsDuplicateMapKeyError(strictErr),
		"expected a duplicate-map-key error, got %v",
		strictErr,
	)
	// ConwayRedeemers decodes it, keeping the last value for the duplicate key.
	var r ConwayRedeemers
	require.NoError(t, r.UnmarshalCBOR(dupCbor))
	key := common.RedeemerKey{Tag: common.RedeemerTagSpend, Index: 0}
	assert.Len(t, r.Redeemers, 1)
	assert.Equal(
		t,
		common.ExUnits{Memory: 3, Steps: 4},
		r.Redeemers[key].ExUnits,
		"duplicate key should resolve last-wins",
	)
}
