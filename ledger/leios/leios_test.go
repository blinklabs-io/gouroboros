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

package leios

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestLeiosEra(t *testing.T) {
	if EraLeios.Id != EraIdLeios {
		t.Errorf(
			"Era ID mismatch: expected %d, got %d",
			EraIdLeios,
			EraLeios.Id,
		)
	}
	if EraLeios.Name != EraNameLeios {
		t.Errorf(
			"Era name mismatch: expected %s, got %s",
			EraNameLeios,
			EraLeios.Name,
		)
	}
}

func TestLeiosBlockTypes(t *testing.T) {
	if BlockTypeLeiosRanking != 8 {
		t.Errorf(
			"BlockTypeLeiosRanking mismatch: expected 8, got %d",
			BlockTypeLeiosRanking,
		)
	}
	if BlockTypeLeiosEndorser != 9 {
		t.Errorf(
			"BlockTypeLeiosEndorser mismatch: expected 9, got %d",
			BlockTypeLeiosEndorser,
		)
	}
	if BlockHeaderTypeLeios != 7 {
		t.Errorf(
			"BlockHeaderTypeLeios mismatch: expected 7, got %d",
			BlockHeaderTypeLeios,
		)
	}
	if TxTypeLeios != 7 {
		t.Errorf("TxTypeLeios mismatch: expected 7, got %d", TxTypeLeios)
	}
}

func TestLeiosEndorserBlock_Type(t *testing.T) {
	block := &LeiosEndorserBlock{}
	if block.Type() != BlockTypeLeiosEndorser {
		t.Errorf(
			"LeiosEndorserBlock.Type() mismatch: expected %d, got %d",
			BlockTypeLeiosEndorser,
			block.Type(),
		)
	}
}

func TestLeiosRankingBlock_Type(t *testing.T) {
	block := &LeiosRankingBlock{}
	if block.Type() != BlockTypeLeiosRanking {
		t.Errorf(
			"LeiosRankingBlock.Type() mismatch: expected %d, got %d",
			BlockTypeLeiosRanking,
			block.Type(),
		)
	}
}

func TestLeiosBlockHeader_Era(t *testing.T) {
	header := &LeiosBlockHeader{}
	if header.Era() != EraLeios {
		t.Errorf(
			"LeiosBlockHeader.Era() mismatch: expected %v, got %v",
			EraLeios,
			header.Era(),
		)
	}
}

func TestLeiosEndorserBlock_Era(t *testing.T) {
	block := &LeiosEndorserBlock{}
	if block.Era() != EraLeios {
		t.Errorf(
			"LeiosEndorserBlock.Era() mismatch: expected %v, got %v",
			EraLeios,
			block.Era(),
		)
	}
}

func TestLeiosRankingBlock_Era(t *testing.T) {
	block := &LeiosRankingBlock{}
	if block.Era() != EraLeios {
		t.Errorf(
			"LeiosRankingBlock.Era() mismatch: expected %v, got %v",
			EraLeios,
			block.Era(),
		)
	}
}

func TestLeiosEndorserBlock_Hash(t *testing.T) {
	block := &LeiosEndorserBlock{
		Body: &LeiosEndorserBlockBody{
			TxReferences: []TxReference{
				{TxHash: common.Blake2b256{0x01, 0x02}, TxSize: 100},
				{TxHash: common.Blake2b256{0x03, 0x04}, TxSize: 200},
			},
		},
	}
	hash := block.Hash()
	if hash == (common.Blake2b256{}) {
		t.Error("LeiosEndorserBlock.Hash() returned zero hash")
	}
	// Hash should be consistent
	hash2 := block.Hash()
	if hash != hash2 {
		t.Error("LeiosEndorserBlock.Hash() not consistent")
	}
}

