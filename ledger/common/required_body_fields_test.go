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

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func withRequiredBodyFields(t *testing.T, wire []byte, sub bool) []byte {
	t.Helper()
	var fields map[uint]cbor.RawMessage
	_, err := cbor.Decode(wire, &fields)
	require.NoError(t, err)
	if fields == nil {
		t.Fatal("transaction body did not decode as a CBOR map")
	}
	defaults := map[uint]any{0: cbor.NewSetType([]any{}, false), 1: []any{}}
	if sub {
		delete(fields, 2)
	} else {
		defaults[2] = uint64(0)
	}
	for key, value := range defaults {
		if _, ok := fields[key]; ok {
			continue
		}
		encoded, encodeErr := cbor.Encode(value)
		if encodeErr != nil {
			t.Fatal(encodeErr)
		}
		fields[key] = encoded
	}
	encoded, err := cbor.Encode(fields)
	require.NoError(t, err)
	return encoded
}
