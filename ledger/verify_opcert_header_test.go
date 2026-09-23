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
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestExtractOpCertFromEveryPraosEra(t *testing.T) {
	for _, fixture := range testdata.GetTestBlocks() {
		t.Run(fixture.Name, func(t *testing.T) {
			block, err := ledger.NewBlockFromCbor(
				fixture.BlockType,
				fixture.Cbor,
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.NoError(t, err)

			opCert, err := ledger.ExtractOpCertFromHeader(block.Header())
			require.NoError(t, err)
			if fixture.Name == "Byron" {
				require.Nil(t, opCert)
				return
			}
			require.NotNil(t, opCert)
			issuerVkey := block.Header().IssuerVkey()
			require.NoError(t, ledger.VerifyOpCertSignature(
				opCert,
				issuerVkey[:],
			))
		})
	}
}

func TestExtractOpCertFromDijkstraHeader(t *testing.T) {
	headerCbor, err := hex.DecodeString(realConwayHeaderHex)
	require.NoError(t, err)
	header, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeDijkstra,
		headerCbor,
	)
	require.NoError(t, err)

	opCert, err := ledger.ExtractOpCertFromHeader(header)
	require.NoError(t, err)
	require.NotNil(t, opCert)
	issuerVkey := header.IssuerVkey()
	require.NoError(t, ledger.VerifyOpCertSignature(
		opCert,
		issuerVkey[:],
	))
}

func TestExtractOpCertFromHeaderRejectsUnsupportedAndNilHeaders(t *testing.T) {
	_, err := ledger.ExtractOpCertFromHeader(unsupportedBlockHeader{})
	require.Error(t, err)

	var nilHeader *unsupportedBlockHeader
	_, err = ledger.ExtractOpCertFromHeader(nilHeader)
	require.Error(t, err)
}

type unsupportedBlockHeader struct{}

func (unsupportedBlockHeader) Hash() common.Blake2b256          { return common.Blake2b256{} }
func (unsupportedBlockHeader) PrevHash() common.Blake2b256      { return common.Blake2b256{} }
func (unsupportedBlockHeader) BlockNumber() uint64              { return 0 }
func (unsupportedBlockHeader) SlotNumber() uint64               { return 0 }
func (unsupportedBlockHeader) IssuerVkey() common.IssuerVkey    { return common.IssuerVkey{} }
func (unsupportedBlockHeader) BlockBodySize() uint64            { return 0 }
func (unsupportedBlockHeader) Era() common.Era                  { return common.Era{} }
func (unsupportedBlockHeader) Cbor() []byte                     { return nil }
func (unsupportedBlockHeader) BlockBodyHash() common.Blake2b256 { return common.Blake2b256{} }
