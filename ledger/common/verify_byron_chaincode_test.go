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
	"golang.org/x/crypto/sha3"
)

func TestComputeByronAddressRootHashesChainCodeOfAnyLength(t *testing.T) {
	t.Parallel()
	pubkey := make([]byte, 32)
	attrs := []byte{0xa0}
	for _, n := range []int{0, 31, 32, 33} {
		chainCode := make([]byte, n)
		for i := range chainCode {
			chainCode[i] = byte(i + 1)
		}
		got, err := computeByronAddressRoot(pubkey, chainCode, attrs)
		require.NoError(t, err, "chain code length %d", n)

		// The reference keeps the 0x5840 byte-string header fixed and
		// hashes whatever chain code bytes were supplied.
		buf := []byte{0x83, 0x00, 0x82, 0x00, 0x58, 0x40}
		buf = append(buf, pubkey...)
		buf = append(buf, chainCode...)
		buf = append(buf, attrs...)
		sum := sha3.Sum256(buf)
		require.Equal(t, Blake2b224Hash(sum[:]), got, "chain code length %d", n)
	}
}
