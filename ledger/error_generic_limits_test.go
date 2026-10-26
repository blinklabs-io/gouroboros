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

package ledger

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const genericErrorOverLimitCount = 131073

// genericErrorArrayOverLimit declares one array entry more than the
// untrusted-data limit, using one-byte entries.
func genericErrorArrayOverLimit() []byte {
	data := []byte{0x9a, 0, 0x02, 0x00, 0x01}
	return append(data, make([]byte, genericErrorOverLimitCount)...)
}

// genericErrorMapOverLimit declares one map pair more than the
// untrusted-data limit. Keys are distinct so duplicate-key rejection cannot
// mask the count check.
func genericErrorMapOverLimit() []byte {
	data := []byte{0xba, 0, 0x02, 0x00, 0x01}
	for i := range genericErrorOverLimitCount {
		data = append(
			data,
			0x1a,
			byte(i>>24),
			byte(i>>16),
			byte(i>>8),
			byte(i),
			0x00,
		)
	}
	return data
}

func TestGenericErrorRejectsOversizedCollections(t *testing.T) {
	t.Parallel()
	cases := map[string][]byte{
		"array": genericErrorArrayOverLimit(),
		"map":   genericErrorMapOverLimit(),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			got, err := NewGenericErrorFromCbor(data)
			require.Error(t, err, "peer-declared count must be bounded")
			require.Nil(t, got)
			// A GenericError nested in another failure is decoded by
			// its own UnmarshalCBOR, not by NewGenericErrorFromCbor.
			var nested GenericError
			require.Error(
				t,
				nested.UnmarshalCBOR(data),
				"nested peer-declared count must be bounded",
			)
		})
	}
}

func TestGenericErrorDecodesSmallCollections(t *testing.T) {
	t.Parallel()
	data := []byte{0x83, 0x01, 0x02, 0x03}
	got, err := NewGenericErrorFromCbor(data)
	require.NoError(t, err)
	genErr, ok := got.(*GenericError)
	require.True(t, ok)
	require.Equal(t, data, genErr.Cbor)
	require.Equal(t, []any{uint64(1), uint64(2), uint64(3)}, genErr.Value)
}
