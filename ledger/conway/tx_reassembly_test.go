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

package conway_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

const conwayPoolRegistrationExampleTxCborHex = "84a500d901028282582004d97ebdeb064082639d67c8318ce069a35983bb05782d1327b004cca330ab5b008258204430e4bc2db0ef794c70b79851eecc332d8f77fb022c0d03ad24797f390ae54f000181825839005e7faca37d22d8753db699b104cbb2586f8787e17c116ff254ef0401e669129d1393c159b9b5a84d894271b5689910cc2e364ca05771988d1b0000000487a0103c021a0002d719031a0661906704d90102818a03581c7f4a5ac4b6a0f40cf07f989238d8e623315d80cc0602255b15c01eb3582025b400987b8e6d3f2d1913f7e7179611dc6563dc6731064de6b6dbe05114006e1b00000002540be4001a1908b100d81e82151901f4581de0e669129d1393c159b9b5a84d894271b5689910cc2e364ca05771988dd9010281581ce669129d1393c159b9b5a84d894271b5689910cc2e364ca05771988d818400190bb9444017f8d6f6827668747470733a2f2f6269742e6c792f34634e34374d31582086ed8edc5e20678c124d49dd1f6f6cb0b358797b71586f8a9db36bccf313f9eea100d9010283825820e61a0ef75ebcfba9569f2ef450d50320f376c36056f09f759d0e18ebf30a5ece5840c329a870e41de8e59b3ec872ec8d06f10e19c5dc436311e409827bf5792f86e75bb2c46785991563f42a03498c9c5342957efa15b348fffbd38f4fe64aef4f01825820942aaf02196ca16a79483b5862ff3d521e4c62c24dbc6aa495a360c101249de3584071ea7ed1740fbabe61f9c73f7306ef1ade9c2cf07a9d3c75d3ca130dd7e2078ea687cc326e7e790038580fdb3d9ec8e7e0edf70f5ff47527dd5ae0de6f5eca04825820eb2dbcf867f0611ca671a3ce89ae6c89a1a2eea96d6dcba82c607d4c9dbc489e5840f7e9a45d24cfbe8a7e7bc8200d84aa914cb51448873a41e0cf80aa641dd266490a0568b3039377fc5836d94320dc5c125f56352e0ad529f518035b4c2a313102f5f6"

const previewPoolRegistrationTxCborHex = "84a500d9010281825820d5d133a93beb5e8c2ba2cdf070e657968a38f5c23a6209cb93296abd970f249c010181825839002b27776f6ab2550f9df9220e2dfe0bc9afc270e820a61f548e5f3effd3034b07165afa8334932d0c69826d3935beec2337306115f0c66b3e1b0000000235f3b7e4021a0002da05031a0661c22c04d90102828a03581cd5aa3168bf06e67559d38e91f8dbf7ccb7013fa94e56e2341fa0cb7e582091da5ccee5912ab0d5789a708fecbd957fdb17b462deb77e8cb26e69fcef63431a000f42401a0a21fe80d81e82011864581de0d3034b07165afa8334932d0c69826d3935beec2337306115f0c66b3ed9010281581cd3034b07165afa8334932d0c69826d3935beec2337306115f0c66b3e818400193e814460d9aec3f6827068747470733a2f2f666f6e7a2e636f6d582011f1f9834c419f22f3fed61d02a1d3e87bcb97740764734963f6695c451fb01583028200581cd3034b07165afa8334932d0c69826d3935beec2337306115f0c66b3e581cd5aa3168bf06e67559d38e91f8dbf7ccb7013fa94e56e2341fa0cb7ea100d901028382582041b65112e7d61c5c1ad55abc27df3602321dba74c07eb5a19bedec9ee60b9044584008df0e40b06ad1c73d1d3c8eb4c76be453c16b7a66b44fc4c939134633ee70d199e7fded4c26b7b6b9d69d919182636f56cb611a831ae2f4563cd22173ae9c00825820a43be7f457242ce44a7bf1b1a072f284d599d5ad382672621f0560c19910a770584064da6abaccc703adabd0e76e27ba5f8553e9c3815ea73c88411a0ce00e549657ef2539379ab4ba479dddfd4478aab8c45085bbcd1f97c46162c673aa2ac3850582582090efbd49cbcfc0d1df1003d485e216319071dba64df57028e0b4bba647a54d335840351f090bd1c23930f72dccd25d823cfebcac1a8a633a9516764cb66154ff477b14fe53873ca63cd259f1a0d438e7cd5ff10c6c4b28890818f521e602fc007802f5f6"

