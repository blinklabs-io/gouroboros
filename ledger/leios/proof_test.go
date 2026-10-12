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
	"github.com/stretchr/testify/require"
)

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
