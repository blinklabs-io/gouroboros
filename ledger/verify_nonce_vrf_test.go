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
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/vrf"
	"github.com/stretchr/testify/require"
)

func TestVerifyNonceVrf(t *testing.T) {
	const slot = int64(1200)
	eta0 := bytes.Repeat([]byte{0x91}, 32)
	publicKey, secretKey, err := vrf.KeyGen(bytes.Repeat([]byte{0x27}, 32))
	require.NoError(t, err)
	message, err := vrf.MkSeedTPraos(slot, eta0, vrf.SeedEta())
	require.NoError(t, err)
	proof, output, err := vrf.Prove(secretKey, message)
	require.NoError(t, err)

	result := common.VrfResult{Proof: proof, Output: output}
	require.NoError(t, verifyNonceVrf(result, publicKey, slot, eta0))
	require.Error(t, verifyNonceVrf(result, publicKey, slot, bytes.Repeat([]byte{0x92}, 32)))

	result.Output = bytes.Clone(result.Output)
	result.Output[0] ^= 0x80
	require.Error(t, verifyNonceVrf(result, publicKey, slot, eta0))
}
