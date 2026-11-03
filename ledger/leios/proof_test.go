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
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestVerifyPossessionProofDijkstraNetDomain(t *testing.T) {
	t.Parallel()

	// Pool registration in DijkstraNet transaction
	// 91efd803162e58db214eff88c6a639a9e03cd56d683610017c333ce61c19f530,
	// included at slot 1351585, uses the older Haskell BLS_SIG proof domain.
	public, err := hex.DecodeString(
		"938b11448795257fa3ef3931ca39adf059c523c4a2e3fae01eb2a36bc3c33b1abdc67a99c4768b855c38ab745126f4bc17295edf6be5da6e9d9a3fe82d463a6149f34ab0034193fa33d7491bd90d1d7485a062f72e57e04aee859d52986313e1",
	)
	require.NoError(t, err)
	proof, err := hex.DecodeString(
		"92fd453967e3904b726aed536ca4cc3ca16954d2a946257197977bb06b0ae81deb159a257b54a2eb4079f792e6edb572",
	)
	require.NoError(t, err)
	key := common.LeiosKey{PublicKey: public, PossessionProof: proof}
	require.ErrorIs(t, VerifyPossessionProof(key), ErrInvalidProof)
	require.ErrorIs(t, VerifyPossessionProofForDomain(
		key, common.DijkstraBlsProofDomainPop,
	), ErrInvalidProof)
	require.NoError(t, VerifyPossessionProofForDomain(
		key, common.DijkstraBlsProofDomainLegacySig,
	))
	err = VerifyPossessionProofForDomain(
		key, common.DijkstraBlsProofDomain(255),
	)
	var configErr *common.ValidationError
	require.ErrorAs(t, err, &configErr)
	require.Equal(t, common.ValidationErrorTypeConfiguration, configErr.Type)
	require.NotErrorIs(t, err, ErrInvalidProof)
	key.PossessionProof = makeLeiosTestKey(t, big.NewInt(8)).PossessionProof
	require.ErrorIs(t, VerifyPossessionProofForDomain(
		key, common.DijkstraBlsProofDomainLegacySig,
	), ErrInvalidProof)
}

func TestVerifyPossessionProof(t *testing.T) {
	t.Parallel()
	key := makeLeiosTestKey(t, big.NewInt(7))
	require.NoError(t, VerifyPossessionProof(*key))

	other := makeLeiosTestKey(t, big.NewInt(8))
	swapped := common.LeiosKey{
		PublicKey:       key.PublicKey,
		PossessionProof: other.PossessionProof,
	}
	require.ErrorIs(t, VerifyPossessionProof(swapped), ErrInvalidProof)

	// A signature over the key under the signing DST is not a proof of
	// possession: the two DSTs keep the schemes apart.
	signedKey := common.LeiosKey{
		PublicKey: key.PublicKey,
		PossessionProof: signLeiosTestMessage(
			t,
			big.NewInt(7),
			key.PublicKey,
			leiosSignatureDST,
		),
	}
	require.ErrorIs(t, VerifyPossessionProof(signedKey), ErrInvalidProof)

	malformed := common.LeiosKey{
		PublicKey:       make([]byte, common.LeiosBlsPublicKeySize),
		PossessionProof: key.PossessionProof,
	}
	require.ErrorIs(t, VerifyPossessionProof(malformed), ErrInvalidProof)
}

// A proof that fails the pairing check is a proof failure, not an aggregate
// signature failure.
func TestVerifyPossessionProofFailureIsNotDescribedAsAggregate(t *testing.T) {
	t.Parallel()
	key := makeLeiosTestKey(t, big.NewInt(7))
	other := makeLeiosTestKey(t, big.NewInt(8))
	swapped := common.LeiosKey{
		PublicKey:       key.PublicKey,
		PossessionProof: other.PossessionProof,
	}
	err := VerifyPossessionProof(swapped)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidProof)
	require.NotErrorIs(t, err, ErrInvalidSignature)
	require.NotContains(t, err.Error(), "aggregate")
}
