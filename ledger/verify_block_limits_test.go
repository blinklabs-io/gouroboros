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
	"encoding/hex"
	"errors"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/stretchr/testify/require"
)

// blockLimitsTestHeaderHex is a real mainnet Conway block header (block
// 10882991, also used by TestVerifyBlockBody/TestVerifyBlock_TransactionValidation
// /TestVerifyBlock_StakePoolValidation in verify_block_test.go) whose VRF and
// KES both verify successfully against blockLimitsTestEta0Hex. Reusing it lets
// these tests exercise the real crypto-verification path in VerifyBlock while
// controlling the transaction bodies/witnesses (and therefore ExUnits and
// serialized sizes) precisely.
const blockLimitsTestHeaderHex = "828a1a00a60faf1a0817580c58204eac1e7264c0e80436b04687e75d46d6a0d6b2338c2abb73a14fafbd689f69b2582012209e0b93f0128f670c9a02781c5466c4c4be003da3a51344b6a94f709ce51f58209c1a5fc5dec0a4b822d5a3b254ce9b168299479127aadcf97506ef257517fff682584023c2d70c24c44041644f5152f7e8a1bb580e516eb8e73c7df287116adb5f009c0c001feccfeebdf34c2275d1fce859c6c46182631b6306d5fd2724ac7ab1c6be58500dbe31ef7c00c34b6522e983d223e05075359cb170668d960b8cebfced178287ee6ca5cfc6e8e60aec97fd197aebfefc24aae695680631d575c6dacdfd9efc5687e46eb2a5c04a755c7f260af9ef830819c5ea5820d2b74b6333637801f2e9c7265792d5b8fc1647f9056d67c769dbac27f25f2fd08458200946347d22a3b6da29d79102424973c932b898808ff2436fa138df102484230a0a1904165840c75619c3ebad0758349eb1dedc154a8cd280d8189d6da973b4a147b0cdb0f60442d493feeba64167a05b5fc40bc695192bf1c08afad3c07ebd33cb5925f378018209015901c00a8442332bd3f33a4d78fe2736a75110b528a1e7501bc7887910d1475fc0e425f49a84f94e98f87047916cf622f3db1f61b60c5f06709769f98c4cc67de8f50c320c6772b647ac9916765b6985d4eafccb54e71064d01df41f8d0638ed5cd62b7b6e49ba15dd87cc687ab87d3fb22490d355e8fa9c5f7c24ed88b800fcc4cb1f1b54e65b5ba82c442f4643caadc86583072b8b6956f4f9a4530c29873f7231605efd7a7f961a863530512ef86b50f9b1004748c31fa07978f2ece7d8e76ffde67d713015824b28e19f05f0383c2def3cdeb67247f33f5eae329c38a375b2eb06a586dcc2e102a776a6deaad1741f2a7f5aa604074698e876afab4455278fd84a1db5768078e2848cc85e3c8a0b48630a2622832ecd2dbb3c505df2a70b93b49ce99616f601e5e2004a8ce8926319c23f2a26ac8550cb1c05c9d2d25fc5fcd122fc35b057a71d6e961250c99b19a7bfd9acdc60a8151d6c81ef2d7d69a62fd0f17d184dd753cce9a2e9c32b53baf317e31c6c5e3cf8ea8b203b413ae8b0253db53d0cbe19b0f0547a0e67d3591d1cade6ceb4a47779ba4a09e7526280acb62200f42c98f6185ea9da3daf47aa3d10ffe5307331fa3430af6c6361154943c39375"

const blockLimitsTestEta0Hex = "4ef95a10f639d0cf16bb963c3a580d4bf2a95b6ae7848702665884843e3c661d"

const blockLimitsTestSlotsPerKesPeriod = uint64(129600)

