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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	bls12381 "github.com/consensys/gnark-crypto/ecc/bls12-381"
	"github.com/stretchr/testify/require"
)

// Tests here touch the package-level verified-key cache and do not run in
// parallel.

func TestCertificateVerifiesWithCachedKeys(t *testing.T) {
	signers, signature, context := benchCertificate(t, 12)
	for range 2 {
		// The second pass verifies every seat from the cache.
		require.NoError(t, VerifyDijkstraCertificate(
			signers, signature, 12, big.NewRat(3, 5), context,
		))
		// Dropping a signer changes the aggregate key, so the signature,
		// made by every seat, no longer verifies.
		dropped := bytes.Clone(signers)
		dropped[0] &^= 0x80
		require.ErrorIs(t, VerifyDijkstraCertificate(
			dropped, signature, 12, big.NewRat(3, 5), context,
		), ErrInvalidSignature)
	}
}

func TestInvalidPossessionProofIsNeverCached(t *testing.T) {
	key := makeLeiosTestKey(t, big.NewInt(101))
	other := makeLeiosTestKey(t, big.NewInt(102))
	forged := common.LeiosKey{
		PublicKey:       key.PublicKey,
		PossessionProof: other.PossessionProof,
	}
	for range 2 {
		require.ErrorIs(t, VerifyPossessionProof(forged), ErrInvalidProof)
		require.NoError(t, VerifyPossessionProof(*key))
	}
	// A seat whose key carries the forged proof fails a certificate even
	// after the genuine proof for the same public key has been cached.
	messageHash := common.Blake2b256Hash([]byte("announcing ranking block"))
	message := append([]byte{0x58, 0x20}, messageHash[:]...)
	signature := signLeiosTestMessage(
		t,
		big.NewInt(101),
		message,
		leiosSignatureDST,
	)
	context := common.DijkstraLeiosCertificateContext{
		AnnouncingBlockHash: messageHash,
		TotalActiveStake:    1,
		Committee: []common.DijkstraLeiosCommitteeMember{
			{Stake: 1, Key: &forged},
		},
	}
	require.ErrorIs(t, VerifyDijkstraCertificate(
		[]byte{0x80}, signature, 1, big.NewRat(1, 1), context,
	), ErrInvalidProof)
}

func TestVerifiedKeyIsBoundToItsProofDomain(t *testing.T) {
	key := makeLeiosTestKey(t, big.NewInt(103))
	require.NoError(t, VerifyPossessionProofForDomain(
		*key, common.DijkstraBlsProofDomainPop,
	))
	// The proof was made under the proof-of-possession DST, so the
	// legacy signature domain rejects it although the key is cached.
	require.ErrorIs(t, VerifyPossessionProofForDomain(
		*key, common.DijkstraBlsProofDomainLegacySig,
	), ErrInvalidProof)
}

func TestVerifiedKeyCacheStaysBounded(t *testing.T) {
	verifiedKeys.Lock()
	saved := verifiedKeys.keys
	full := make(map[verifiedKeyID]*bls12381.G2Affine, verifiedKeyCacheMax)
	for i := range verifiedKeyCacheMax {
		var id verifiedKeyID
		id.publicKey[0], id.publicKey[1] = byte(i), byte(i>>8)
		full[id] = &bls12381.G2Affine{}
	}
	verifiedKeys.keys = full
	verifiedKeys.Unlock()
	t.Cleanup(func() {
		verifiedKeys.Lock()
		verifiedKeys.keys = saved
		verifiedKeys.Unlock()
	})

	require.NoError(
		t,
		VerifyPossessionProof(*makeLeiosTestKey(t, big.NewInt(104))),
	)
	verifiedKeys.RLock()
	defer verifiedKeys.RUnlock()
	require.Len(
		t,
		verifiedKeys.keys,
		1,
		"a full cache empties before admitting a key",
	)
}
