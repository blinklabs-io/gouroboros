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

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func withRequiredEraBodyFields(t *testing.T, raw []byte, subTransaction bool) []byte {
	t.Helper()
	var fields map[uint]cbor.RawMessage
	_, err := cbor.Decode(raw, &fields)
	require.NoError(t, err)
	values := map[uint]any{0: []any{}, 1: []any{}}
	if !subTransaction {
		values[2] = uint64(0)
	}
	for key, value := range values {
		if _, ok := fields[key]; ok {
			continue
		}
		fields[key], err = cbor.Encode(value)
		require.NoError(t, err)
	}
	raw, err = cbor.Encode(fields)
	require.NoError(t, err)
	return raw
}
