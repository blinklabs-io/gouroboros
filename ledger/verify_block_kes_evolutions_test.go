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

package ledger_test

import (
	"bytes"
	"crypto/ed25519"
	"testing"

	"github.com/blinklabs-io/gouroboros/kes"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

const kesEvolutionsTestMax = uint64(62)

// kesEvolutionsTestBlock returns a block that passes every VerifyBlock check
// except possibly the KES evolution bound: its operational certificate and
// KES signature are regenerated so that the KES evolution implied by the
// header's slot is exactly evolution. The cold signature is made with a fresh
// key installed as the issuer vkey, and the KES signature with a depth-6 key
// evolved to that index.
func kesEvolutionsTestBlock(t *testing.T, evolution uint64) ledger.Block {
	t.Helper()
	header := decodeOpCertTestHeader(t)
	currentPeriod := header.Body.Slot / blockLimitsTestSlotsPerKesPeriod
	require.GreaterOrEqual(t, currentPeriod, evolution)

	sKey, vKey, err := kes.KeyGen(
		kes.CardanoKesDepth,
		bytes.Repeat([]byte{0x2a}, 32),
	)
	require.NoError(t, err)
	coldSeed := bytes.Repeat([]byte{0x07}, ed25519.SeedSize)
	coldKey := ed25519.NewKeyFromSeed(coldSeed)

	opCert := &header.Body.OpCert
	opCert.HotVkey = vKey
	opCert.KesPeriod = currentPeriod - evolution
	opCert.Signature = ed25519.Sign(
		coldKey,
		common.OpCertSignableBytes(
			opCert.HotVkey,
			opCert.SequenceNumber,
			opCert.KesPeriod,
		),
	)
	copy(header.Body.IssuerVkey[:], coldKey.Public().(ed25519.PublicKey))

	header.Signature = make([]byte, kes.CardanoKesSignatureSize)
	bodyCbor := reencodedHeaderBodyCbor(t, header)
	for range evolution {
		sKey, err = kes.Update(sKey)
		require.NoError(t, err)
	}
	header.Signature, err = kes.Sign(sKey, evolution, bodyCbor)
	require.NoError(t, err)

	forged := reencodeHeader(t, header)
	kesValid, err := ledger.VerifyKes(
		forged,
		blockLimitsTestSlotsPerKesPeriod,
	)
	require.NoError(t, err)
	require.True(t, kesValid, "fixture must carry a valid KES signature")
	return opCertTestBlock(t, forged)
}

func kesEvolutionsVerify(
	block ledger.Block,
	config common.VerifyConfig,
) (bool, error) {
	valid, _, _, _, err := ledger.VerifyBlock(
		block,
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		config,
	)
	return valid, err
}

func TestVerifyBlock_EnforcesMaxKesEvolutions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		evolution uint64
		wantErr   bool
	}{
		{"below max accepted", kesEvolutionsTestMax - 1, false},
		{"at max rejected", kesEvolutionsTestMax, true},
		{"above max rejected", kesEvolutionsTestMax + 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block := kesEvolutionsTestBlock(t, tc.evolution)
			config := opCertTestVerifyConfig()
			config.MaxKesEvolutions = kesEvolutionsTestMax

			valid, err := kesEvolutionsVerify(block, config)
			if !tc.wantErr {
				require.NoError(t, err)
				require.True(t, valid)
				return
			}
			require.Error(
				t,
				err,
				"VerifyBlock accepted KES evolution %d with MaxKesEvolutions %d",
				tc.evolution,
				kesEvolutionsTestMax,
			)
			require.False(t, valid)
			var validationErr *common.ValidationError
			require.ErrorAs(t, err, &validationErr)
			require.Equal(t, common.ValidationErrorTypeKES, validationErr.Type)
			require.Equal(
				t,
				kesEvolutionsTestMax,
				validationErr.Details["max_kes_evolutions"],
			)
			var opCertErr *ledger.OpCertError
			require.ErrorAs(t, err, &opCertErr)
		})
	}
}

func TestVerifyBlock_KesEvolutionsRequiresConfiguration(t *testing.T) {
	t.Parallel()

	// Evolution 62 is outside the mainnet bound but within the key depth, so
	// the skip flag must restore the previous acceptance.
	block := kesEvolutionsTestBlock(t, kesEvolutionsTestMax)

	t.Run("zero max without skip fails closed", func(t *testing.T) {
		t.Parallel()
		config := opCertTestVerifyConfig()
		config.MaxKesEvolutions = 0
		valid, err := kesEvolutionsVerify(block, config)
		require.Error(t, err)
		require.False(t, valid)
		var validationErr *common.ValidationError
		require.ErrorAs(t, err, &validationErr)
		require.Equal(
			t,
			common.ValidationErrorTypeConfiguration,
			validationErr.Type,
		)
	})

	t.Run("skip flag restores unbounded behavior", func(t *testing.T) {
		t.Parallel()
		config := opCertTestVerifyConfig()
		config.MaxKesEvolutions = 0
		config.SkipKesEvolutionsValidation = true
		valid, err := kesEvolutionsVerify(block, config)
		require.NoError(t, err)
		require.True(t, valid)
	})

	t.Run("skip flag ignores a configured max", func(t *testing.T) {
		t.Parallel()
		config := opCertTestVerifyConfig()
		config.SkipKesEvolutionsValidation = true
		config.MaxKesEvolutions = kesEvolutionsTestMax
		valid, err := kesEvolutionsVerify(block, config)
		require.NoError(t, err)
		require.True(t, valid)
	})
}
