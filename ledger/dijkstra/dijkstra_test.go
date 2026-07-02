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

package dijkstra

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

func testPlutusInteger(v int64) data.PlutusData {
	return data.NewInteger(big.NewInt(v))
}

func minimalTxBody() map[uint]any {
	return map[uint]any{
		0: []any{},
		1: []any{},
		2: uint64(0),
	}
}

func minimalWitnessSet() map[uint]any {
	return map[uint]any{}
}

func minimalTxParts() []any {
	return []any{minimalTxBody(), minimalWitnessSet(), nil}
}

func minimalBlockBodyParts(invalidTxs []uint) []any {
	if invalidTxs == nil {
		invalidTxs = []uint{}
	}
	return []any{
		[]any{minimalTxBody()},
		[]any{minimalWitnessSet()},
		map[uint]any{},
		invalidTxs,
		nil,
		nil,
	}
}

func TestDijkstraEra(t *testing.T) {
	require.Equal(t, uint8(EraIdDijkstra), EraDijkstra.Id)
	require.Equal(t, EraNameDijkstra, EraDijkstra.Name)
	require.Equal(t, EraDijkstra, common.EraById(EraIdDijkstra))
}

func TestDijkstraTypeConstants(t *testing.T) {
	require.Equal(t, 8, BlockTypeDijkstra)
	require.Equal(t, 7, BlockHeaderTypeDijkstra)
	require.Equal(t, 7, TxTypeDijkstra)
}

func TestDijkstraTransactionDecodesThreePartTx(t *testing.T) {
	txCbor, err := cbor.Encode(minimalTxParts())
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.True(t, tx.IsValid())
	require.Equal(t, TxTypeDijkstra, tx.Type())
	require.Equal(t, txCbor, tx.Cbor())
}

func TestDijkstraTransactionAllowsOnlyTrueIsValidForMempool(t *testing.T) {
	parts := minimalTxParts()
	withTrue := []any{parts[0], parts[1], true, parts[2]}
	txCbor, err := cbor.Encode(withTrue)
	require.NoError(t, err)

	tx, err := NewDijkstraTransactionFromCbor(txCbor)
	require.NoError(t, err)
	require.True(t, tx.IsValid())

	withFalse := []any{parts[0], parts[1], false, parts[2]}
	txCbor, err = cbor.Encode(withFalse)
	require.NoError(t, err)

	_, err = NewDijkstraTransactionFromCbor(txCbor)
	require.ErrorContains(t, err, "is_valid=false")
}

func TestDijkstraTransactionRejectsOversizedCbor(t *testing.T) {
	txCbor := make([]byte, MaxTxSize+1)

	_, err := NewDijkstraTransactionFromCbor(txCbor)
	require.ErrorContains(t, err, "MaxTxSize")
}

func TestDijkstraBlockBodyRejectsMismatchedBodiesAndWitnesses(t *testing.T) {
	bodyCbor, err := cbor.Encode([]any{
		[]any{minimalTxBody()},
		[]any{},
		map[uint]any{},
		[]uint{},
		nil,
		nil,
	})
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	err = blockBody.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "different number of transaction bodies")
}

func TestDijkstraBlockBodyAppliesInvalidTransactionIndices(t *testing.T) {
	bodyCbor, err := cbor.Encode(minimalBlockBodyParts([]uint{0}))
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	require.NoError(t, blockBody.UnmarshalCBOR(bodyCbor))
	require.Len(t, blockBody.Transactions, 1)
	require.False(t, blockBody.Transactions[0].IsValid())
}

func TestDijkstraBlockBodyRejectsInvalidTransactionIndexOutOfRange(t *testing.T) {
	bodyCbor, err := cbor.Encode(minimalBlockBodyParts([]uint{1}))
	require.NoError(t, err)

	var blockBody DijkstraBlockBody
	err = blockBody.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "outside transaction list length")
}

