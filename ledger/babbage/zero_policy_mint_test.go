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

func TestUtxoValidateValueNotConservedChecksZeroPolicyAsset(t *testing.T) {
	mint := common.NewMultiAsset[common.MultiAssetTypeMint](map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeMint{
		{}: {cbor.NewByteString([]byte("token")): big.NewInt(1)},
	})
	assets := common.NewMultiAsset[common.MultiAssetTypeOutput](map[common.Blake2b224]map[cbor.ByteString]common.MultiAssetTypeOutput{
		{}: {cbor.NewByteString([]byte("token")): big.NewInt(1)},
	})
	tx := &babbage.BabbageTransaction{Body: babbage.BabbageTransactionBody{
		TxMint: &mint,
		TxOutputs: []babbage.BabbageTransactionOutput{{
			OutputAmount: mary.MaryTransactionOutputValue{Assets: &assets},
		}},
	}}
	require.NoError(t, babbage.UtxoValidateValueNotConservedUtxo(
		tx, 0, mockledger.NewLedgerStateBuilder().Build(),
		&babbage.BabbageProtocolParameters{},
	))
}
