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
	"bytes"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

type withdrawalBodyDecoder interface {
	UnmarshalCBOR([]byte) error
	Withdrawals() map[*common.Address]*big.Int
	Cbor() []byte
	Id() common.Blake2b256
}

func withdrawalBody(withdrawals []byte) []byte {
	// These are decoder fixtures, not signed or ledger-valid transactions.
	return append([]byte{0xa5, 0, 0x80, 1, 0x80, 2, 0, 3, 0, 5}, withdrawals...)
}

func withdrawalEntry(address, amount []byte) []byte {
	return append(bytes.Clone(address), amount...)
}

func withdrawalMap(indefinite bool, entries ...[]byte) []byte {
	raw := []byte{0xa0 + byte(len(entries))}
	if indefinite {
		raw[0] = 0xbf
	}
	for _, entry := range entries {
		raw = append(raw, entry...)
	}
	if indefinite {
		raw = append(raw, 0xff)
	}
	return raw
}

func TestPreConwayWithdrawalsKeepLastOccurrence(t *testing.T) {
	account := append([]byte{0xe0}, make([]byte, 28)...)
	key := append([]byte{0x58, 29}, account...)
	wideKey := append([]byte{0x59, 0, 29}, account...)
	chunkedKey := append([]byte{0x5f, 0x41, 0xe0, 0x58, 28}, account[1:]...)
	chunkedKey = append(chunkedKey, 0xff)
	otherAccount := bytes.Clone(account)
	otherAccount[28] = 1
	otherKey := append([]byte{0x58, 29}, otherAccount...)
	for _, tc := range []struct {
		name  string
		raw   []byte
		last  uint64
		count int
	}{
		{"identical", withdrawalMap(false, withdrawalEntry(key, []byte{1}), withdrawalEntry(key, []byte{1})), 1, 1},
		{"different", withdrawalMap(false, withdrawalEntry(key, []byte{1}), withdrawalEntry(key, []byte{2})), 2, 1},
		{"zero last", withdrawalMap(false, withdrawalEntry(key, []byte{2}), withdrawalEntry(key, []byte{0})), 0, 1},
		{"indefinite", withdrawalMap(true, withdrawalEntry(key, []byte{1}), withdrawalEntry(key, []byte{2})), 2, 1},
		{"nonadjacent", withdrawalMap(false, withdrawalEntry(key, []byte{1}), withdrawalEntry(otherKey, []byte{3}), withdrawalEntry(key, []byte{2})), 2, 2},
		{"wide key", withdrawalMap(false, withdrawalEntry(key, []byte{1}), withdrawalEntry(wideKey, []byte{2})), 2, 1},
		{"chunked key", withdrawalMap(false, withdrawalEntry(key, []byte{1}), withdrawalEntry(chunkedKey, []byte{2})), 2, 1},
	} {
		for era, newBody := range preConwayTransactionBodyDecoders() {
			t.Run(era+"/"+tc.name, func(t *testing.T) {
				raw := withdrawalBody(tc.raw)
				body := newBody().(withdrawalBodyDecoder)
				require.NoError(t, body.UnmarshalCBOR(raw))
				require.Len(t, body.Withdrawals(), tc.count)
				found := false
				for address, amount := range body.Withdrawals() {
					encoded, err := address.Bytes()
					require.NoError(t, err)
					if bytes.Equal(encoded, account) {
						found = true
						require.Equal(t, tc.last, amount.Uint64())
					} else {
						require.Equal(t, otherAccount, encoded)
						require.Equal(t, uint64(3), amount.Uint64())
					}
				}
				require.True(t, found)
				require.Equal(t, raw, body.Cbor())
				require.Equal(t, common.Blake2b256Hash(raw), body.Id())
				encoded, err := cbor.Encode(body)
				require.NoError(t, err)
				require.Equal(t, raw, encoded)
				var strict conway.ConwayTransactionBody
				require.ErrorContains(
					t,
					strict.UnmarshalCBOR(raw),
					"duplicate withdrawal reward account",
				)
			})
		}
	}
}

func TestPreConwayWithdrawalsValidateEveryEntry(t *testing.T) {
	account := append([]byte{0xe0}, make([]byte, 28)...)
	key := append([]byte{0x58, 29}, account...)
	badNetwork := bytes.Clone(key)
	badNetwork[2] = 0xe2
	notReward := bytes.Clone(key)
	notReward[2] = 0x60
	valid := withdrawalEntry(key, []byte{1})
	for _, tc := range []struct {
		name  string
		entry []byte
	}{
		{"invalid network", withdrawalEntry(badNetwork, []byte{1})},
		{"not reward account", withdrawalEntry(notReward, []byte{1})},
		{"negative amount", withdrawalEntry(key, []byte{0x20})},
		{"text amount", withdrawalEntry(key, []byte{0x61, 0x31})},
		{"integer key", []byte{0, 1}},
	} {
		for era, newBody := range preConwayTransactionBodyDecoders() {
			t.Run(era+"/"+tc.name, func(t *testing.T) {
				for _, entries := range [][][]byte{{tc.entry, valid}, {valid, tc.entry}} {
					body := newBody().(withdrawalBodyDecoder)
					require.Error(
						t,
						body.UnmarshalCBOR(
							withdrawalBody(withdrawalMap(false, entries...)),
						),
					)
				}
			})
		}
	}
}
