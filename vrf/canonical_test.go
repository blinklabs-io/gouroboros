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

package vrf

import (
	"testing"

	"filippo.io/edwards25519"
	"github.com/stretchr/testify/require"
)

// nonCanonicalEncodings returns encodings of y'+p (and the sign-bit variant)
// for every y' in [2, 18] that SetBytes accepts as a valid point, paired with
// the canonical encoding of the same y'.
func nonCanonicalEncodings(t *testing.T) (nonCanonical, canonical [][32]byte) {
	t.Helper()
	for yPrime := byte(2); yPrime <= 18; yPrime++ {
		for _, sign := range []byte{0x00, 0x80} {
			var nc, c [32]byte
			// p = 2^255 - 19, so y'+p = 2^255 + (y' - 19), which is
			// 0x7f ff ... ff (0xed + y') in little-endian.
			for i := range nc {
				nc[i] = 0xff
			}
			nc[0] = 0xed + yPrime
			nc[31] = 0x7f | sign
			c[0] = yPrime
			c[31] = sign
			if _, err := (&edwards25519.Point{}).SetBytes(nc[:]); err != nil {
				continue
			}
			if _, err := (&edwards25519.Point{}).SetBytes(c[:]); err != nil {
				continue
			}
			nonCanonical = append(nonCanonical, nc)
			canonical = append(canonical, c)
		}
	}
	return nonCanonical, canonical
}

func TestRejectsNonCanonicalPointEncodings(t *testing.T) {
	t.Parallel()
	nonCanonical, canonical := nonCanonicalEncodings(t)
	require.NotEmpty(t, nonCanonical)

	pk, sk, err := KeyGen(testSeed)
	require.NoError(t, err)
	proof, _, err := Prove(sk, []byte("msg"))
	require.NoError(t, err)

	t.Run("public key", func(t *testing.T) {
		t.Parallel()
		for _, enc := range nonCanonical {
			_, err := VerifyAndHash(enc[:], proof, []byte("msg"))
			require.ErrorIs(t, err, ErrNonCanonicalEncoding, "key %x", enc)
		}
		_, err := VerifyAndHash(pk, proof, []byte("msg"))
		require.NoError(t, err)
	})

	t.Run("gamma", func(t *testing.T) {
		t.Parallel()
		var c, s [32]byte
		for _, enc := range nonCanonical {
			pi := append(append([]byte{}, enc[:]...), proof[32:]...)
			_, err := decodeProofArrays(pi, &c, &s)
			require.ErrorIs(t, err, ErrNonCanonicalEncoding, "gamma %x", enc)
		}
		for _, enc := range canonical {
			pi := append(append([]byte{}, enc[:]...), proof[32:]...)
			_, err := decodeProofArrays(pi, &c, &s)
			require.NoError(t, err, "canonical gamma %x", enc)
		}
	})
}

func TestWrongLengthKeyKeepsLengthError(t *testing.T) {
	t.Parallel()
	for _, key := range [][]byte{nil, make([]byte, 31), make([]byte, 33)} {
		_, wantErr := (&edwards25519.Point{}).SetBytes(key)
		require.Error(t, wantErr, "key length %d", len(key))
		_, err := VerifyAndHash(key, make([]byte, ProofSize), []byte("msg"))
		require.EqualError(t, err, wantErr.Error(), "key length %d", len(key))
	}
}
