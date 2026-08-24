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

package cbor_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test "github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/stretchr/testify/require"
)

func TestDecodeByIdAcceptsListLengthEncodings(t *testing.T) {
	canonical := test.DecodeHexString("8403010203")
	for _, encoding := range test.CanonicalAndNonShortestList(canonical) {
		t.Run(encoding.Name, func(t *testing.T) {
			objects := map[int]any{
				1: &decodeByIdObjectA{},
				2: &decodeByIdObjectB{},
				3: &decodeByIdObjectC{},
			}
			decoded, err := cbor.DecodeById(encoding.Data, objects)
			require.NoError(t, err)
			require.Equal(
				t,
				&decodeByIdObjectC{Type: 3, Foo: 1, Bar: 2, Baz: 3},
				decoded,
			)
		})
	}
}
