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

package txsubmission

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTxIdRejectsWrongLength(t *testing.T) {
	for _, size := range []int{31, 32, 33} {
		t.Run(map[int]string{31: "short", 32: "exact", 33: "long"}[size], func(t *testing.T) {
			wire, err := cbor.Encode([]any{uint16(6), bytes.Repeat([]byte{0xaa}, size)})
			require.NoError(t, err)

			var value TxId
			_, err = cbor.Decode(wire, &value)
			if size == 32 {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
