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

package shelley_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/require"
)

func rewardAccountBytes(header byte, credential []byte) []byte {
	return append([]byte{header}, credential...)
}

func encodeWithdrawalBody(
	t *testing.T,
	withdrawals map[cbor.ByteString]uint64,
) []byte {
	t.Helper()
	ret, err := cbor.Encode(map[uint64]any{5: withdrawals})
	require.NoError(t, err)
	return ret
}

func TestShelleyTransactionBodyWithdrawalAddressForms(t *testing.T) {
	credential := bytes.Repeat([]byte{0x42}, common.AddressHashSize)
	keyAddr := rewardAccountBytes(0xe1, credential)
	scriptAddr := rewardAccountBytes(0xf1, credential)

	bodyCbor := encodeWithdrawalBody(t, map[cbor.ByteString]uint64{
		cbor.NewByteString(keyAddr):    1,
		cbor.NewByteString(scriptAddr): 2,
	})
	var body shelley.ShelleyTransactionBody
	_, err := cbor.Decode(bodyCbor, &body)
	require.NoError(t, err)
	require.Len(t, body.TxWithdrawals, 2)

	credentialTypes := make(map[uint]struct{}, 2)
	for addr := range body.TxWithdrawals {
		decoded, err := addr.RewardAccountCredential()
		require.NoError(t, err)
		credentialTypes[decoded.CredType] = struct{}{}
		require.Equal(t, common.NewBlake2b224(credential), decoded.Credential)
	}
	require.Contains(t, credentialTypes, uint(common.CredentialTypeAddrKeyHash))
	require.Contains(t, credentialTypes, uint(common.CredentialTypeScriptHash))
}

func TestShelleyTransactionBodyRejectsInvalidWithdrawalAddresses(t *testing.T) {
	hash := bytes.Repeat([]byte{0x33}, common.AddressHashSize)
	tests := []struct {
		name string
		addr []byte
	}{
		{
			name: "base address",
			addr: append(rewardAccountBytes(0x01, hash), hash...),
		},
		{
			name: "enterprise address",
			addr: rewardAccountBytes(0x61, hash),
		},
		{
			name: "reserved header",
			addr: rewardAccountBytes(0x91, hash),
		},
		{
			name: "invalid network tag",
			addr: rewardAccountBytes(0xe2, hash),
		},
		{
			name: "short key credential",
			addr: rewardAccountBytes(0xe1, hash[:len(hash)-1]),
		},
		{
			name: "short script credential",
			addr: rewardAccountBytes(0xf1, hash[:len(hash)-1]),
		},
		{
			// Address parsing preserves a small mainnet trailing-byte
			// compatibility whitelist. Reward accounts are not part of that
			// historical exception and must retain their exact 29-byte form.
			name: "trailing byte",
			addr: append(rewardAccountBytes(0xe1, hash), 0x00),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bodyCbor := encodeWithdrawalBody(t, map[cbor.ByteString]uint64{
				cbor.NewByteString(test.addr): 1,
			})
			var body shelley.ShelleyTransactionBody
			_, err := cbor.Decode(bodyCbor, &body)
			require.Error(t, err)
		})
	}
}

func TestShelleyTransactionBodyRejectsSemanticDuplicateWithdrawal(t *testing.T) {
	addr := rewardAccountBytes(
		0xe1,
		bytes.Repeat([]byte{0x55}, common.AddressHashSize),
	)
	// {5: {_ <addr>: 1, <addr>: 2}}. The first key is an indefinite-length
	// byte string and the second is definite-length. Both decode to the same
	// reward account and must not survive as distinct pointer map keys.
	bodyCbor := []byte{0xa1, 0x05, 0xa2, 0x5f, 0x58, 0x1d}
	bodyCbor = append(bodyCbor, addr...)
	bodyCbor = append(bodyCbor, 0xff, 0x01, 0x58, 0x1d)
	bodyCbor = append(bodyCbor, addr...)
	bodyCbor = append(bodyCbor, 0x02)

	var body shelley.ShelleyTransactionBody
	_, err := cbor.Decode(bodyCbor, &body)
	require.Error(t, err)
	require.ErrorContains(t, err, "duplicate withdrawal reward account")
}

func TestShelleyFamilyBodiesRejectNonRewardWithdrawals(t *testing.T) {
	hash := bytes.Repeat([]byte{0x66}, common.AddressHashSize)
	baseAddr := append(rewardAccountBytes(0x01, hash), hash...)
	bodyCbor := encodeWithdrawalBody(t, map[cbor.ByteString]uint64{
		cbor.NewByteString(baseAddr): 1,
	})
	tests := []struct {
		name string
		dest func() any
	}{
		{"Shelley", func() any { return &shelley.ShelleyTransactionBody{} }},
		{"Allegra", func() any { return &allegra.AllegraTransactionBody{} }},
		{"Mary", func() any { return &mary.MaryTransactionBody{} }},
		{"Alonzo", func() any { return &alonzo.AlonzoTransactionBody{} }},
		{"Babbage", func() any { return &babbage.BabbageTransactionBody{} }},
		{"Conway", func() any { return &conway.ConwayTransactionBody{} }},
		{"Dijkstra", func() any { return &dijkstra.DijkstraTransactionBody{} }},
		{
			"Dijkstra subtransaction",
			func() any { return &dijkstra.DijkstraSubTransactionBody{} },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := cbor.Decode(bodyCbor, test.dest())
			require.ErrorContains(t, err, "not a reward account")
		})
	}
}