func TestDijkstraBlockMarshalUsesSevenItemEnvelope(t *testing.T) {
	block := DijkstraBlock{
		BlockBody: DijkstraBlockBody{
			TransactionBodies: []DijkstraTransactionBody{
				{TxFee: 1},
			},
			TransactionWitnessSets: []DijkstraTransactionWitnessSet{{}},
			InvalidTransactions:    []uint{},
			LeiosCertificate:       &DijkstraLeiosCertificate{},
			PerasCertificate:       &DijkstraPerasCertificate{},
		},
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.Len(t, raw, 7)
	// When a Leios cert is present, the prototype encodes empty transaction
	// components and resolves the EB closure through LeiosDB.
	require.Equal(t, []byte{0x80}, []byte(raw[1]))
	require.Equal(t, []byte{0x80}, []byte(raw[2]))
	require.Equal(t, []byte{0xa0}, []byte(raw[3]))
	require.Equal(t, []byte{0x80}, []byte(raw[4]))
	require.Equal(t, []byte{0x80}, []byte(raw[5]))
	require.Equal(t, []byte{0x80}, []byte(raw[6]))
}

// leiosExtendedHeaderHex is a real Dijkstra block header captured from the
// Leios prototype testnet (network magic 164) at slot 1309596. After the
// Leios header extension activated mid-Dijkstra, the header body carries an
// 11th element (a [hash, uint] pair) following protocol_version, which a plain
// Babbage header decoder rejects.
const leiosExtendedHeaderHex = "828b19ff661a0013fb9c582017f18b18364e406fdf68a72845f9b005119425ddd344d171a7d5dd005df8b7f058200c058acd8105777ffee584b5bc3e7507fe1615179bb2d291f83b089e7733de5f582017658451b63aa1614b7a93c12533a9d73032a599a411533cf5e88f9fb98343c58258405231c2bde7d989ef9df5f31dd636ad8bb6773ef1d3b1e54f35e9c2ebac647aa525b6a9219763b3a9e285a5f7acd8d1cf8c5c3ed2fc780a9f4b9f1f063a7346a05850d58cc6245cad4d434f6b17fc5af1d9d56df46f7d483af40b134f817903f0061ce3cac58fda15cfe085fa42407d03b40d207dad9d2255791f1c000bfebd36a3017aa274cbb138e6768199574e0466f8091a000151c158206f351bafba4758062454b2125a57419c3599e62be312ed0677411d4ad05c0f6184582022cead0f99f08fb8002d88ce78ec2b8bc2657b0b9e4b96f68b01dcc249e71671000058403f56eec6cb0cb526154a6fd0dcc27fe1efcbeabac244bc49c2c1ac304db52bd5d40da6960c6331a808307340c285b1abc94ea69a947ef64980f0015f1f77a607820c00825820a2dcb7c0d77a9ab7396d734665d5b25f7ead1ee0595704a90f9825c6b533925019567f5901c0103d6a7191e176f98c0dd438ba9fb84f665d17728eb659e26096fee55f9493f8d3914ebd42204b6d1ca0c93d0edab06235e7e44b5458c3ab676763dd1306d60388573c0fd8f581c24f0bbeb91eae9379d175fd6c532120acd570e3b83db66b03dade4cdfa86f4fc857c3c3580eeb14d5eb7922b2c83531aea1c725d9a296ee51a24b6006c0c0b4f0842ed247536aafe86b983252566d869ae04a3bab013ff55a116b7a220285cce2e49355b2e9aa316f3e4f71c4e1ffc880462bc49e39e064c783ce40c166b39673407fea92ed55a9721fdf9d3c06d38ec0edd2278effc70b9c985ec7c776edc22234f7659e4c085821c27756713ad2243080e1e32d28a635ff9f45309b537c142f880ff07898aa4cf5b2f26356e267a2c8fb425b324653c991d0c9138a27891be8d5d8a427446cce884e3cb4b3d9d9b3a8d551a3e0471091a4dffb3ef2868c9982a9fafcc6ef22ba750bf9a38c821e1c496511b98ee90e43b01f088e3a96e2ddb0be9309b63ca34c317e708d0a739f69e979f9b9a97cd608e1a64266877574542014af4f55223c6ff56eaa98730bde2ad7bef8d164b5a4d0a729e746714501986f1d97c69e5645befd8771bf628a3fdf6011bf8e438aaadc35"

// TestDijkstraBlockHeaderDecodesLeiosExtension verifies that the Leios-extended
// Dijkstra block header (11-field body) decodes, exposes the standard Babbage
// fields, retains the extra Leios field, and round-trips byte-for-byte so the
// header hash is stable.
func TestDijkstraBlockHeaderDecodesLeiosExtension(t *testing.T) {
	raw, err := hex.DecodeString(leiosExtendedHeaderHex)
	require.NoError(t, err)

	var header DijkstraBlockHeader
	_, err = cbor.Decode(raw, &header)
	require.NoError(t, err)

	// Standard Babbage header fields decode positionally.
	require.Equal(t, uint64(65382), header.BlockNumber())
	require.Equal(t, uint64(1309596), header.SlotNumber())

	// The 11th body element is retained verbatim.
	require.Len(t, header.LeiosHeaderExtension, 1)

	// The body's stored CBOR must be the ORIGINAL (Leios-extended) body bytes,
	// not a re-encoding of the leading Babbage fields: KES signature
	// verification (ledger.extractOriginalBodyCbor) is computed over the
	// original header-body encoding.
	var top []cbor.RawMessage
	_, err = cbor.Decode(raw, &top)
	require.NoError(t, err)
	if len(top) == 0 {
		t.Fatal("expected header CBOR to contain a body")
	}
	require.Equal(t, []byte(top[0]), header.Body.Cbor())

	// Round-trips byte-for-byte so the header hash matches the wire bytes.
	out, err := header.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, raw, out)
	require.Equal(t, common.Blake2b256Hash(raw), header.Hash())
}

