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
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

type headerMemoryFixture struct {
	name      string
	blockType uint
	block     []byte
}

func headerMemoryFixtures(t testing.TB) []headerMemoryFixture {
	t.Helper()
	var ret []headerMemoryFixture
	for _, f := range testdata.GetTestBlocks() {
		if f.BlockType == ledger.BlockTypeByronMain {
			continue
		}
		ret = append(ret, headerMemoryFixture{f.Name, f.BlockType, f.Cbor})
	}
	raw, err := os.ReadFile(filepath.Join(
		"dijkstra", "testdata", "musashi_dijkstra_block.hex",
	))
	require.NoError(t, err)
	block, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	require.NoError(t, err)
	return append(ret, headerMemoryFixture{
		"Dijkstra", ledger.BlockTypeDijkstra, block,
	})
}

// headerParts returns the stored CBOR of a decoded header's body and the
// decoded byte-string fields that are retained alongside it.
func headerParts(t testing.TB, h ledger.BlockHeader) ([]byte, map[string][]byte) {
	t.Helper()
	shelleyParts := func(s *shelley.ShelleyBlockHeader) ([]byte, map[string][]byte) {
		b := &s.Body
		return b.Cbor(), map[string][]byte{
			"VrfKey":          b.VrfKey,
			"NonceVrf.Output": b.NonceVrf.Output,
			"NonceVrf.Proof":  b.NonceVrf.Proof,
			"LeaderVrf.Out":   b.LeaderVrf.Output,
			"LeaderVrf.Proof": b.LeaderVrf.Proof,
			"OpCertHotVkey":   b.OpCertHotVkey,
			"OpCertSignature": b.OpCertSignature,
			"Signature":       s.Signature,
		}
	}
	babbageParts := func(s *babbage.BabbageBlockHeader) ([]byte, map[string][]byte) {
		b := &s.Body
		return b.Cbor(), map[string][]byte{
			"VrfKey":          b.VrfKey,
			"VrfResult.Out":   b.VrfResult.Output,
			"VrfResult.Proof": b.VrfResult.Proof,
			"OpCert.HotVkey":  b.OpCert.HotVkey,
			"OpCert.Sig":      b.OpCert.Signature,
			"Signature":       s.Signature,
		}
	}
	switch v := h.(type) {
	case *shelley.ShelleyBlockHeader:
		return shelleyParts(v)
	case *allegra.AllegraBlockHeader:
		return shelleyParts(&v.ShelleyBlockHeader)
	case *mary.MaryBlockHeader:
		return shelleyParts(&v.ShelleyBlockHeader)
	case *alonzo.AlonzoBlockHeader:
		return shelleyParts(&v.ShelleyBlockHeader)
	case *babbage.BabbageBlockHeader:
		return babbageParts(v)
	case *conway.ConwayBlockHeader:
		return babbageParts(&v.BabbageBlockHeader)
	case *dijkstra.DijkstraBlockHeader:
		return babbageParts(&v.BabbageBlockHeader)
	}
	t.Fatalf("unexpected header type %T", h)
	return nil, nil
}

// Every Shelley-family header decode must retain the header CBOR exactly
// once: the body's stored CBOR is a view into the header's stored CBOR, and
// neither aliases the caller's input.
func TestHeaderDecodeStoresCborOnce(t *testing.T) {
	t.Parallel()
	for _, f := range headerMemoryFixtures(t) {
		t.Run(f.name, func(t *testing.T) {
			t.Parallel()
			hdrCbor, err := ledger.ExtractBlockHeaderCbor(f.block)
			require.NoError(t, err)
			input := bytes.Clone(hdrCbor)
			header, err := ledger.NewBlockHeaderFromCbor(f.blockType, input)
			require.NoError(t, err)

			stored := header.Cbor()
			require.Equal(t, hdrCbor, stored)
			require.Equal(t, common.Blake2b256Hash(hdrCbor), header.Hash())

			body, fields := headerParts(t, header)
			require.NotEmpty(t, body)
			// The header is [body, signature]; the body follows the
			// one-byte array head.
			require.Equal(t, hdrCbor[1:1+len(body)], body)
			require.Same(
				t, &stored[1], &body[0],
				"body CBOR must share backing memory with header CBOR",
			)

			for name, field := range fields {
				require.NotEmpty(t, field, name)
				i := bytes.Index(stored, field)
				require.GreaterOrEqual(t, i, 0, name)
				require.Same(
					t, &stored[i], &field[0],
					"%s must share backing memory with header CBOR", name,
				)
			}

			// The stored bytes must not alias the caller's input.
			require.NotSame(t, &input[0], &stored[0])
			for i := range input {
				input[i] ^= 0xff
			}
			require.Equal(t, hdrCbor, header.Cbor())
			require.Equal(t, hdrCbor, bytes.Clone(stored))
			require.Equal(t, hdrCbor[1:1+len(body)], body)

			enc, err := cbor.Encode(header)
			require.NoError(t, err)
			require.Equal(t, hdrCbor, enc)
		})
	}
}

func BenchmarkNewBlockHeaderFromCbor(b *testing.B) {
	for _, f := range headerMemoryFixtures(b) {
		hdrCbor, err := ledger.ExtractBlockHeaderCbor(f.block)
		require.NoError(b, err)
		b.Run(f.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(hdrCbor)))
			for b.Loop() {
				if _, err := ledger.NewBlockHeaderFromCbor(f.blockType, hdrCbor); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