func TestLeiosBlockHeader_Hash(t *testing.T) {
	header := &LeiosBlockHeader{
		Body: LeiosBlockHeaderBody{
			BabbageBlockHeaderBody: babbage.BabbageBlockHeaderBody{
				BlockNumber:   1,
				Slot:          100,
				PrevHash:      common.Blake2b256{0x01},
				BlockBodySize: 1000,
				BlockBodyHash: common.Blake2b256{0x02},
			},
		},
	}
	hash := header.Hash()
	if hash == (common.Blake2b256{}) {
		t.Error("LeiosBlockHeader.Hash() returned zero hash")
	}
	// Hash should be consistent
	hash2 := header.Hash()
	if hash != hash2 {
		t.Error("LeiosBlockHeader.Hash() not consistent")
	}
}

func TestLeiosEndorserBlock_CborRoundTrip(t *testing.T) {
	original := &LeiosEndorserBlock{
		Body: &LeiosEndorserBlockBody{
			TxReferences: []TxReference{
				{TxHash: common.Blake2b256{0x01, 0x02}, TxSize: 100},
				{TxHash: common.Blake2b256{0x03, 0x04}, TxSize: 200},
			},
		},
	}

	// Encode to CBOR (encodes as map per CIP-0164)
	cborData, err := cbor.Encode(original)
	if err != nil {
		t.Fatalf("Failed to encode LeiosEndorserBlock: %v", err)
	}

	// Decode back
	var decoded LeiosEndorserBlock
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		t.Fatalf("Failed to decode LeiosEndorserBlock: %v", err)
	}

	// Check TxReferences
	if original.Body == nil || decoded.Body == nil {
		t.Fatal("Body is nil")
	}
	if len(decoded.Body.TxReferences) != len(original.Body.TxReferences) {
		t.Errorf(
			"TxReferences length mismatch: expected %d, got %d",
			len(original.Body.TxReferences),
			len(decoded.Body.TxReferences),
		)
	}
	// Build map of original refs for comparison (order may differ after round-trip)
	origRefMap := make(map[common.Blake2b256]uint16)
	for _, ref := range original.Body.TxReferences {
		origRefMap[ref.TxHash] = ref.TxSize
	}
	// Verify all decoded refs exist in original
	for _, ref := range decoded.Body.TxReferences {
		if size, ok := origRefMap[ref.TxHash]; !ok {
			t.Errorf("Decoded TxReference %x not found in original", ref.TxHash[:8])
		} else if size != ref.TxSize {
			t.Errorf("TxSize mismatch for %x: expected %d, got %d", ref.TxHash[:8], size, ref.TxSize)
		}
	}
}

// TestLeiosBlockHeaderOptionalFields tests header optional fields per CIP-0164
func TestLeiosBlockHeaderOptionalFields(t *testing.T) {
	ebSize := uint32(2048)
	certified := true
	header := &LeiosBlockHeader{
		Body: LeiosBlockHeaderBody{
			BabbageBlockHeaderBody: babbage.BabbageBlockHeaderBody{
				BlockNumber:   1,
				Slot:          100,
				PrevHash:      common.Blake2b256{1, 2, 3},
				BlockBodySize: 1024,
				BlockBodyHash: common.Blake2b256{13, 14, 15},
			},
			AnnouncedEb:     &common.Blake2b256{4, 5, 6},
			AnnouncedEbSize: &ebSize,
			CertifiedEb:     &certified,
		},
	}

	// Test optional fields are present
	if header.Body.AnnouncedEb == nil {
		t.Error("AnnouncedEb should not be nil")
	}
	if header.Body.AnnouncedEbSize == nil {
		t.Error("AnnouncedEbSize should not be nil")
	}
	if header.Body.CertifiedEb == nil {
		t.Error("CertifiedEb should not be nil")
	}
	if *header.Body.AnnouncedEb != (common.Blake2b256{4, 5, 6}) {
		t.Error("AnnouncedEb value mismatch")
	}
	if *header.Body.AnnouncedEbSize != 2048 {
		t.Error("AnnouncedEbSize value mismatch")
	}
	if *header.Body.CertifiedEb != true {
		t.Error("CertifiedEb value mismatch")
	}
}

