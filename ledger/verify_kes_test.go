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
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
)

func TestVerifyKesComponentsRejectsSmallOrderLeaf(t *testing.T) {
	identity := make(ed25519.PublicKey, ed25519.PublicKeySize)
	identity[0] = 1
	generator := []byte{
		0x58, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66,
		0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66,
		0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66,
		0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66, 0x66,
	}
	leafSignature := make([]byte, ed25519.SignatureSize)
	copy(leafSignature[:32], generator)
	leafSignature[32] = 1

	proof := append([]byte(nil), leafSignature...)
	root := identity
	for level := range uint64(kes.CardanoKesDepth) {
		right := make(ed25519.PublicKey, ed25519.PublicKeySize)
		right[0] = byte(level + 2)
		proof = append(proof, root...)
		proof = append(proof, right...)
		root = kes.HashPair(root, right)
	}
	require.Len(t, proof, kes.CardanoKesSignatureSize)

	valid, err := VerifyKesComponents(
		[]byte("header body"),
		proof,
		root,
		0,
		0,
		1,
	)
	require.NoError(t, err)
	require.False(t, valid, "consensus boundary accepted a small-order leaf")
}
