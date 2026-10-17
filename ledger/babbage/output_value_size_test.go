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

package babbage_test

import (
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

// indefiniteMapHeaderOutputValue returns a one-policy output value with 256
// assets and its encoded size. The encoded map uses the same indefinite-length
// header cardano-ledger uses to measure this value.
func indefiniteMapHeaderOutputValue(
	t *testing.T,
) (mary.MaryTransactionOutputValue, uint) {
	t.Helper()
	assets := make(map[cbor.ByteString]*big.Int, 256)
	for i := range 256 {
		assets[cbor.NewByteString([]byte{byte(i >> 8), byte(i)})] = big.NewInt(1)
	}
	multiAsset := common.NewMultiAsset(
		map[common.Blake2b224]map[cbor.ByteString]*big.Int{{}: assets},
	)
	value := mary.MaryTransactionOutputValue{Amount: 1, Assets: &multiAsset}
	encoded, err := cbor.Encode(&value)
	require.NoError(t, err)
	return value, uint(len(encoded))
}

func TestUtxoValidateOutputTooBigUtxoIndefiniteMapHeader(t *testing.T) {
	t.Parallel()
	value, encodedSize := indefiniteMapHeaderOutputValue(t)
	testCases := []struct {
		name         string
		maxValueSize uint
		wantErr      bool
	}{
		{"ledger size equals limit", encodedSize, false},
		{"ledger size above limit", encodedSize - 1, true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tx := &babbage.BabbageTransaction{
				Body: babbage.BabbageTransactionBody{
					TxOutputs: []babbage.BabbageTransactionOutput{
						{OutputAmount: value},
					},
				},
			}
			err := babbage.UtxoValidateOutputTooBigUtxo(
				tx,
				0,
				mockledger.NewLedgerStateBuilder().Build(),
				&babbage.BabbageProtocolParameters{MaxValueSize: tc.maxValueSize},
			)
			if tc.wantErr {
				var tooBig mary.OutputTooBigUtxoError
				require.ErrorAs(t, err, &tooBig)
				return
			}
			require.NoError(t, err)
		})
	}
}