// TestDijkstraBlockHeaderDecodesLegacyBabbageBody verifies that a plain
// 10-field Babbage-shaped Dijkstra header (pre-Leios-extension) still decodes
// with no Leios extension captured.
func TestDijkstraBlockHeaderDecodesLegacyBabbageBody(t *testing.T) {
	// Re-encode the captured header with only the first 10 body fields to
	// model a legacy Dijkstra header.
	full, err := hex.DecodeString(leiosExtendedHeaderHex)
	require.NoError(t, err)
	var top []cbor.RawMessage
	_, err = cbor.Decode(full, &top)
	require.NoError(t, err)
	if len(top) < 2 {
		t.Fatal("expected header CBOR to contain body and signature")
	}
	var bodyElems []cbor.RawMessage
	_, err = cbor.Decode(top[0], &bodyElems)
	require.NoError(t, err)
	if len(bodyElems) < 10 {
		t.Fatal("expected at least 10 header body fields")
	}
	legacyBody, err := cbor.Encode(bodyElems[:10])
	require.NoError(t, err)
	legacyHeader, err := cbor.Encode([]cbor.RawMessage{
		cbor.RawMessage(legacyBody),
		top[1],
	})
	require.NoError(t, err)

	var header DijkstraBlockHeader
	_, err = cbor.Decode(legacyHeader, &header)
	require.NoError(t, err)
	require.Nil(t, header.LeiosHeaderExtension)
	require.Equal(t, uint64(65382), header.BlockNumber())
	require.Equal(t, uint64(1309596), header.SlotNumber())
}

func TestDijkstraBlockBodyHashIncludesLeiosAndPerasCertSlots(t *testing.T) {
	withoutCerts := DijkstraBlockBody{}
	withCerts := DijkstraBlockBody{
		LeiosCertificate: &DijkstraLeiosCertificate{},
		PerasCertificate: &DijkstraPerasCertificate{},
	}

	require.NotEqual(t, withoutCerts.Hash(), withCerts.Hash())
}

func TestDijkstraLeiosCertificateMatchesCddlPlaceholder(t *testing.T) {
	raw, err := cbor.Encode([]any{})
	require.NoError(t, err)
	require.Equal(t, []byte{0x80}, raw)

	var cert DijkstraLeiosCertificate
	require.NoError(t, cert.UnmarshalCBOR(raw))
	require.Equal(t, raw, cert.Cbor())

	encoded, err := cbor.Encode(&cert)
	require.NoError(t, err)
	require.Equal(t, raw, encoded)

	nonEmpty, err := cbor.Encode([]any{uint64(99)})
	require.NoError(t, err)
	err = cert.UnmarshalCBOR(nonEmpty)
	require.ErrorContains(t, err, "empty list")
}

func TestDijkstraBlockRoundTripWithBodyHash(t *testing.T) {
	blockBody := DijkstraBlockBody{
		TransactionBodies:      []DijkstraTransactionBody{},
		TransactionWitnessSets: []DijkstraTransactionWitnessSet{},
		InvalidTransactions:    []uint{},
	}
	block := DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: blockBody.Hash(),
					VrfKey:        make([]byte, 32),
					VrfResult: common.VrfResult{
						Output: []byte{},
						Proof:  make([]byte, 80),
					},
					OpCert: babbage.BabbageOpCert{
						HotVkey:   make([]byte, 32),
						Signature: make([]byte, 64),
					},
					ProtoVersion: babbage.BabbageProtoVersion{
						Major: MinProtocolVersionDijkstra,
					},
				},
				Signature: make([]byte, 448),
			},
		},
		BlockBody: blockBody,
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Equal(t, blockBody.Hash(), decoded.BlockBodyHash())
	require.Empty(t, decoded.Transactions())

	var raw []cbor.RawMessage
	_, err = cbor.Decode(blockCbor, &raw)
	require.NoError(t, err)
	require.NotEmpty(t, raw)
	require.NotEmpty(t, decoded.BlockHeader.Cbor())
	require.Equal(t, []byte(raw[0]), decoded.BlockHeader.Cbor())
}

