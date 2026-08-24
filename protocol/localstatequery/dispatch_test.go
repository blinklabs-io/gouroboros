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

package localstatequery

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalStateQueryDispatchersAcceptListLengthEncodings(t *testing.T) {
	tests := []struct {
		name      string
		canonical []byte
		decode    func(*testing.T, []byte)
	}{
		{
			name:      "QueryWrapper",
			canonical: mustEncodeLocalStateQueryFixture(t, []any{QueryTypeChainPoint}),
			decode: func(t *testing.T, data []byte) {
				var decoded QueryWrapper
				require.NoError(t, decoded.UnmarshalCBOR(data))
				require.IsType(t, &ChainPointQuery{}, decoded.Query)
			},
		},
		{
			name:      "HotCredAuthStatusValue",
			canonical: mustEncodeLocalStateQueryFixture(t, []any{0}),
			decode: func(t *testing.T, data []byte) {
				var decoded HotCredAuthStatusValue
				require.NoError(t, decoded.UnmarshalCBOR(data))
				require.Equal(t, HotCredNotAuthorized, decoded.Status)
			},
		},
		{
			name:      "WithOriginSlot",
			canonical: mustEncodeLocalStateQueryFixture(t, []any{0}),
			decode: func(t *testing.T, data []byte) {
				var decoded WithOriginSlot
				require.NoError(t, decoded.UnmarshalCBOR(data))
				require.False(t, decoded.HasSlot)
			},
		},
		{
			name: "RelayAccessPoint",
			canonical: mustEncodeLocalStateQueryFixture(
				t,
				[]any{int(RelayKindSRV), []byte("relay.example")},
			),
			decode: func(t *testing.T, data []byte) {
				var decoded RelayAccessPoint
				require.NoError(t, decoded.UnmarshalCBOR(data))
				assert.Equal(t, RelayKindSRV, decoded.Kind)
				require.NotNil(t, decoded.Domain)
				assert.Equal(t, "relay.example", *decoded.Domain)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, encoding := range test.CanonicalAndNonShortestList(tc.canonical) {
				t.Run(encoding.Name, func(t *testing.T) {
					tc.decode(t, encoding.Data)
				})
			}
		})
	}
}

func mustEncodeLocalStateQueryFixture(t *testing.T, value any) []byte {
	t.Helper()
	data, err := cbor.Encode(value)
	require.NoError(t, err)
	return data
}
