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

package common

import (
	"bytes"
	"math"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

// Metadatum integers span -2^64..2^64-1 on the wire while UTxO-RPC carries
// int64. A value outside int64 is valid chain data, so it must not fail the
// conversion of the transaction (and with it the whole block).
func TestMetadatumToUtxorpcWideIntegers(t *testing.T) {
	t.Parallel()
	pow := func(exp uint) *big.Int { return new(big.Int).Lsh(big.NewInt(1), exp) }
	for _, tc := range []struct {
		name string
		in   *big.Int
		want int64
	}{
		{"maxUint64", new(big.Int).Sub(pow(64), big.NewInt(1)), -1},
		{"twoToThe63", pow(63), math.MinInt64},
		{"belowInt64", new(big.Int).Sub(big.NewInt(math.MinInt64), big.NewInt(1)), math.MaxInt64},
		{"minusTwoToThe64", new(big.Int).Neg(pow(64)), 0},
	} {
		got, err := metadatumToUtxorpc(MetaInt{Value: tc.in})
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.want, got.GetInt(), tc.name)
	}
}

// A threshold above uint32 cannot be met by any script list a transaction
// can carry, so it is carried as the largest representable threshold.
func TestNativeScriptToUtxorpcWideThreshold(t *testing.T) {
	t.Parallel()
	got, err := nativeScriptToUtxorpc(&NativeScriptNofK{N: 1 << 40})
	require.NoError(t, err)
	require.Equal(t, uint32(math.MaxUint32), got.GetScriptNOfK().K)
}

// cardano-ledger orders credentials script hash before key hash.
func TestUpdateCommitteeToUtxorpcLedgerOrder(t *testing.T) {
	t.Parallel()
	key := Credential{CredType: CredentialTypeAddrKeyHash}
	key.Credential[0] = 0x01
	script := Credential{CredType: CredentialTypeScriptHash}
	script.Credential[0] = 0x02
	got, err := govActionToUtxorpc(&UpdateCommitteeGovAction{
		CredEpochs: map[*Credential]uint64{&key: 1, &script: 1 << 40},
		Quorum:     cbor.Rat{Rat: big.NewRat(1, 2)},
	})
	require.NoError(t, err)
	added := got.GetUpdateCommitteeAction().NewCommitteeCredentials
	require.Len(t, added, 2)
	require.Equal(t, script.Credential[:], added[0].CommitteeColdCredential.GetScriptHash())
	require.Equal(t, uint32(math.MaxUint32), added[0].ExpiresEpoch)
	require.Equal(t, key.Credential[:], added[1].CommitteeColdCredential.GetAddrKeyHash())
}

// Treasury withdrawals follow the ledger's reward-account order, which puts
// script credentials before key credentials.
func TestTreasuryWithdrawalToUtxorpcLedgerOrder(t *testing.T) {
	t.Parallel()
	key, err := NewAddressFromBytes(append([]byte{0xe1}, bytes.Repeat([]byte{0x01}, 28)...))
	require.NoError(t, err)
	script, err := NewAddressFromBytes(append([]byte{0xf1}, bytes.Repeat([]byte{0x02}, 28)...))
	require.NoError(t, err)
	got, err := govActionToUtxorpc(&TreasuryWithdrawalGovAction{
		Withdrawals: map[*Address]uint64{&key: 1, &script: 2},
	})
	require.NoError(t, err)
	withdrawals := got.GetTreasuryWithdrawalsAction().Withdrawals
	require.Len(t, withdrawals, 2)
	require.Equal(t, int64(2), withdrawals[0].Coin.GetInt())
	require.Equal(t, int64(1), withdrawals[1].Coin.GetInt())
}
