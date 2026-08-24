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

package byron_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestByronTransactionInputAcceptsListLengthEncodings(t *testing.T) {
	hash := common.Blake2b256{1, 2, 3}
	inner, err := cbor.Encode([]any{hash, uint32(7)})
	require.NoError(t, err)
	canonical, err := cbor.Encode([]any{0, inner})
	require.NoError(t, err)
	for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
		t.Run(encoding.Name, func(t *testing.T) {
			var decoded byron.ByronTransactionInput
			require.NoError(t, decoded.UnmarshalCBOR(encoding.Data))
			require.Equal(t, hash, decoded.TxId)
			require.Equal(t, uint32(7), decoded.OutputIndex)
		})
	}
}