func TestConwayTransactionCborReassemblyPreservesRawBytes(t *testing.T) {
	testDefs := []struct {
		name   string
		txCbor []byte
	}{
		{
			name:   "preview_pool_registration_tx",
			txCbor: previewPoolRegistrationTxCbor(t),
		},
		{
			name:   "alternate_pool_registration_tx",
			txCbor: mustDecodeHex(t, conwayPoolRegistrationExampleTxCborHex),
		},
	}

	for _, testDef := range testDefs {
		t.Run(testDef.name, func(t *testing.T) {
			tx, err := conway.NewConwayTransactionFromCbor(testDef.txCbor)
			require.NoError(t, err)

			// Force the raw-component reassembly path instead of returning the
			// stored full transaction bytes.
			tx.SetCbor(nil)

			require.True(t, bytes.Equal(testDef.txCbor, tx.Cbor()))
			require.Len(t, tx.Cbor(), len(testDef.txCbor))
		})
	}
}

func TestConwayBlockTransactionsPreserveOriginalTxBytes(
	t *testing.T,
) {
	txCbor := previewPoolRegistrationTxCbor(t)
	blockCbor := syntheticConwayBlockWithTransaction(t, txCbor)

	block, err := conway.NewConwayBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	txs := block.Transactions()
	require.Len(t, txs, 1)
	require.Equal(t, txCbor, txs[0].Cbor())
	require.Len(t, txs[0].Cbor(), len(txCbor))
}

func TestConwayMinFeeTxUsesOriginalSizeAfterBlockDecode(
	t *testing.T,
) {
	// This real preview pool registration transaction previously measured 7 bytes
	// too large after block decode, which inflated min-fee validation.
	txCbor := previewPoolRegistrationTxCbor(t)
	blockCbor := syntheticConwayBlockWithTransaction(t, txCbor)

	block, err := conway.NewConwayBlockFromCbor(
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	txs := block.Transactions()
	require.Len(t, txs, 1)

	txSize, err := common.TxSizeForFee(txs[0])
	require.NoError(t, err)
	require.Equal(t, len(txCbor)-1, txSize)
	require.Equal(t, 716, txSize)

	minFee, err := conway.MinFeeTx(
		txs[0],
		&conway.ConwayProtocolParameters{
			MinFeeA: 44,
			MinFeeB: 155381,
		},
	)
	require.NoError(t, err)
	require.Equal(t, uint64(186885), minFee)
}

func previewPoolRegistrationTxCbor(t *testing.T) []byte {
	t.Helper()

	txCbor := mustDecodeHex(t, previewPoolRegistrationTxCborHex)
	require.Len(t, txCbor, 717)
	return txCbor
}

func mustDecodeHex(t *testing.T, value string) []byte {
	t.Helper()

	decoded, err := hex.DecodeString(value)
	require.NoError(t, err)
	return decoded
}

func syntheticConwayBlockWithTransaction(t *testing.T, txCbor []byte) []byte {
	t.Helper()

	templateBlockCbor, err := hex.DecodeString(strings.TrimSpace(testdata.ConwayBlockHex))
	require.NoError(t, err)

	var blockComponents []cbor.RawMessage
	_, err = cbor.Decode(templateBlockCbor, &blockComponents)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(blockComponents), 5)

	var txComponents []cbor.RawMessage
	_, err = cbor.Decode(txCbor, &txComponents)
	require.NoError(t, err)
	require.Len(t, txComponents, 4)

	var isValid bool
	_, err = cbor.Decode(txComponents[2], &isValid)
	require.NoError(t, err)

	metadataSet := map[uint]cbor.RawMessage{}
	if len(txComponents[3]) > 0 && txComponents[3][0] != 0xf6 {
		metadataSet[0] = txComponents[3]
	}
	invalidTransactions := []uint{}
	if !isValid {
		invalidTransactions = []uint{0}
	}

	blockCbor, err := cbor.Encode([]any{
		cbor.RawMessage(blockComponents[0]),
		[]any{cbor.RawMessage(txComponents[0])},
		[]any{cbor.RawMessage(txComponents[1])},
		metadataSet,
		invalidTransactions,
	})
	require.NoError(t, err)
	return blockCbor
}
