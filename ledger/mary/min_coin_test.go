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

package mary_test

import (
	"bytes"
	"math"
	"math/big"
	"math/bits"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minCoinOf(
	output mary.MaryTransactionOutput,
	params common.ProtocolParameters,
) (uint64, error) {
	return mary.MinCoinTxOut(&output, params)
}

func testMultiAsset(
	assets map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput,
) *common.MultiAsset[common.MultiAssetTypeOutput] {
	multiAsset := common.NewMultiAsset(assets)
	return &multiAsset
}

// Mary prices an output with scaledMinDeposit: exactly minUTxOValue for an
// ada-only value, otherwise
// max minUTxOValue (quot minUTxOValue 27 * (27 + size value)). The size is
// roundupBytesToWords(12 per asset + 28 per policy + the bytes of the distinct
// asset names) + 6, in 8-byte words. The expected values below are derived
// from that formula with minUTxOValue 1,000,000 (quot 1000000 27 = 37037), not
// from this implementation.
//
// Reference: scaledMinDeposit in
// eras/mary/impl/src/Cardano/Ledger/Mary/TxOut.hs, and size and
// representationSize in eras/mary/impl/src/Cardano/Ledger/Mary/Value.hs.
func TestMinCoinTxOutScalesWithValue(t *testing.T) {
	t.Parallel()
	const minUtxoValue = 1_000_000
	policyOne := common.NewBlake2b224(bytes.Repeat([]byte{0x01}, 28))
	policyTwo := common.NewBlake2b224(bytes.Repeat([]byte{0x02}, 28))
	name := func(s string) cbor.ByteString {
		return cbor.NewByteString([]byte(s))
	}
	one := big.NewInt(1)
	testCases := []struct {
		name    string
		assets  *common.MultiAsset[common.MultiAssetTypeOutput]
		minCoin uint64
	}{
		{
			name:    "ada only",
			minCoin: 1_000_000,
		},
		{
			// 12 + 28 + 0 = 40 bytes, 5 words, + 6 = 11; 37037 * 38
			name: "one policy, one asset with an empty name",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {name(""): one},
				},
			),
			minCoin: 1_407_406,
		},
		{
			// 12 + 28 + 5 = 45 bytes, 6 words, + 6 = 12; 37037 * 39
			name: "one policy, one asset with a 5-byte name",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {name("token"): one},
				},
			),
			minCoin: 1_444_443,
		},
		{
			// 36 + 28 + (1 + 2 + 3) = 70 bytes, 9 words, + 6 = 15;
			// 37037 * 42
			name: "one policy, three assets with names of 1, 2 and 3 bytes",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {
						name("a"):   one,
						name("bb"):  one,
						name("ccc"): one,
					},
				},
			),
			minCoin: 1_555_554,
		},
		{
			// 24 + 56 + 5 = 85 bytes, 11 words, + 6 = 17; 37037 * 44. A
			// name shared by two policies is counted once.
			name: "two policies sharing one 5-byte name",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {name("token"): one},
					policyTwo: {name("token"): one},
				},
			),
			minCoin: 1_629_628,
		},
		{
			// 48 + 56 + (32 + 10 + 4) = 150 bytes, 19 words, + 6 = 25;
			// 37037 * 52. The 32-byte name is shared and counted once.
			name: "two policies, four assets, one shared 32-byte name",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {
						name(string(bytes.Repeat([]byte{'a'}, 32))): one,
						name("bbbbbbbbbb"):                          one,
					},
					policyTwo: {
						name(string(bytes.Repeat([]byte{'a'}, 32))): one,
						name("cccc"): one,
					},
				},
			),
			minCoin: 1_925_924,
		},
		{
			// 24 + 28 + 64 = 116 bytes, 15 words, + 6 = 21; 37037 * 48
			name: "one policy, two assets with 32-byte names",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {
						name(string(bytes.Repeat([]byte{'a'}, 32))): one,
						name(string(bytes.Repeat([]byte{'b'}, 32))): one,
					},
				},
			),
			minCoin: 1_777_776,
		},
		{
			// The reference decoder drops zero quantities, so the value is
			// ada only.
			name: "zero quantity asset",
			assets: testMultiAsset(
				map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
					policyOne: {name("token"): big.NewInt(0)},
				},
			),
			minCoin: 1_000_000,
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			minCoin, err := minCoinOf(
				mary.MaryTransactionOutput{
					OutputAmount: mary.MaryTransactionOutputValue{
						Assets: testCase.assets,
					},
				},
				&mary.MaryProtocolParameters{MinUtxoValue: minUtxoValue},
			)
			require.NoError(t, err)
			assert.Equal(t, testCase.minCoin, minCoin)
		})
	}
}

// With a minUTxOValue below 27, quot minUTxOValue 27 is 0 and the scaled value
// is 0, so the minimum is minUTxOValue itself.
func TestMinCoinTxOutNeverBelowMinUtxoValue(t *testing.T) {
	t.Parallel()
	assets := testMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
			common.NewBlake2b224(bytes.Repeat([]byte{0x01}, 28)): {
				cbor.NewByteString([]byte("token")): big.NewInt(1),
			},
		},
	)
	minCoin, err := minCoinOf(
		mary.MaryTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{Assets: assets},
		},
		&mary.MaryProtocolParameters{MinUtxoValue: 10},
	)
	require.NoError(t, err)
	assert.Equal(t, uint64(10), minCoin)
}

func TestMinCoinTxOutOverflow(t *testing.T) {
	t.Parallel()
	if bits.UintSize < 64 {
		t.Skip("MinUtxoValue is 32 bits wide; the product cannot overflow")
	}
	assets := testMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
			common.NewBlake2b224(bytes.Repeat([]byte{0x01}, 28)): {
				cbor.NewByteString([]byte("token")): big.NewInt(1),
			},
		},
	)
	_, err := minCoinOf(
		mary.MaryTransactionOutput{
			OutputAmount: mary.MaryTransactionOutputValue{Assets: assets},
		},
		&mary.MaryProtocolParameters{MinUtxoValue: math.MaxUint},
	)
	require.ErrorContains(t, err, "overflow")
}

func TestMinCoinTxOutRejectsOtherParameters(t *testing.T) {
	t.Parallel()
	_, err := minCoinOf(
		mary.MaryTransactionOutput{},
		&shelley.ShelleyProtocolParameters{},
	)
	require.Error(t, err)
}
