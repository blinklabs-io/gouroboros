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

	"github.com/stretchr/testify/require"
)

// TestEmptyTransactionMetadataSetEncodesAsEmptyMap proves that a freshly
// constructed (never-decoded) TransactionMetadataSet encodes as the CBOR
// empty map {} (0xa0), not as CBOR null (0xf6). Every Shelley-through-Conway
// block CDDL declares this field as a required, non-nullable map -- e.g.
// cardano-ledger's shelley.cddl "transaction_metadata_set : {* transaction_index
// => metadata}" and conway.cddl "auxiliary_data_set : {* transaction_index =>
// auxiliary_data}", neither with a "/ nil" alternative -- so a block built
// in-process with no metadata must still emit an empty map at that position.
func TestEmptyTransactionMetadataSetEncodesAsEmptyMap(t *testing.T) {
	t.Parallel()
	var set TransactionMetadataSet
	encoded, err := set.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(
		t,
		[]byte{0xa0},
		encoded,
		"an empty transaction metadata set must encode as the CBOR empty map, not null",
	)
}