// TestLeiosEndorserBlockFormat tests EB format per CIP-0164
func TestLeiosEndorserBlockFormat(t *testing.T) {
	block := &LeiosEndorserBlock{
		Body: &LeiosEndorserBlockBody{
			TxReferences: []TxReference{
				{TxHash: common.Blake2b256{1, 2, 3}, TxSize: 100},
			},
		},
	}

	if block.Type() != BlockTypeLeiosEndorser {
		t.Errorf("Expected block type %d, got %d", BlockTypeLeiosEndorser, block.Type())
	}
	if len(block.Body.TxReferences) == 0 {
		t.Error("TxReferences should not be empty")
	}
}

// TestLeiosRankingBlockFormat tests ranking block format
func TestLeiosRankingBlockFormat(t *testing.T) {
	block := &LeiosRankingBlock{
		BlockHeader: &LeiosBlockHeader{
			Body: LeiosBlockHeaderBody{
				BabbageBlockHeaderBody: babbage.BabbageBlockHeaderBody{
					BlockNumber: 2,
				},
			},
		},
	}

	if block.Type() != BlockTypeLeiosRanking {
		t.Errorf("Expected block type %d, got %d", BlockTypeLeiosRanking, block.Type())
	}
}

// TestLeiosEndorserBlockNonEmptyTxReferences tests non-empty TxReferences per CIP-0164
func TestLeiosEndorserBlockNonEmptyTxReferences(t *testing.T) {
	block := &LeiosEndorserBlock{
		Body: &LeiosEndorserBlockBody{
			TxReferences: []TxReference{
				{TxHash: common.Blake2b256{1, 2, 3}, TxSize: 100},
			},
		},
	}

	if len(block.Body.TxReferences) == 0 {
		t.Error("TxReferences should not be empty")
	}
	if block.Body.TxReferences[0].TxSize != 100 {
		t.Error("TxSize mismatch")
	}
}

// TestLeiosEndorserBlockLargeMap tests CBOR encoding/decoding with large map lengths (>= 65536)
func TestLeiosEndorserBlockLargeMap(t *testing.T) {
	// Create a block with exactly 65536 TxReferences to test 4-byte length encoding
	// In practice this would be unrealistic, but tests the edge case
	txRefs := make([]TxReference, 65536)
	for i := range txRefs {
		var hash common.Blake2b256
		hash[0] = byte(i >> 8)
		hash[1] = byte(i)
		txRefs[i] = TxReference{TxHash: hash, TxSize: uint16(i % 65536)}
	}

	original := &LeiosEndorserBlock{
		Body: &LeiosEndorserBlockBody{
			TxReferences: txRefs,
		},
	}

	// Test MarshalCBOR handles large maps correctly
	cborData, err := original.MarshalCBOR()
	if err != nil {
		t.Fatalf("Failed to marshal large LeiosEndorserBlock: %v", err)
	}

	// Verify the CBOR starts with 4-byte map length encoding (0xba)
	if len(cborData) < 6 || cborData[1] != 0xba {
		t.Errorf("Expected 4-byte map length encoding (0xba), got: %x", cborData[:min(10, len(cborData))])
	}

	// Test UnmarshalCBOR can decode it back
	var decoded LeiosEndorserBlock
	if err := decoded.UnmarshalCBOR(cborData); err != nil {
		t.Fatalf("Failed to unmarshal large LeiosEndorserBlock: %v", err)
	}

	// Verify the data is correct
	if len(decoded.Body.TxReferences) != 65536 {
		t.Errorf("Expected 65536 TxReferences, got %d", len(decoded.Body.TxReferences))
	}

	for i, ref := range decoded.Body.TxReferences {
		expectedHash := txRefs[i].TxHash
		if ref.TxHash != expectedHash {
			t.Errorf("TxReference %d hash mismatch", i)
			break
		}
		if ref.TxSize != txRefs[i].TxSize {
			t.Errorf("TxReference %d size mismatch: expected %d, got %d", i, txRefs[i].TxSize, ref.TxSize)
			break
		}
	}
}
