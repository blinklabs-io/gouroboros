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
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// decodeGovActionId builds the wire encoding of a governance action ID and
// decodes it, so the index under test is one the decoder produced rather than
// one the test assigned.
func decodeGovActionId(t *testing.T, idx uint32) GovActionId {
	t.Helper()
	encoded, err := cbor.Encode(
		struct {
			cbor.StructAsArray
			TransactionId []byte
			GovActionIdx  uint32
		}{
			TransactionId: bytes.Repeat([]byte{0x2b}, Blake2b256Size),
			GovActionIdx:  idx,
		},
	)
	require.NoError(t, err)
	var id GovActionId
	require.NoError(t, id.UnmarshalCBOR(encoded))
	require.Equal(t, idx, id.GovActionIdx)
	return id
}

// The Conway CDDL types gov_action_index as `uint .size 2`, so the decoder
// accepts indices up to 65535 while CIP-0129's bech32 payload carries a
// single byte. String cannot report an error, so it renders those as a
// non-bech32 sentinel rather than killing the caller.
func TestGovActionIdStringAboveCip0129Index(t *testing.T) {
	t.Parallel()

	for _, idx := range []uint32{256, 65535} {
		id := decodeGovActionId(t, idx)

		var rendered string
		require.NotPanics(t, func() { rendered = id.String() })
		require.Equal(
			t,
			fmt.Sprintf("%x#%d", id.TransactionId, idx),
			rendered,
		)
		require.False(
			t,
			strings.HasPrefix(rendered, "gov_action1"),
			"an unrepresentable index must not render as bech32",
		)

		// MarshalText keeps rejecting the same value, so no
		// machine-readable output gains the sentinel.
		_, err := id.MarshalText()
		require.Error(t, err)
	}
}

// An index CIP-0129 can represent still renders as bech32, so the sentinel
// cannot be reached by rendering everything that way.
func TestGovActionIdStringWithinCip0129Index(t *testing.T) {
	t.Parallel()

	for _, idx := range []uint32{0, 255} {
		id := decodeGovActionId(t, idx)

		rendered := id.String()
		require.True(
			t,
			strings.HasPrefix(rendered, "gov_action1"),
			"expected bech32 for index %d, got %q", idx, rendered,
		)

		var roundTripped GovActionId
		require.NoError(t, roundTripped.UnmarshalText([]byte(rendered)))
		require.True(t, id.Equal(roundTripped))
	}
}
