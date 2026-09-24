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

package ledger_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func TestVerifyBlockValidatesTPraosNonceProofs(t *testing.T) {
	fixtures := map[string]string{
		"Shelley": "829749cb2701843214ae3aee67ae12ec9bdb3502e060ac0b75275d0f52af349c",
		"Allegra": "8d60fab791ae353486f176318d52501dc8f2d1843bfd69492b52ae724d2cb33d",
		"Mary":    "b9c6e9073b124c26f89556d2d7fbb99d8047787517c0ed5e4f0d323b480444b5",
		"Alonzo":  "d932efa15a43b37c7d3389bed5711b1ff4d1038ce9a864cb66d8ed0fe9129852",
	}
	for _, fixture := range testdata.GetTestBlocks() {
		eta0, ok := fixtures[fixture.Name]
		if !ok {
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) {
			block, err := ledger.NewBlockFromCbor(
				fixture.BlockType,
				fixture.Cbor,
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.NoError(t, err)
			isValid, _, _, _, err := ledger.VerifyBlock(
				block,
				eta0,
				testSlotsPerKesPeriod,
				headerOnlyConfig(),
			)
			require.NoError(t, err)
			require.True(t, isValid)

			switch header := block.Header().(type) {
			case *shelley.ShelleyBlockHeader:
				header.Body.NonceVrf.Proof[0] ^= 0x80
			case *allegra.AllegraBlockHeader:
				header.Body.NonceVrf.Proof[0] ^= 0x80
			case *mary.MaryBlockHeader:
				header.Body.NonceVrf.Proof[0] ^= 0x80
			case *alonzo.AlonzoBlockHeader:
				header.Body.NonceVrf.Proof[0] ^= 0x80
			default:
				t.Fatalf("unexpected TPraos fixture header %T", block.Header())
			}
			isValid, _, _, _, err = ledger.VerifyBlock(
				block,
				eta0,
				testSlotsPerKesPeriod,
				headerOnlyConfig(),
			)
			require.ErrorContains(t, err, "nonce VRF verification failed")
			require.False(t, isValid)
		})
	}
}
