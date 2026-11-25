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
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// TestEraHistoryReplyRequiresAnEra checks the reference's Summary decoder
// (HardFork/History/Summary.hs, "Summary: expected at least one era
// summary"): a GetEraHistory reply names at least one era, so an empty
// list, a null or an undefined is an error, not an empty history.
func TestEraHistoryReplyRequiresAnEra(t *testing.T) {
	for _, wire := range [][]byte{
		{0x80},                   // []
		{0x9f, 0xff},             // indefinite []
		{0xf6},                   // null
		{0xf7},                   // undefined
		{0xd9, 0x01, 0x02, 0x80}, // tag 258 []
	} {
		t.Run(fmt.Sprintf("%x", wire), func(t *testing.T) {
			var reply eraHistoryReply
			_, err := cbor.Decode(wire, &reply)
			require.Error(t, err)
		})
	}

	one, err := cbor.Encode([]any{[]any{
		[]any{0, 0, 0},
		nil,
		[]any{432000, 1000, []any{uint8(1)}, 36},
	}})
	require.NoError(t, err)
	var reply eraHistoryReply
	_, err = cbor.Decode(one, &reply)
	require.NoError(t, err)
	require.Len(t, reply, 1)
}
