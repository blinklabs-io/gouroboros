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

package common_test

import (
	"encoding/hex"
	"math/big"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mirCertWire builds the wire form of a move instantaneous rewards certificate
// carrying a single stake credential and the given encoded delta.
//
// The envelope follows the upstream golden encoding for "mir" in
// eras/shelley/test-suite/test/Test/Cardano/Ledger/Shelley/Serialisation/Golden/Encoding.hs:
//
//	TkListLen 2 . TkWord 6 . TkListLen 2 . TkWord 0 <> S rws
//
// with rws a one-entry Map (Credential Staking) DeltaCoin. No on-disk upstream
// fixture carries a MIR certificate, so the delta bytes are appended directly.
func mirCertWire(t *testing.T, deltaHex string) []byte {
	t.Helper()
	credential := "8200" + "581c" + strings.Repeat("ab", 28)
	wire, err := hex.DecodeString(
		"8206" + "8200" + "a1" + credential + deltaHex,
	)
	require.NoError(t, err)
	return wire
}

// TestMirRewardDeltaDecodesAsSigned covers the sign and range of delta_coin.
// The CDDL types it as int (eras/mary/impl/cddl/data/mary.cddl) and the
// reference decodes Map (Credential Staking) DeltaCoin by dispatching on the
// CBOR major type with no sign or range constraint
// (eras/shelley/impl/src/Cardano/Ledger/Shelley/TxCert.hs, instance DecCBOR
// MIRTarget), DeltaCoin being a signed unbounded Integer newtype
// (libs/cardano-ledger-core/src/Cardano/Ledger/Coin.hs). A delta this decoder
// rejects fails the whole containing block.
func TestMirRewardDeltaDecodesAsSigned(t *testing.T) {
	maxUint64 := new(big.Int).SetUint64(^uint64(0))
	beyondUint64 := new(big.Int).Lsh(big.NewInt(1), 64)

	for _, testDef := range []struct {
		name     string
		deltaHex string
		expected *big.Int
	}{
		// DeltaCoin 77, the upstream golden "mir" value.
		{"positive", "184d", big.NewInt(77)},
		{"zero", "00", big.NewInt(0)},
		// aliceOnlyDelta (-1) in
		// eras/shelley/test-suite/test/Test/Cardano/Ledger/Shelley/Examples/MirTransfer.hs.
		{"negativeOne", "20", big.NewInt(-1)},
		{"negativeLarge", "3b00000002540be3ff", big.NewInt(-10_000_000_000)},
		{"minInt64", "3b7fffffffffffffff", big.NewInt(-1 << 63)},
		{"maxUint64", "1bffffffffffffffff", maxUint64},
		{"bignumPositive", "c249010000000000000000", beyondUint64},
		{
			"bignumNegative",
			"c349010000000000000000",
			new(big.Int).Neg(new(big.Int).Add(beyondUint64, big.NewInt(1))),
		},
	} {
		t.Run(testDef.name, func(t *testing.T) {
			var cert common.MoveInstantaneousRewardsCertificate
			require.NoError(
				t,
				cert.UnmarshalCBOR(mirCertWire(t, testDef.deltaHex)),
			)
			assert.Equal(t, uint(0), cert.Reward.Source)
			assert.Equal(t, uint64(0), cert.Reward.OtherPot)
			require.Len(t, cert.Reward.Rewards, 1)
			// Read the delta through RewardsAmount so that this test
			// compiles against the unsigned value type it replaces and
			// fails on the decode rather than on the build.
			amounts := cert.Reward.RewardsAmount()
			require.Len(t, amounts, 1)
			for credential, amount := range amounts {
				require.NotNil(t, credential)
				assert.Equal(
					t,
					uint(common.CredentialTypeAddrKeyHash),
					credential.CredType,
				)
				require.NotNil(t, amount)
				assert.Zero(
					t,
					testDef.expected.Cmp(amount),
					"delta %s",
					amount,
				)
			}
		})
	}
}

// TestMirRewardDeltaRejectsNonInteger is the negative control for the widened
// value type: delta_coin is int, so a value that is not a CBOR integer must
// still fail. A *big.Int target accepts CBOR null and undefined as a nil
// pointer, which the decoder has to reject explicitly.
func TestMirRewardDeltaRejectsNonInteger(t *testing.T) {
	for _, testDef := range []struct {
		name     string
		deltaHex string
	}{
		{"null", "f6"},
		{"undefined", "f7"},
		{"textString", "623737"},
		{"byteString", "4101"},
		{"array", "8101"},
		{"float", "fb4052000000000000"},
		{"boolean", "f5"},
	} {
		t.Run(testDef.name, func(t *testing.T) {
			var cert common.MoveInstantaneousRewardsCertificate
			require.Error(
				t,
				cert.UnmarshalCBOR(mirCertWire(t, testDef.deltaHex)),
			)
		})
	}
}

// TestMirOppositePotDecode covers the other MIRTarget branch. The CDDL types
// the opposite-pot amount as coin, which is uint, so it stays unsigned, and the
// source pot must come from the branch that decoded.
func TestMirOppositePotDecode(t *testing.T) {
	t.Run("treasury", func(t *testing.T) {
		wire, err := hex.DecodeString("8206" + "8201" + "1903e8")
		require.NoError(t, err)
		var cert common.MoveInstantaneousRewardsCertificate
		require.NoError(t, cert.UnmarshalCBOR(wire))
		assert.Equal(t, uint(1), cert.Reward.Source)
		assert.Equal(t, uint64(1000), cert.Reward.OtherPot)
		assert.Empty(t, cert.Reward.Rewards)
	})

	t.Run("reserves", func(t *testing.T) {
		wire, err := hex.DecodeString("8206" + "8200" + "1903e8")
		require.NoError(t, err)
		var cert common.MoveInstantaneousRewardsCertificate
		require.NoError(t, cert.UnmarshalCBOR(wire))
		assert.Equal(t, uint(0), cert.Reward.Source)
		assert.Equal(t, uint64(1000), cert.Reward.OtherPot)
		assert.Empty(t, cert.Reward.Rewards)
	})

	t.Run("negative amount is rejected", func(t *testing.T) {
		wire, err := hex.DecodeString("8206" + "8200" + "20")
		require.NoError(t, err)
		var cert common.MoveInstantaneousRewardsCertificate
		require.Error(t, cert.UnmarshalCBOR(wire))
	})
}
