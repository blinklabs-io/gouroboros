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

package leios

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/stretchr/testify/require"
)

func TestVerifyDijkstraCertificate(t *testing.T) {
	secret := big.NewInt(17)
	key := makeLeiosTestKey(t, secret)
	messageHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	message := append([]byte{0x58, 0x20}, messageHash[:]...)
	signature := signLeiosTestMessage(t, secret, message, leiosSignatureDST)
	context := common.DijkstraLeiosCertificateContext{
		AnnouncingBlockHash: messageHash,
		TotalActiveStake:    100,
		Committee: []common.DijkstraLeiosCommitteeMember{
			{Stake: 60, Key: key},
			{Stake: 40},
		},
	}

	t.Run("accepts valid signature at quorum boundary", func(t *testing.T) {
		err := VerifyDijkstraCertificate(
			[]byte{0x80}, signature, 2, big.NewRat(3, 5), context,
		)
		require.NoError(t, err)
	})
	t.Run("rejects signature with no cryptographic validity", func(t *testing.T) {
		err := VerifyDijkstraCertificate(
			[]byte{0x80}, make([]byte, common.LeiosBlsSignatureSize),
			2, big.NewRat(3, 5), context,
		)
		require.ErrorIs(t, err, ErrInvalidSignature)
	})
	t.Run("rejects signer bitfield changed after aggregation", func(t *testing.T) {
		err := VerifyDijkstraCertificate(
			[]byte{0xc0}, signature, 2, big.NewRat(3, 5), context,
		)
		require.ErrorIs(t, err, ErrKeylessSigner)
	})
	t.Run("rejects signer stake below quorum", func(t *testing.T) {
		err := VerifyDijkstraCertificate(
			[]byte{0x80}, signature, 2, big.NewRat(61, 100), context,
		)
		require.ErrorIs(t, err, ErrInsufficientQuorum)
	})
	t.Run("rejects missing proof of possession", func(t *testing.T) {
		bad := context
		bad.Committee = append(
			[]common.DijkstraLeiosCommitteeMember(nil),
			context.Committee...,
		)
		bad.Committee[0].Key = &common.LeiosKey{
			PublicKey:       key.PublicKey,
			PossessionProof: make([]byte, common.LeiosBlsPossessionProofSize),
		}
		err := VerifyDijkstraCertificate(
			[]byte{0x80}, signature, 2, big.NewRat(3, 5), bad,
		)
		require.ErrorIs(t, err, ErrInvalidProof)
	})
}

func makeLeiosTestKey(t *testing.T, secret *big.Int) *common.LeiosKey {
	t.Helper()
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(secret)
	encodedPublic := public.Bytes()
	proof := signLeiosTestMessage(t, secret, encodedPublic[:], leiosProofDST)
	return &common.LeiosKey{
		PublicKey:       encodedPublic[:],
		PossessionProof: proof,
	}
}

func signLeiosTestMessage(
	t *testing.T,
	secret *big.Int,
	message []byte,
	dst string,
) []byte {
	t.Helper()
	hash, err := bls12381.HashToG1(message, []byte(dst))
	require.NoError(t, err)
	var signature bls12381.G1Affine
	signature.ScalarMultiplication(&hash, secret)
	encoded := signature.Bytes()
	return encoded[:]
}
