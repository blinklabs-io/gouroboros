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
	"github.com/stretchr/testify/require"
)

func TestExtractBlockHeaderCborPreservesSupportedHeaders(t *testing.T) {
	for _, fixture := range testdata.GetTestBlocks() {
		t.Run(fixture.Name, func(t *testing.T) {
			var fields []cbor.RawMessage
			_, err := cbor.Decode(fixture.Cbor, &fields)
			require.NoError(t, err)
			require.NotEmpty(t, fields)

			header, err := ledger.ExtractBlockHeaderCbor(fixture.Cbor)
			require.NoError(t, err)
			require.Equal(t, []byte(fields[0]), header)

			header[0] ^= 0xff
			require.False(t, bytes.Equal(header, fields[0]))
			require.Equal(t, []byte(fields[0]), fixture.Cbor[1:1+len(fields[0])])
		})
	}
	t.Run("Byron EBB", func(t *testing.T) {
		hexData, err := os.ReadFile(filepath.Join(
			"..", "protocol", "chainsync", "testdata",
			"byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
		))
		require.NoError(t, err)
		raw, err := hex.DecodeString(strings.TrimSpace(string(hexData)))
		require.NoError(t, err)
		var fields []cbor.RawMessage
		_, err = cbor.Decode(raw, &fields)
		require.NoError(t, err)
		require.NotEmpty(t, fields)

		header, err := ledger.ExtractBlockHeaderCbor(raw)
		require.NoError(t, err)
		require.Equal(t, []byte(fields[0]), header)

		header[0] ^= 0xff
		require.False(t, bytes.Equal(header, fields[0]))
	})
}

func TestExtractBlockHeaderCborRejectsInvalidOuterArrays(t *testing.T) {
	tests := map[string][]byte{
		"empty":      {0x80},
		"too many":   {0x88},
		"indefinite": {0x9f, 0xff},
		"not array":  {0xa0},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ledger.ExtractBlockHeaderCbor(data)
			require.Error(t, err)
		})
	}
}
