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
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
)

// benchCertificate builds a committee of seats keys, every one of which
// signs, and the aggregate certificate over an announcing block hash.
func benchCertificate(
	b testing.TB,
	seats int,
) ([]byte, []byte, common.DijkstraLeiosCertificateContext) {
	b.Helper()
	messageHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	message := append([]byte{0x58, 0x20}, messageHash[:]...)
	hash, err := bls12381.HashToG1(message, []byte(leiosSignatureDST))
	if err != nil {
		b.Fatal(err)
	}
	context := common.DijkstraLeiosCertificateContext{
		AnnouncingBlockHash: messageHash,
		TotalActiveStake:    uint64(seats),
	}
	var aggregate bls12381.G1Jac
	signers := make([]byte, (seats+7)/8)
	for i := range seats {
		secret := big.NewInt(int64(i) + 1_000_003)
		var public bls12381.G2Affine
		public.ScalarMultiplicationBase(secret)
		encodedPublic := public.Bytes()
		proofHash, err := bls12381.HashToG1(
			encodedPublic[:],
			[]byte(leiosProofDST),
		)
		if err != nil {
			b.Fatal(err)
		}
		var proof bls12381.G1Affine
		proof.ScalarMultiplication(&proofHash, secret)
		encodedProof := proof.Bytes()
		context.Committee = append(
			context.Committee,
			common.DijkstraLeiosCommitteeMember{
				Stake: 1,
				Key: &common.LeiosKey{
					PublicKey:       encodedPublic[:],
					PossessionProof: encodedProof[:],
				},
			},
		)
		var signature bls12381.G1Affine
		signature.ScalarMultiplication(&hash, secret)
		var sigJac bls12381.G1Jac
		sigJac.FromAffine(&signature)
		aggregate.AddAssign(&sigJac)
		signers[i/8] |= 0x80 >> (i % 8)
	}
	var aggregateAffine bls12381.G1Affine
	aggregateAffine.FromJacobian(&aggregate)
	encoded := aggregateAffine.Bytes()
	return signers, encoded[:], context
}

func BenchmarkVerifyDijkstraCertificate(b *testing.B) {
	for _, seats := range []int{100, 900} {
		signers, signature, context := benchCertificate(b, seats)
		if err := VerifyDijkstraCertificate(
			signers, signature, uint16(seats), big.NewRat(3, 5), context,
		); err != nil {
			b.Fatal(err)
		}
		b.Run(fmt.Sprintf("signers=%d", seats), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := VerifyDijkstraCertificate(
					signers, signature, uint16(seats), big.NewRat(3, 5), context,
				); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
