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

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
)

// hex.DecodeString returns the bytes it decoded before the offending
// character alongside its error, so discarding that error verifies a
// truncation of the caller's input. "80" is a complete CBOR empty array, so
// the trailing garbage below is dropped and the body verifies against the
// zero-transaction hash as if the input had been well formed.
func TestVerifyBlockBodyRejectsMalformedHex(t *testing.T) {
	t.Parallel()

	ok, err := ledger.VerifyBlockBody(
		"80zz",
		ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
		nil,
	)
	require.Error(t, err)
	require.False(t, ok)
}

// The same input without the trailing garbage must still verify, so the new
// check cannot be satisfied by rejecting everything.
func TestVerifyBlockBodyAcceptsWellFormedHex(t *testing.T) {
	t.Parallel()

	ok, err := ledger.VerifyBlockBody(
		"80",
		ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
		nil,
	)
	require.NoError(t, err)
	require.True(t, ok)
}
