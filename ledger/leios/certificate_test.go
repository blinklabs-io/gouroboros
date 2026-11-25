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

// testLeiosKeyAge honours every key registered in epoch 0: mainnet's KES
// parameters give a maximum key age of 21 epochs.
var testLeiosKeyAge = common.DijkstraLeiosKeyAgeContext{
	Epoch:             1,
	MarkEpochLength:   432000,
	MaxKESEvolutions:  62,
	SlotsPerKESPeriod: 129600,
}

func TestVerifyDijkstraCertificate(t *testing.T) {
	secret := big.NewInt(17)
	key := makeLeiosTestKey(t, secret)
	messageHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	message := messageHash[:]
	signature := signLeiosTestMessage(t, secret, message, leiosSignatureDST)
	context := common.DijkstraLeiosCertificateContext{
		KeyAge: testLeiosKeyAge, AnnouncingBlockHash: messageHash,
		TotalActiveStake: 100,
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
	t.Run("ignores padding bits beyond the committee size", func(t *testing.T) {
		for _, signers := range [][]byte{{0x81}, {0xbf}} {
			err := VerifyDijkstraCertificate(
				signers, signature, 2, big.NewRat(3, 5), context,
			)
			require.NoError(t, err, "signers %x", signers)
		}
	})
	t.Run(
		"rejects signature with no cryptographic validity",
		func(t *testing.T) {
			err := VerifyDijkstraCertificate(
				[]byte{0x80}, make([]byte, common.LeiosBlsSignatureSize),
				2, big.NewRat(3, 5), context,
			)
			require.ErrorIs(t, err, ErrInvalidSignature)
		},
	)
	t.Run(
		"rejects signer bitfield changed after aggregation",
		func(t *testing.T) {
			err := VerifyDijkstraCertificate(
				[]byte{0xc0}, signature, 2, big.NewRat(3, 5), context,
			)
			require.ErrorIs(t, err, ErrKeylessSigner)
		},
	)
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

func TestVerifyDijkstraCertificateForDomain(t *testing.T) {
	secret := big.NewInt(19)
	var public bls12381.G2Affine
	public.ScalarMultiplicationBase(secret)
	encodedPublic := public.Bytes()
	legacyKey := &common.LeiosKey{
		PublicKey: encodedPublic[:],
		PossessionProof: signLeiosTestMessage(
			t, secret, encodedPublic[:], leiosSignatureDST,
		),
	}
	messageHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	message := messageHash[:]
	signature := signLeiosTestMessage(t, secret, message, leiosSignatureDST)
	context := common.DijkstraLeiosCertificateContext{
		KeyAge: testLeiosKeyAge, AnnouncingBlockHash: messageHash,
		TotalActiveStake: 1,
		Committee: []common.DijkstraLeiosCommitteeMember{
			{Stake: 1, Key: legacyKey},
		},
	}
	quorum := big.NewRat(1, 1)

	require.NoError(t, VerifyDijkstraCertificateForDomain(
		[]byte{0x80}, signature, 1, quorum, context,
		common.DijkstraBlsProofDomainLegacySig,
	))
	require.ErrorIs(t, VerifyDijkstraCertificateForDomain(
		[]byte{0x80}, signature, 1, quorum, context,
		common.DijkstraBlsProofDomainPop,
	), ErrInvalidProof)
	require.ErrorIs(t, VerifyDijkstraCertificate(
		[]byte{0x80}, signature, 1, quorum, context,
	), ErrInvalidProof)

	err := VerifyDijkstraCertificateForDomain(
		[]byte{0x80}, signature, 1, quorum, context,
		common.DijkstraBlsProofDomain(255),
	)
	var configErr *common.ValidationError
	require.ErrorAs(t, err, &configErr)
	if configErr == nil {
		t.Fatal("expected validation error")
	}
	require.Equal(t, common.ValidationErrorTypeConfiguration, configErr.Type)
}

func TestVerifyDijkstraCertificateForMessage(t *testing.T) {
	t.Parallel()

	secret := big.NewInt(29)
	key := makeLeiosTestKey(t, secret)
	messageHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	framed := append([]byte{0x58, 0x20}, messageHash[:]...)
	rawSignature := signLeiosTestMessage(
		t, secret, messageHash[:], leiosSignatureDST,
	)
	framedSignature := signLeiosTestMessage(
		t, secret, framed, leiosSignatureDST,
	)
	context := common.DijkstraLeiosCertificateContext{
		KeyAge: testLeiosKeyAge, AnnouncingBlockHash: messageHash,
		TotalActiveStake: 1,
		Committee: []common.DijkstraLeiosCommitteeMember{
			{Stake: 1, Key: key},
		},
	}
	quorum := big.NewRat(1, 1)
	pop := common.DijkstraBlsProofDomainPop
	raw := common.DijkstraLeiosCertificateMessageRawHash
	cborHash := common.DijkstraLeiosCertificateMessageCborHash

	require.NoError(t, VerifyDijkstraCertificate(
		[]byte{0x80}, rawSignature, 1, quorum, context,
	))
	require.ErrorIs(t, VerifyDijkstraCertificate(
		[]byte{0x80}, framedSignature, 1, quorum, context,
	), ErrInvalidSignature)
	require.NoError(t, VerifyDijkstraCertificateForDomain(
		[]byte{0x80}, rawSignature, 1, quorum, context, pop,
	))

	require.NoError(t, VerifyDijkstraCertificateForMessage(
		[]byte{0x80}, rawSignature, 1, quorum, context, pop, raw,
	))
	require.ErrorIs(t, VerifyDijkstraCertificateForMessage(
		[]byte{0x80}, framedSignature, 1, quorum, context, pop, raw,
	), ErrInvalidSignature)
	require.NoError(t, VerifyDijkstraCertificateForMessage(
		[]byte{0x80}, framedSignature, 1, quorum, context, pop, cborHash,
	))
	require.ErrorIs(t, VerifyDijkstraCertificateForMessage(
		[]byte{0x80}, rawSignature, 1, quorum, context, pop, cborHash,
	), ErrInvalidSignature)

	err := VerifyDijkstraCertificateForMessage(
		[]byte{0x80}, rawSignature, 1, quorum, context, pop,
		common.DijkstraLeiosCertificateMessage(255),
	)
	var configErr *common.ValidationError
	require.ErrorAs(t, err, &configErr)
	require.NotNil(t, configErr)
	require.Equal(t, common.ValidationErrorTypeConfiguration, configErr.Type)
}

// The committee is the top committeeSize pools, or every pool when fewer are
// registered (selectLeiosCommittee), and verifyLeiosCert sizes the signer
// bitfield by the committee's own seat count.
func TestVerifyDijkstraCertificateCommitteeSmallerThanParameter(t *testing.T) {
	t.Parallel()
	secrets := []*big.Int{big.NewInt(29), big.NewInt(31)}
	hash := common.Blake2b256Hash([]byte("announcing ranking block"))
	signature := signLeiosTestMessage(
		t,
		new(big.Int).Add(secrets[0], secrets[1]),
		hash[:],
		leiosSignatureDST,
	)
	context := common.DijkstraLeiosCertificateContext{
		KeyAge:              testLeiosKeyAge,
		AnnouncingBlockHash: hash,
		TotalActiveStake:    2,
		Committee: []common.DijkstraLeiosCommitteeMember{
			{Stake: 1, Key: makeLeiosTestKey(t, secrets[0])},
			{Stake: 1, Key: makeLeiosTestKey(t, secrets[1])},
		},
	}
	quorum := big.NewRat(3, 4)

	t.Run("bitfield sized by the seated committee", func(t *testing.T) {
		require.NoError(t, VerifyDijkstraCertificate(
			[]byte{0xc0}, signature, 900, quorum, context,
		))
	})
	t.Run("bitfield sized by the parameter", func(t *testing.T) {
		signers := make([]byte, common.LeiosSignerBitfieldSize(900))
		signers[0] = 0xc0
		err := VerifyDijkstraCertificate(
			signers, signature, 900, quorum, context,
		)
		require.ErrorIs(t, err, ErrMalformedCommittee)
	})
	t.Run("committee larger than the parameter", func(t *testing.T) {
		err := VerifyDijkstraCertificate(
			[]byte{0xc0}, signature, 1, quorum, context,
		)
		require.ErrorIs(t, err, ErrMalformedCommittee)
	})
}
