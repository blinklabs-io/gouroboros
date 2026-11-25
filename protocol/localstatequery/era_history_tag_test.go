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
	"github.com/stretchr/testify/require"
)

// TestEraHistoryReplyRefusesTags checks that a GetEraHistory reply is a
// plain list of era summaries: the reference decodes it as a definite or
// indefinite list, which accepts no tag in front.
func TestEraHistoryReplyRefusesTags(t *testing.T) {
	era := []any{
		[]any{0, 0, 0},
		nil,
		[]any{432000, 1000, []any{uint8(1)}, 36},
	}
	definite, err := cbor.Encode([]any{era})
	require.NoError(t, err)
	indefinite, err := cbor.Encode(cbor.IndefLengthList{era})
	require.NoError(t, err)

	for name, wire := range map[string][]byte{
		"definite":   definite,
		"indefinite": indefinite,
	} {
		t.Run(name, func(t *testing.T) {
			var reply eraHistoryReply
			_, err := cbor.Decode(wire, &reply)
			require.NoError(t, err)
			require.Len(t, reply, 1)
		})
		for tagName, head := range map[string][]byte{
			"tag 258":  {0xd9, 0x01, 0x02},
			"tag 31":   {0xd8, 0x1f},
			"two tags": {0xd8, 0x1f, 0xd8, 0x1f},
		} {
			t.Run(name+"/"+tagName, func(t *testing.T) {
				tagged := append(append([]byte{}, head...), wire...)
				var reply eraHistoryReply
				_, err := cbor.Decode(tagged, &reply)
				require.Error(t, err)
			})
		}
	}
}