func TestDijkstraRedeemersRejectsDuplicateMapKey(t *testing.T) {
	// Craft a raw CBOR map with two identical RedeemerKey entries.
	// RedeemerKey{Tag:0, Index:0} encodes as StructAsArray [0,0] = 82 00 00.
	// RedeemerValue{Datum(int 1), ExUnits{1,1}} encodes as [1,[1,1]] = 82 01 82 01 01.
	// DupMapKeyEnforcedAPF on the shared decode mode must reject this before rule evaluation.
	dupCbor := []byte{
		0xa2,             // map(2)
		0x82, 0x00, 0x00, // key:   [0, 0]
		0x82, 0x01, 0x82, 0x01, 0x01, // value: [1, [1, 1]]
		0x82, 0x00, 0x00, // key:   [0, 0]  -- duplicate
		0x82, 0x02, 0x82, 0x02, 0x02, // value: [2, [2, 2]]
	}
	var r DijkstraRedeemers
	err := r.UnmarshalCBOR(dupCbor)
	require.Error(t, err)
}

func TestDijkstraWitnessSetRejectsDuplicateTaggedVkeyWitness(t *testing.T) {
	// Craft a witness set CBOR where field 0 (vkey witnesses) is a tag-258 set
	// containing two identical VkeyWitness entries.
	// VkeyWitness{Vkey:[0x01], Signature:[0x02]} = 82 41 01 41 02.
	// The Dijkstra guard introduced in UnmarshalCBOR must reject this.
	dupCbor := []byte{
		0xa1,             // map(1)
		0x00,             // key: 0  (VkeyWitnesses field)
		0xd9, 0x01, 0x02, // tag(258) — CBOR set
		0x82,                         // array(2)
		0x82, 0x41, 0x01, 0x41, 0x02, // VkeyWitness{[0x01], [0x02]}
		0x82, 0x41, 0x01, 0x41, 0x02, // duplicate
	}
	var ws DijkstraTransactionWitnessSet
	err := ws.UnmarshalCBOR(dupCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateSubTransaction(t *testing.T) {
	// Craft a transaction body where field 23 (TxSubTransactions) is a tag-258 set
	// containing two identical minimal sub-transactions.
	// Minimal sub-transaction = [empty-body, empty-witness, null] = 83 a0 a0 f6.
	dupCbor := []byte{
		0xa1,             // map(1)
		0x17,             // key: 23 (TxSubTransactions field)
		0xd9, 0x01, 0x02, // tag(258) — CBOR set
		0x82,                   // array(2)
		0x83, 0xa0, 0xa0, 0xf6, // sub-tx: [empty-body, empty-witness, null]
		0x83, 0xa0, 0xa0, 0xf6, // duplicate
	}
	var body DijkstraTransactionBody
	err := body.UnmarshalCBOR(dupCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateTaggedInputs(t *testing.T) {
	input := testShelleyInput()
	bodyCbor, err := cbor.Encode(map[uint]any{
		0: cbor.NewSetType([]shelley.ShelleyTransactionInput{input, input}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateSubTransactionInputs(t *testing.T) {
	input := testShelleyInput()
	subTx := []any{
		map[uint]any{
			0: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{input, input},
				true,
			),
		},
		map[uint]any{},
		nil,
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		23: cbor.NewSetType([]any{subTx}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateSubTransactionReferenceInputs(t *testing.T) {
	refInput := testShelleyInput()
	subTx := DijkstraSubTransaction{
		Body: DijkstraSubTransactionBody{
			TxReferenceInputs: cbor.NewSetType(
				[]shelley.ShelleyTransactionInput{refInput, refInput},
				true,
			),
		},
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		23: cbor.NewSetType([]DijkstraSubTransaction{subTx}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func testShelleyInput() shelley.ShelleyTransactionInput {
	var txId common.Blake2b256
	txId[0] = 1
	return shelley.ShelleyTransactionInput{
		TxId:        txId,
		OutputIndex: 0,
	}
}

func TestDijkstraTransactionBodyRejectsDuplicateCredentialGuards(t *testing.T) {
	var hash common.Blake2b224
	hash[0] = 1
	guard := common.Credential{
		CredType:   common.CredentialTypeAddrKeyHash,
		Credential: hash,
	}
	bodyCbor, err := cbor.Encode(map[uint]any{
		14: cbor.NewSetType([]common.Credential{guard, guard}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraTransactionBodyRejectsDuplicateKeyHashGuards(t *testing.T) {
	var hash common.Blake2b224
	hash[0] = 1
	bodyCbor, err := cbor.Encode(map[uint]any{
		14: cbor.NewSetType([]common.Blake2b224{hash, hash}, true),
	})
	require.NoError(t, err)

	var body DijkstraTransactionBody
	err = body.UnmarshalCBOR(bodyCbor)
	require.ErrorContains(t, err, "duplicate member in set")
}

func TestDijkstraWitnessSetAllowsDuplicateUntaggedVkeyWitness(t *testing.T) {
	// Untagged arrays (no tag 258) are not checked for duplicates so that
	// pre-Dijkstra encodings decoded via this path remain accepted.
	dupCbor := []byte{
		0xa1, // map(1)
		0x00, // key: 0  (VkeyWitnesses field)
		// plain array — no tag 258
		0x82,                         // array(2)
		0x82, 0x41, 0x01, 0x41, 0x02, // VkeyWitness{[0x01], [0x02]}
		0x82, 0x41, 0x01, 0x41, 0x02, // duplicate — must NOT be rejected
	}
	var ws DijkstraTransactionWitnessSet
	require.NoError(t, ws.UnmarshalCBOR(dupCbor))
}

func TestDijkstraBlockDecodesRedeemerWitnessMap(t *testing.T) {
	expectedRedeemerData := data.NewInteger(big.NewInt(42))
	blockBody := DijkstraBlockBody{
		TransactionBodies: []DijkstraTransactionBody{
			{TxFee: 1},
		},
		TransactionWitnessSets: []DijkstraTransactionWitnessSet{
			{
				WsRedeemers: DijkstraRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagGuarding, Index: 0}: {
							Data: common.Datum{
								Data: expectedRedeemerData,
							},
							ExUnits: common.ExUnits{
								Memory: 11,
								Steps:  22,
							},
						},
					},
				},
			},
		},
		InvalidTransactions: []uint{},
	}
	block := DijkstraBlock{
		BlockHeader: &DijkstraBlockHeader{
			BabbageBlockHeader: babbage.BabbageBlockHeader{
				Body: babbage.BabbageBlockHeaderBody{
					BlockBodyHash: blockBody.Hash(),
					VrfKey:        make([]byte, 32),
					VrfResult: common.VrfResult{
						Output: []byte{},
						Proof:  make([]byte, 80),
					},
					OpCert: babbage.BabbageOpCert{
						HotVkey:   make([]byte, 32),
						Signature: make([]byte, 64),
					},
					ProtoVersion: babbage.BabbageProtoVersion{
						Major: MinProtocolVersionDijkstra,
					},
				},
				Signature: make([]byte, 448),
			},
		},
		BlockBody: blockBody,
	}

	blockCbor, err := block.MarshalCBOR()
	require.NoError(t, err)

	decoded, err := NewDijkstraBlockFromCbor(blockCbor)
	require.NoError(t, err)
	require.Len(t, decoded.BlockBody.TransactionWitnessSets, 1)

	redeemers := decoded.BlockBody.TransactionWitnessSets[0].WsRedeemers
	require.Equal(t, 1, redeemers.Len())
	redeemer := redeemers.Value(0, common.RedeemerTagGuarding)
	require.NotNil(t, redeemer.Data.Data)
	require.True(
		t,
		expectedRedeemerData.Equal(redeemer.Data.Data),
		"redeemer data mismatch: got %s, want %s",
		redeemer.Data.Data,
		expectedRedeemerData,
	)
	require.Equal(t, int64(11), redeemer.ExUnits.Memory)
	require.Equal(t, int64(22), redeemer.ExUnits.Steps)
	require.Equal(t, blockBody.Hash(), decoded.BlockBodyHash())
}

func TestIsCborNullOnlyAcceptsEncodedNull(t *testing.T) {
	require.True(t, isCborNull(cbor.RawMessage{0xf6}))
	require.False(t, isCborNull(nil))
	require.False(t, isCborNull(cbor.RawMessage{}))
}

func TestDijkstraProtocolParametersRoundTrip(t *testing.T) {
	rat := func(num, denom int64) *cbor.Rat {
		return &cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	ratValue := func(num, denom int64) cbor.Rat {
		return cbor.Rat{Rat: big.NewRat(num, denom)}
	}
	pparams := DijkstraProtocolParameters{
		ConwayProtocolParameters: conway.ConwayProtocolParameters{
			MinFeeA:    44,
			MaxTxSize:  16384,
			A0:         rat(1, 2),
			Rho:        rat(3, 1000),
			Tau:        rat(1, 5),
			CostModels: map[uint][]int64{3: {1, 2, 3}},
			ProtocolVersion: common.ProtocolParametersProtocolVersion{
				Major: 12,
			},
			ExecutionCosts: common.ExUnitPrice{
				MemPrice:  rat(1, 10),
				StepPrice: rat(2, 10),
			},
			MaxTxExUnits:        common.ExUnits{Memory: 1, Steps: 2},
			MaxBlockExUnits:     common.ExUnits{Memory: 3, Steps: 4},
			MaxCollateralInputs: 3,
			PoolVotingThresholds: conway.PoolVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpSecurityGroup:       ratValue(1, 2),
			},
			DRepVotingThresholds: conway.DRepVotingThresholds{
				MotionNoConfidence:    ratValue(1, 2),
				CommitteeNormal:       ratValue(1, 2),
				CommitteeNoConfidence: ratValue(1, 2),
				UpdateToConstitution:  ratValue(1, 2),
				HardForkInitiation:    ratValue(1, 2),
				PpNetworkGroup:        ratValue(1, 2),
				PpEconomicGroup:       ratValue(1, 2),
				PpTechnicalGroup:      ratValue(1, 2),
				PpGovGroup:            ratValue(1, 2),
				TreasuryWithdrawal:    ratValue(1, 2),
			},
			MinFeeRefScriptCostPerByte: rat(15, 1000),
		},
		MaxRefScriptSizePerBlock: 1000,
		MaxRefScriptSizePerTx:    2000,
		RefScriptCostStride:      16,
		RefScriptCostMultiplier:  rat(2, 1),
	}

	pparamsCbor, err := cbor.Encode(pparams)
	require.NoError(t, err)

	var decoded DijkstraProtocolParameters
	_, err = cbor.Decode(pparamsCbor, &decoded)
	require.NoError(t, err)
	require.Equal(t, uint(44), decoded.MinFeeA)
	require.Equal(t, uint(16384), decoded.MaxTxSize)
	require.Equal(t, uint(3), decoded.MaxCollateralInputs)
	require.Equal(t, uint32(1000), decoded.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), decoded.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(16), decoded.RefScriptCostStride)
	require.Equal(t, 0, decoded.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))
}

func TestDijkstraProtocolParametersUpdateNil(t *testing.T) {
	pparams := DijkstraProtocolParameters{
		MaxRefScriptSizePerBlock: 1000,
	}

	require.NotPanics(t, func() {
		pparams.Update(nil)
	})
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
}

func TestDijkstraProtocolParameterUpdateDecodesConwayAndDijkstraFields(t *testing.T) {
	updateCbor, err := cbor.Encode(map[uint]any{
		0:  uint(44),
		34: uint32(1000),
		35: uint32(2000),
		36: uint32(16),
		37: cbor.Rat{Rat: big.NewRat(2, 1)},
	})
	require.NoError(t, err)

	var update DijkstraProtocolParameterUpdate
	_, err = cbor.Decode(updateCbor, &update)
	require.NoError(t, err)
	require.Equal(t, updateCbor, update.Cbor())
	require.NotNil(t, update.MinFeeA)
	require.Equal(t, uint(44), *update.MinFeeA)
	require.NotNil(t, update.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(1000), *update.MaxRefScriptSizePerBlock)
	require.NotNil(t, update.RefScriptCostMultiplier)
	require.Equal(t, 0, update.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))

	var pparams DijkstraProtocolParameters
	pparams.Update(&update)
	require.Equal(t, uint(44), pparams.MinFeeA)
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(2000), pparams.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(16), pparams.RefScriptCostStride)
	require.Equal(t, 0, pparams.RefScriptCostMultiplier.Cmp(big.NewRat(2, 1)))
}

func TestDijkstraProtocolParameterUpdateLeiosStakeFieldsExcludedFromCbor(t *testing.T) {
	updateCbor, err := cbor.Encode(DijkstraProtocolParameterUpdate{
		CommitteeStakeCoverage: &cbor.Rat{Rat: big.NewRat(99, 100)},
		QuorumStakeThreshold:   &cbor.Rat{Rat: big.NewRat(3, 4)},
	})
	require.NoError(t, err)

	var decoded map[uint]cbor.RawMessage
	_, err = cbor.Decode(updateCbor, &decoded)
	require.NoError(t, err)
	require.Empty(t, decoded)
}

func TestDijkstraGenesisDecodesCurrentDevnetExample(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "maxRefScriptSizePerBlock": 1048576,
  "maxRefScriptSizePerTx": 204800,
  "refScriptCostStride": 25600,
  "refScriptCostMultiplier": 1.2
}`))
	require.NoError(t, err)
	require.Equal(t, uint32(1048576), genesis.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(204800), genesis.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(25600), genesis.RefScriptCostStride)
	require.Equal(t, 0, genesis.RefScriptCostMultiplier.Cmp(big.NewRat(6, 5)))

	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&genesis))
	require.Equal(t, uint32(1048576), pparams.MaxRefScriptSizePerBlock)
	require.Equal(t, uint32(204800), pparams.MaxRefScriptSizePerTx)
	require.Equal(t, uint32(25600), pparams.RefScriptCostStride)
	require.Equal(t, 0, pparams.RefScriptCostMultiplier.Cmp(big.NewRat(6, 5)))
}

func TestDijkstraGenesisLeiosStakeParameters(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "committeeStakeCoverage": 0.99,
  "quorumStakeThreshold": 0.75
}`))
	require.NoError(t, err)

	var pparams DijkstraProtocolParameters
	require.NoError(t, pparams.UpdateFromGenesis(&genesis))
	require.Equal(
		t,
		0,
		pparams.CommitteeStakeCoverage.Cmp(big.NewRat(99, 100)),
	)
	require.Equal(
		t,
		0,
		pparams.QuorumStakeThreshold.Cmp(big.NewRat(3, 4)),
	)
}

func TestDijkstraGenesisRejectsInvalidLeiosStakeParameters(t *testing.T) {
	genesis, err := NewDijkstraGenesisFromReader(strings.NewReader(`{
  "committeeStakeCoverage": 0.75,
  "quorumStakeThreshold": 0.75
}`))
	require.NoError(t, err)

	var pparams DijkstraProtocolParameters
	err = pparams.UpdateFromGenesis(&genesis)
	require.ErrorAs(t, err, &LeiosCommitteeStakeParametersError{})
}

func TestDijkstraLeiosStakeParametersValidateSingleField(t *testing.T) {
	tests := []struct {
		name                   string
		committeeStakeCoverage *cbor.Rat
		quorumStakeThreshold   *cbor.Rat
		expectErr              bool
	}{
		{
			name:                   "valid coverage without quorum",
			committeeStakeCoverage: &cbor.Rat{Rat: big.NewRat(1, 2)},
		},
		{
			name:                 "valid quorum without coverage",
			quorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(1, 2)},
		},
		{
			name:                   "invalid coverage without quorum",
			committeeStakeCoverage: &cbor.Rat{Rat: big.NewRat(0, 1)},
			expectErr:              true,
		},
		{
			name:                 "invalid quorum without coverage",
			quorumStakeThreshold: &cbor.Rat{Rat: big.NewRat(2, 1)},
			expectErr:            true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLeiosCommitteeStakeParameters(
				tt.committeeStakeCoverage,
				tt.quorumStakeThreshold,
			)
			if tt.expectErr {
				require.ErrorAs(
					t,
					err,
					&LeiosCommitteeStakeParametersError{},
				)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestDijkstraProtocolParametersApplyUpdateLeiosStakeInvariant(t *testing.T) {
	pparams := DijkstraProtocolParameters{
		CommitteeStakeCoverage: &cbor.Rat{Rat: big.NewRat(99, 100)},
		QuorumStakeThreshold:   &cbor.Rat{Rat: big.NewRat(3, 4)},
	}
	validQuorum := &cbor.Rat{Rat: big.NewRat(4, 5)}
	require.NoError(t, pparams.ApplyUpdate(&DijkstraProtocolParameterUpdate{
		QuorumStakeThreshold: validQuorum,
	}))
	require.Equal(t, 0, pparams.QuorumStakeThreshold.Cmp(big.NewRat(4, 5)))

	invalidCoverage := &cbor.Rat{Rat: big.NewRat(4, 5)}
	err := pparams.ApplyUpdate(&DijkstraProtocolParameterUpdate{
		CommitteeStakeCoverage: invalidCoverage,
	})
	require.ErrorAs(t, err, &LeiosCommitteeStakeParametersError{})
	require.Equal(
		t,
		0,
		pparams.CommitteeStakeCoverage.Cmp(big.NewRat(99, 100)),
	)
}

func TestDijkstraProtocolParametersUpdatePreservesLeiosStakeInvariant(t *testing.T) {
	pparams := DijkstraProtocolParameters{
		MaxRefScriptSizePerBlock: 1000,
		CommitteeStakeCoverage:   &cbor.Rat{Rat: big.NewRat(99, 100)},
		QuorumStakeThreshold:     &cbor.Rat{Rat: big.NewRat(3, 4)},
	}
	validQuorum := &cbor.Rat{Rat: big.NewRat(4, 5)}
	pparams.Update(&DijkstraProtocolParameterUpdate{
		QuorumStakeThreshold: validQuorum,
	})
	require.Equal(t, 0, pparams.QuorumStakeThreshold.Cmp(big.NewRat(4, 5)))

	invalidCoverage := &cbor.Rat{Rat: big.NewRat(4, 5)}
	maxRefScriptSizePerBlock := uint32(2000)
	pparams.Update(&DijkstraProtocolParameterUpdate{
		MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
		CommitteeStakeCoverage:   invalidCoverage,
	})
	require.Equal(t, uint32(1000), pparams.MaxRefScriptSizePerBlock)
	require.Equal(
		t,
		0,
		pparams.CommitteeStakeCoverage.Cmp(big.NewRat(99, 100)),
	)
	require.Equal(t, 0, pparams.QuorumStakeThreshold.Cmp(big.NewRat(4, 5)))
}

func TestDijkstraProtocolParameterUpdateToPlutusDataCostModels(t *testing.T) {
	maxRefScriptSizePerBlock := uint32(1234)
	maxRefScriptSizePerTx := uint32(4321)
	refScriptCostStride := uint32(64)
	update := DijkstraProtocolParameterUpdate{
		CostModels: map[uint][]int64{
			3: {9, 8},
			1: {7},
		},
		MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
		MaxRefScriptSizePerTx:    &maxRefScriptSizePerTx,
		RefScriptCostStride:      &refScriptCostStride,
		RefScriptCostMultiplier:  &cbor.Rat{Rat: big.NewRat(5, 2)},
	}
	expected := data.NewMap([][2]data.PlutusData{
		{
			testPlutusInteger(18),
			data.NewMap([][2]data.PlutusData{
				{
					testPlutusInteger(1),
					data.NewList(testPlutusInteger(7)),
				},
				{
					testPlutusInteger(3),
					data.NewList(
						testPlutusInteger(9),
						testPlutusInteger(8),
					),
				},
			}),
		},
		{testPlutusInteger(34), testPlutusInteger(1234)},
		{testPlutusInteger(35), testPlutusInteger(4321)},
		{testPlutusInteger(36), testPlutusInteger(64)},
		{
			testPlutusInteger(37),
			data.NewList(testPlutusInteger(5), testPlutusInteger(2)),
		},
	})

	result := update.ToPlutusData()
	require.True(t, expected.Equal(result), "got %#v", result)
}

func TestDijkstraParameterChangeGovActionDecodesDijkstraUpdateFields(t *testing.T) {
	maxRefScriptSizePerBlock := uint32(1000)
	action := &DijkstraParameterChangeGovAction{
		Type: uint(common.GovActionTypeParameterChange),
		ParamUpdate: DijkstraProtocolParameterUpdate{
			MaxRefScriptSizePerBlock: &maxRefScriptSizePerBlock,
		},
	}
	actionCbor, err := cbor.Encode(&DijkstraGovAction{Action: action})
	require.NoError(t, err)

	var decoded DijkstraGovAction
	_, err = cbor.Decode(actionCbor, &decoded)
	require.NoError(t, err)
	decodedAction, ok := decoded.Action.(*DijkstraParameterChangeGovAction)
	require.True(t, ok)
	require.NotNil(t, decodedAction.ParamUpdate.MaxRefScriptSizePerBlock)
	require.Equal(
		t,
		maxRefScriptSizePerBlock,
		*decodedAction.ParamUpdate.MaxRefScriptSizePerBlock,
	)
}

func TestDijkstraTransactionLeiosHashCaches(t *testing.T) {
	tx := &DijkstraTransaction{}
	tx.SetCbor([]byte{0x83, 0xa0, 0xa0, 0xa0})

	first := tx.LeiosHash()
	require.NotNil(t, tx.hash)
	cached := tx.hash

	second := tx.LeiosHash()
	require.Equal(t, first, second)
	require.True(t, cached == tx.hash)
}