// buildBlockLimitsTestBlock decodes the shared real header, attaches
// numTxs synthetic Conway transactions each carrying a single spend
// redeemer with the given ExUnits, encodes the whole block, and re-decodes
// it via the public API so block.Cbor()/header.Cbor() are populated from
// real serialized bytes (per project convention: measure from preserved
// Cbor() bytes, never re-encode-and-measure).
func buildBlockLimitsTestBlock(
	t *testing.T,
	numTxs int,
	perTxExUnits common.ExUnits,
) ledger.Block {
	t.Helper()

	headerCborBytes, err := hex.DecodeString(blockLimitsTestHeaderHex)
	require.NoError(t, err)
	header, err := ledger.NewBlockHeaderFromCbor(
		ledger.BlockTypeConway,
		headerCborBytes,
	)
	require.NoError(t, err)
	conwayHeader, ok := header.(*conway.ConwayBlockHeader)
	require.True(t, ok)

	txBody := conway.ConwayTransactionBody{
		TxFee: 200000,
	}
	witnessSet := conway.ConwayTransactionWitnessSet{
		WsRedeemers: conway.ConwayRedeemers{
			Redeemers: map[common.RedeemerKey]common.RedeemerValue{
				{Tag: common.RedeemerTagSpend, Index: 0}: {
					Data:    common.Datum{Data: data.NewInteger(big.NewInt(0))},
					ExUnits: perTxExUnits,
				},
			},
		},
	}

	bodies := make([]conway.ConwayTransactionBody, numTxs)
	witnesses := make([]conway.ConwayTransactionWitnessSet, numTxs)
	for i := 0; i < numTxs; i++ {
		bodies[i] = txBody
		witnesses[i] = witnessSet
	}

	craftedBlock := &conway.ConwayBlock{
		BlockHeader:            conwayHeader,
		TransactionBodies:      bodies,
		TransactionWitnessSets: witnesses,
		TransactionMetadataSet: common.TransactionMetadataSet{},
		InvalidTransactions:    []uint{},
	}

	blockCbor, err := cbor.Encode(craftedBlock)
	require.NoError(t, err)

	// SkipBodyHashValidation: the crafted body's hash won't match the real
	// header's declared body hash (this fixture swaps in synthetic
	// transactions), which is orthogonal to what these tests exercise.
	decodedBlock, err := ledger.NewBlockFromCbor(
		ledger.BlockTypeConway,
		blockCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return decodedBlock
}

func blockLimitsTestVerifyConfig(
	pp *conway.ConwayProtocolParameters,
) common.VerifyConfig {
	return common.VerifyConfig{
		SkipBodyHashValidation:    true,
		SkipTransactionValidation: true,
		SkipStakePoolValidation:   true,
		ProtocolParameters:        pp,
	}
}

// TestVerifyBlock_BlockLimits_Positive is the positive fixture: a block
// whose total ExUnits, body size, and header size are all comfortably
// within protocol maxima passes VerifyBlock.
func TestVerifyBlock_BlockLimits_Positive(t *testing.T) {
	block := buildBlockLimitsTestBlock(t, 3, common.ExUnits{
		Memory: 1_000_000,
		Steps:  100_000_000,
	})

	pp := &conway.ConwayProtocolParameters{
		MaxBlockBodySize:   90112,
		MaxBlockHeaderSize: 1100,
		MaxBlockExUnits: common.ExUnits{
			Memory: 62_000_000,
			Steps:  40_000_000_000,
		},
	}

	valid, _, _, _, err := ledger.VerifyBlock(
		block,
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		blockLimitsTestVerifyConfig(pp),
	)
	require.NoError(t, err)
	require.True(t, valid)
}

// TestVerifyBlock_BlockLimits_ExUnitsTooBig is the negative fixture where
// every transaction is individually valid (its ExUnits are confirmed under
// the era's real per-transaction budget check,
// conway.UtxoValidateExUnitsTooBigUtxo) but the block's total ExUnits
// exceeds ppMaxBlockExUnits.
func TestVerifyBlock_BlockLimits_ExUnitsTooBig(t *testing.T) {
	perTxExUnits := common.ExUnits{Memory: 1_500_000, Steps: 300_000_000}
	const numTxs = 3 // total: 4,500,000 memory / 900,000,000 steps

	// Confirm each transaction individually passes the real per-transaction
	// ExUnits rule against a generous per-transaction budget.
	perTxPparams := &conway.ConwayProtocolParameters{
		MaxTxExUnits: common.ExUnits{
			Memory: 2_000_000,
			Steps:  400_000_000,
		},
	}
	for i := 0; i < numTxs; i++ {
		tx := &conway.ConwayTransaction{
			WitnessSet: conway.ConwayTransactionWitnessSet{
				WsRedeemers: conway.ConwayRedeemers{
					Redeemers: map[common.RedeemerKey]common.RedeemerValue{
						{Tag: common.RedeemerTagSpend, Index: 0}: {
							ExUnits: perTxExUnits,
						},
					},
				},
			},
		}
		err := conway.UtxoValidateExUnitsTooBigUtxo(
			tx,
			0,
			nil,
			perTxPparams,
		)
		require.NoError(
			t,
			err,
			"transaction %d must be individually valid w.r.t. per-tx ExUnits",
			i,
		)
	}

	// Now build the same-shaped block and confirm the block-wide total is
	// rejected even though every transaction passed individually above.
	block := buildBlockLimitsTestBlock(t, numTxs, perTxExUnits)

	blockPparams := &conway.ConwayProtocolParameters{
		MaxBlockBodySize:   90112,
		MaxBlockHeaderSize: 1100,
		MaxBlockExUnits: common.ExUnits{
			Memory: 4_000_000,
			Steps:  800_000_000,
		},
	}

	valid, _, _, _, err := ledger.VerifyBlock(
		block,
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		blockLimitsTestVerifyConfig(blockPparams),
	)
	require.Error(t, err)
	require.False(t, valid)
	var exUnitsErr common.BlockExUnitsTooBigError
	require.True(
		t,
		errors.As(err, &exUnitsErr),
		"expected BlockExUnitsTooBigError, got %v",
		err,
	)
}

// TestVerifyBlock_BlockLimits_BodySizeTooBig is the negative fixture for a
// block whose serialized body size exceeds ppMaxBlockBodySize.
func TestVerifyBlock_BlockLimits_BodySizeTooBig(t *testing.T) {
	block := buildBlockLimitsTestBlock(t, 3, common.ExUnits{
		Memory: 1_000_000,
		Steps:  100_000_000,
	})

	bodySize, err := common.BlockBodySizeFromCbor(block.Cbor())
	require.NoError(t, err)
	require.Greater(t, bodySize, uint64(1))

	pp := &conway.ConwayProtocolParameters{
		// One byte under the real serialized body size.
		MaxBlockBodySize:   uint(bodySize - 1),
		MaxBlockHeaderSize: 1100,
		MaxBlockExUnits: common.ExUnits{
			Memory: 62_000_000,
			Steps:  40_000_000_000,
		},
	}

	valid, _, _, _, err := ledger.VerifyBlock(
		block,
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		blockLimitsTestVerifyConfig(pp),
	)
	require.Error(t, err)
	require.False(t, valid)
	var bodySizeErr common.BlockBodySizeTooBigError
	require.True(
		t,
		errors.As(err, &bodySizeErr),
		"expected BlockBodySizeTooBigError, got %v",
		err,
	)
}

// TestVerifyBlock_BlockLimits_HeaderSizeTooBig is the negative fixture for
// a block whose serialized header size exceeds ppMaxBlockHeaderSize.
func TestVerifyBlock_BlockLimits_HeaderSizeTooBig(t *testing.T) {
	block := buildBlockLimitsTestBlock(t, 3, common.ExUnits{
		Memory: 1_000_000,
		Steps:  100_000_000,
	})

	headerSize := uint64(len(block.Header().Cbor()))
	require.Greater(t, headerSize, uint64(1))

	pp := &conway.ConwayProtocolParameters{
		MaxBlockBodySize: 90112,
		// One byte under the real serialized header size.
		MaxBlockHeaderSize: uint(headerSize - 1),
		MaxBlockExUnits: common.ExUnits{
			Memory: 62_000_000,
			Steps:  40_000_000_000,
		},
	}

	valid, _, _, _, err := ledger.VerifyBlock(
		block,
		blockLimitsTestEta0Hex,
		blockLimitsTestSlotsPerKesPeriod,
		blockLimitsTestVerifyConfig(pp),
	)
	require.Error(t, err)
	require.False(t, valid)
	var headerSizeErr common.BlockHeaderSizeTooBigError
	require.True(
		t,
		errors.As(err, &headerSizeErr),
		"expected BlockHeaderSizeTooBigError, got %v",
		err,
	)
}
