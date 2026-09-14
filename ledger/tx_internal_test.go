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

func TestDecodeTxComponentsBoundsTopLevelArray(t *testing.T) {
	t.Run("oversized declared count", func(t *testing.T) {
		// A definite array claiming one million components must be rejected
		// from its header, before a decoder allocates a RawMessage slice or
		// attempts to read the absent items.
		data := []byte{0x9a, 0x00, 0x0f, 0x42, 0x40}
		_, _, err := decodeTxComponents(data)
		require.EqualError(
			t,
			err,
			"invalid transaction component count 1000000",
		)
	})

	t.Run("indefinite fifth component", func(t *testing.T) {
		// The fifth item begins a malformed indefinite text string. The
		// component bound must reject it without asking the decoder to consume
		// that item or scan for a closing break.
		data := []byte{0x9f, 0xa0, 0xa0, 0xf6, 0xf6, 0x7f}
		_, _, err := decodeTxComponents(data)
		require.EqualError(
			t,
			err,
			"invalid transaction component count: more than 4",
		)
	})
}
