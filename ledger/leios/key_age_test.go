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

// The expectations are computed from cardano-ledger 1210d37f
// eras/dijkstra/impl/src/Cardano/Ledger/Dijkstra/Rules/Snap.hs 169-180
// (kesMaxKeyAgeEpochs: ceiling ((maxKESEvo * slotsPerKESPeriod) %
// slotsPerEpoch) + 2), not taken from gOuroboros.
func TestMaxKeyAgeEpochs(t *testing.T) {
	const maxUint64 = ^uint64(0)
	cases := []struct {
		name                       string
		evolutions, period, length uint64
		want                       uint64
		fails                      bool
	}{
		// 62 * 129600 / 432000 = 18.6 epochs.
		{name: "mainnet", evolutions: 62, period: 129600, length: 432000, want: 21},
		// 62 * 129600 / 86400 = 93 epochs exactly.
		{name: "one-day epochs", evolutions: 62, period: 129600, length: 86400, want: 95},
		{name: "lifetime under one epoch", evolutions: 1, period: 1, length: 432000, want: 3},
		{name: "no lifetime", evolutions: 0, period: 129600, length: 432000, want: 2},
		{name: "product past 64 bits", evolutions: maxUint64, period: 2, length: 1, fails: true},
		{name: "product past 64 bits, short again", evolutions: maxUint64, period: 4, length: 8, want: maxUint64/2 + 1 + 2},
		{name: "zero epoch length", evolutions: 62, period: 129600, length: 0, fails: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MaxKeyAgeEpochs(tc.evolutions, tc.period, tc.length)
			if tc.fails {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// A key is honoured while epoch < registeredIn + maxKeyAge (cardano-ledger
// 1210d37f libs/cardano-ledger-core/src/Cardano/Ledger/State/Leios.hs
// 115-121); a signer whose key is not is keyless, and its certificate is
// refused (cardano-base 242f1ebde40b Cardano/Crypto/Leios.hs 312-315).
func TestVerifyDijkstraCertificateKeyAge(t *testing.T) {
	const maxUint64 = ^uint64(0)
	secret := big.NewInt(29)
	key := makeLeiosTestKey(t, secret)
	hash := common.Blake2b256Hash([]byte("key age"))
	signature := signLeiosTestMessage(t, secret, hash[:], leiosSignatureDST)
	// Mainnet's KES parameters: a maximum key age of 21 epochs.
	keyAge := func(epoch uint64) common.DijkstraLeiosKeyAgeContext {
		return common.DijkstraLeiosKeyAgeContext{
			Epoch:             epoch,
			MarkEpochLength:   432000,
			MaxKESEvolutions:  62,
			SlotsPerKESPeriod: 129600,
		}
	}
	cases := []struct {
		name         string
		registeredIn uint64
		age          common.DijkstraLeiosKeyAgeContext
		err          error
	}{
		{"registration epoch", 10, keyAge(10), nil},
		{"last honoured epoch", 10, keyAge(30), nil},
		{"first unhonoured epoch", 10, keyAge(31), ErrKeylessSigner},
		{"long aged out", 10, keyAge(500), ErrKeylessSigner},
		{"registered after the epoch", 40, keyAge(30), nil},
		{"registration near the epoch limit", maxUint64 - 1, keyAge(maxUint64), nil},
		{"no epoch length", 10, common.DijkstraLeiosKeyAgeContext{Epoch: 10}, ErrMalformedCommittee},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := VerifyDijkstraCertificate(
				[]byte{0x80},
				signature,
				1,
				big.NewRat(1, 1),
				common.DijkstraLeiosCertificateContext{
					AnnouncingBlockHash: hash,
					TotalActiveStake:    1,
					Committee: []common.DijkstraLeiosCommitteeMember{
						{Stake: 1, Key: key, KeyRegisteredIn: tc.registeredIn},
					},
					KeyAge: tc.age,
				},
			)
			if tc.err == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.err)
			}
		})
	}

	t.Run("aged key of a seat that did not sign", func(t *testing.T) {
		err := VerifyDijkstraCertificate(
			[]byte{0x80},
			signature,
			2,
			big.NewRat(1, 2),
			common.DijkstraLeiosCertificateContext{
				AnnouncingBlockHash: hash,
				TotalActiveStake:    2,
				Committee: []common.DijkstraLeiosCommitteeMember{
					{Stake: 1, Key: key, KeyRegisteredIn: 20},
					{Stake: 1, Key: makeLeiosTestKey(t, big.NewInt(31))},
				},
				KeyAge: keyAge(30),
			},
		)
		require.NoError(t, err)
	})
}
