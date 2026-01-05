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

	// Encode to CBOR
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
	for i := range original.Body.TxReferences {
		if decoded.Body.TxReferences[i].TxHash != original.Body.TxReferences[i].TxHash ||
			decoded.Body.TxReferences[i].TxSize != original.Body.TxReferences[i].TxSize {
			t.Errorf("TxReference mismatch at index %d", i)
		}
	}
}
