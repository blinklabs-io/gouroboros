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

package shelley_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// blockWithNestedMetadatum returns the mainnet Shelley block fixture with its
// auxiliary data set replaced by a single Shelley auxiliary data value
// (transaction_metadata = { * transaction_metadatum_label =>
// transaction_metadatum }, cardano-ledger
// eras/shelley/impl/cddl/data/shelley.cddl) whose metadatum is depth nested
// single-element arrays around a zero. The metadatum therefore sits four
// levels below the top of the block: block array, metadata set map, auxiliary
// data map, then the metadatum itself.
func blockWithNestedMetadatum(t *testing.T, depth int) []byte {
	t.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(testdata.ShelleyBlockHex))
	if err != nil {
		t.Fatalf("decode block fixture: %v", err)
	}
	var parts []cbor.RawMessage
	if _, err := cbor.Decode(raw, &parts); err != nil {
		t.Fatalf("decode block fixture: %v", err)
	}
	if len(parts) != 4 {
		t.Fatalf("unexpected Shelley block shape: %d elements", len(parts))
	}
	// Built by hand rather than through cbor.Encode: the encoder applies the
	// same nesting cap as the decoder, so the negative control past that cap
	// cannot be produced by encoding.
	metadatum := make([]byte, 0, depth+1)
	for range depth {
		metadatum = append(metadatum, 0x81)
	}
	metadatum = append(metadatum, 0x00)
	// Shelley auxiliary data: a one-entry map from metadatum label 0
	auxData := append([]byte{0xa1, 0x00}, metadatum...)
	// Metadata set: a one-entry map from transaction index 0
	metadataSet := append([]byte{0xa1, 0x00}, auxData...)
	blockBytes := []byte{0x84}
	for i, part := range parts {
		if i == 3 {
			blockBytes = append(blockBytes, metadataSet...)
			continue
		}
		blockBytes = append(blockBytes, part...)
	}
	return blockBytes
}

// metadatumDepth returns the nesting depth of a chain of single-element lists.
func metadatumDepth(md common.TransactionMetadatum) int {
	depth := 0
	for {
		list, ok := md.(common.MetaList)
		if !ok || len(list.Items) != 1 {
			return depth
		}
		depth++
		md = list.Items[0]
	}
}

// TestShelleyBlockDecodesDeeplyNestedMetadatum decodes a metadatum nested past
// the previous fixed cap of 256 inside a real mainnet block, so that the
// levels the block envelope consumes are counted against the same budget.
// decodeMetadatum in cardano-ledger
// (libs/cardano-ledger-core/src/Cardano/Ledger/Metadata.hs) recurses through
// decodeListN with no depth counter, so the reference accepts every depth
// here; the local decoder still applies its explicit resource bound.
func TestShelleyBlockDecodesDeeplyNestedMetadatum(t *testing.T) {
	for _, depth := range []int{128, 200} {
		blockBytes := blockWithNestedMetadatum(t, depth)
		block, err := shelley.NewShelleyBlockFromCbor(
			blockBytes,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		if err != nil {
			t.Fatalf("decode block with metadatum depth %d: %v", depth, err)
		}
		md, ok := block.TransactionMetadataSet.GetMetadata(0)
		if !ok {
			t.Fatalf("metadatum missing at depth %d", depth)
		}
		// Shelley auxiliary data is the label map itself
		labels, ok := md.(common.MetaMap)
		if !ok || len(labels.Pairs) != 1 {
			t.Fatalf("unexpected auxiliary data shape: %T", md)
		}
		if got := metadatumDepth(labels.Pairs[0].Value); got != depth {
			t.Fatalf("metadatum depth: got %d, want %d", got, depth)
		}
	}
}

// TestShelleyBlockRejectsMetadatumPastConfiguredLimit is the negative control
// for the explicit decoder resource bound.
func TestShelleyBlockRejectsMetadatumPastConfiguredLimit(t *testing.T) {
	blockBytes := blockWithNestedMetadatum(t, cbor.MaxNestedLevels+1)
	_, err := shelley.NewShelleyBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	if err == nil {
		t.Fatalf("expected metadatum nesting depth %d to be rejected", cbor.MaxNestedLevels+1)
	}
}
