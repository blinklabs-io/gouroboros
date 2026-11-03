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
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

// nestedMetadatum wraps inner in depth single-element lists.
func nestedMetadatum(depth int, inner common.TransactionMetadatum) common.TransactionMetadatum {
	md := inner
	for range depth {
		md = common.MetaList{Items: []common.TransactionMetadatum{md}}
	}
	return md
}

// TestValidateMetadataContentAcceptsDeepNesting checks that the UTxO metadata
// rule places no bound on nesting depth. validMetadatum in cardano-ledger
// (libs/cardano-ledger-core/src/Cardano/Ledger/Metadata.hs) recurses through
// lists and maps checking only the 64-byte text and byte string limits, so
// the reference accepts every depth the decoder admits.
func TestValidateMetadataContentAcceptsDeepNesting(t *testing.T) {
	zero := common.MetaInt{Value: big.NewInt(0)}
	for _, depth := range []int{64, 65, 1025, 16053} {
		md := common.MetaMap{
			Pairs: []common.MetaPair{
				{
					Key:   common.MetaInt{Value: big.NewInt(1)},
					Value: nestedMetadatum(depth, zero),
				},
			},
		}
		if err := shelley.ValidateMetadataContent(md); err != nil {
			t.Fatalf("metadatum depth %d rejected: %v", depth, err)
		}
	}
}

// TestValidateMetadataContentRejectsOversizedLeafWhenDeep keeps the leaf size
// checks in force below any nesting depth.
func TestValidateMetadataContentRejectsOversizedLeafWhenDeep(t *testing.T) {
	cases := map[string]common.TransactionMetadatum{
		"text":  common.MetaText{Value: strings.Repeat("a", 65)},
		"bytes": common.MetaBytes{Value: make([]byte, 65)},
	}
	for name, leaf := range cases {
		if err := shelley.ValidateMetadataContent(nestedMetadatum(1025, leaf)); err == nil {
			t.Fatalf("oversized %s leaf at depth 1025 accepted", name)
		}
	}
}
