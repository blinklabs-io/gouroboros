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

package consensus

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/stretchr/testify/require"
)

// TestValidateKESSignatureBeforeStartUsesEvolutionZero covers a header whose
// operational certificate starts after the header's KES period. The
// reference reports KESBeforeStartOCERT from the period check and still
// verifies the KES signature, at evolution 0 (cardano-ledger
// Rules/OCert.hs, ouroboros-consensus PolyPraos.doValidateKESSignature').
// The signature check must therefore pass a signature made at evolution 0,
// reject a bad one, and not repeat the period error.
func TestValidateKESSignatureBeforeStartUsesEvolutionZero(t *testing.T) {
	config := testNetworkConfig()
	config.SlotsPerKESPeriod = 100
	config.MaxKESEvolutions = 2
	validator := NewHeaderValidator(config)

	sk, pk, err := kes.KeyGen(
		kes.CardanoKesDepth,
		[]byte("kes_before_start_seed_0123456789"),
	)
	require.NoError(t, err)
	body := []byte("kes before start header body")
	sig, err := kes.Sign(sk, 0, body)
	require.NoError(t, err)

	input := &ValidateHeaderInput{
		Slot:            250, // KES period 2
		OpCertKesPeriod: 3,   // starts in period 3
		OpCertHotVkey:   pk,
		KesSignature:    sig,
		HeaderBodyCbor:  body,
	}
	require.ErrorContains(
		t,
		validator.validateKESPeriod(input),
		"KES period is in the future",
	)
	require.NoError(t, validator.validateKESSignature(input))

	bad := append([]byte(nil), sig...)
	bad[10] ^= 0xff
	input.KesSignature = bad
	require.ErrorContains(
		t,
		validator.validateKESSignature(input),
		"KES signature verification failed",
	)
}
