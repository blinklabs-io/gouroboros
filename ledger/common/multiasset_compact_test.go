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

package common

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestMultiAssetCompactRepresentationSizeDecodeBoundary(t *testing.T) {
	t.Parallel()
	const maxEntries = 1488 // 28 + 44*1488 = 65500 <= 65535
	for _, tc := range []struct {
		name       string
		entryCount int
		wantError  bool
	}{
		{name: "at bound", entryCount: maxEntries},
		{name: "over bound", entryCount: maxEntries + 1, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assets := make(map[cbor.ByteString]uint64, tc.entryCount)
			for i := range tc.entryCount {
				assets[cbor.NewByteString([]byte{byte(i >> 8), byte(i)})] = 1
			}
			wire, err := cbor.Encode(map[Blake2b224]map[cbor.ByteString]uint64{
				{}: assets,
			})
			require.NoError(t, err)
			var decoded MultiAsset[MultiAssetTypeOutput]
			_, err = cbor.Decode(wire, &decoded)
			if tc.wantError {
				require.ErrorContains(t, err, "too big to compact")
				return
			}
			require.NoError(t, err)
		})
	}
}
