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

package byron

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mainnet Byron main block
// 7061f3fe3fe335b569f91b13e0a2fadfbd4f052f62ec7db9743e2838c8e4c30e at slot
// 4341573, fetched over BlockFetch from a mainnet relay. It carries
// three transactions, which is the smallest count where cardano-ledger's
// split-at-the-largest-power-of-two tree and a duplicate-last-padded tree
// disagree: a power-of-two count cannot tell them apart, which is why the
// two-transaction fixture in validate_test.go never caught this.
const testByronMainBlockThreeTxHex = "83851a2d964a09582004c26ff8239b6a7b60cc002a5ac3f1dc97b874fc08787bbfb941173f7ea220ae8483035820334a7b973b88e88f4c29d3bc55572995c64d990bc2df2d1748c29648e64914525820dcf3841fae1716ab133f09c1c199a8735e1625c49a99308849e7785f037e5c8182035820d36a2619a672494604e11bb447cbcf5231e9f2ba25c2169177edc941bd50ad6c5820afc0da64183bf2664f3d4eec7238d524ba607faeeab24fc100eb861dba69971b58204e66280cd94d591072349bec0a3090a53aa945562efb6d08d56e53654b0e4098848218c8195445584026566e86fc6b9b177c8480e275b2b112b573f6d073f9deea53b8d99c4ed976b335b2b3842f0e380001f090bc923caa9691ed9115e286da9421e2745c7acc87f1811a004236d28202828400584026566e86fc6b9b177c8480e275b2b112b573f6d073f9deea53b8d99c4ed976b335b2b3842f0e380001f090bc923caa9691ed9115e286da9421e2745c7acc87f15840f14f712dc600d793052d4842d50cefa4e65884ea6cf83707079eb8ce302efc85dae922d5eb3838d2b91784f04824d26767bfb65bd36a36e74fec46d09d98858d58408ab43e904b06e799c1817c5ced4f3a7bbe15cdbf422dea9d2d5dc2c6105ce2f4d4c71e5d4779f6c44b770a133636109949e1f7786acb5a732bcdea0470fea4065840b60013bd424c18be322e15a02fd29e988fda44810bce2f4caef039d899092834d1afbd7c7c42dd1c17628f90c44d72c75d3475c951787bc6efbebda5e0d73e088483010000826a63617264616e6f2d736c02a058204ba92aa320c60acc9ad7b9a64f2eda55c4d2ec28e604faf186708b4f0c4e8edf849f82839f8200d81858248258203db223f2b11378ce0849a224cb9427e29c6634b7808b0535b0e588b35693002a08ff9f8282d818584283581c7d71ead512f6385150e01a98dab98100c862a0d9bdc2cce4a6c57ad8a101581e581c2b0b011ba3683d0013f3702a6218f57800516faf7c668af98d9db1fa001a1987ec041b0000003a19d9921d8282d818584283581cb1850b1758b1cb5e12e56e57d7f89745fbf6acbf941d20f3bf463f4aa101581e581c8971cd80beecfe13bef68427d02dbb80bd1f782c68bf130ac228f797001a596364141a1b3dcec2ffa0818200d8185885825840ed78e852aab54bfd819f57833ae4a97b3656b7c1e039aa04849fc1fce4b1797d7a02550eb05a0430a4c3298979a680300e20d2d630155a045eaeacd5fcaa26ec5840a2ef2e3ec15a7eae64793861ad1988b7f59749f243587ce8010be64a210f6c1144f018596433926421aa7d2f5967c2db374d9e928bf7f15d8d317263a638700882839f8200d818582482582085cdc3d43a485d5ad862189ca73517eb21cd3db4b9ade3527f5f4ccb59b36c5c018200d818582482582010e53e52d27e743d89b4a19cae19368f99cf99a639f14920daeaa11480f113bf06ff9f8282d818582183581c86d457e1f1be3cb79520306895a0771575bf917738dd3788d6c310dea0001a7cf63fc21a2b8a88ee8282d818582183581c85bbdd8a68b5b856340f8684dbec8c9dfa50fbb5fe5d081278456c00a0001a7b22feaa1b00000001cd5c4ad9ffa0828200d81858858258407daa435bca55d3ee5b8a0e772d2e58f565f4a3ef6d476616bae23ea55f1c45417e8c773b2baf47028beb500752852b06437f9a03893777e8fba9200b4e92a55958402448f50d58d35706c1b83352f2febd505dfa8bef9c2f69a3f525fcd422fe937ba9160d4661a71477f6368e3a2758f04114c302333a954b809a45783d98c0970b8200d81858858258407daa435bca55d3ee5b8a0e772d2e58f565f4a3ef6d476616bae23ea55f1c45417e8c773b2baf47028beb500752852b06437f9a03893777e8fba9200b4e92a55958402448f50d58d35706c1b83352f2febd505dfa8bef9c2f69a3f525fcd422fe937ba9160d4661a71477f6368e3a2758f04114c302333a954b809a45783d98c0970b82839f8200d818582482582001e1feeb951bcc39bfc57d341bfff783c3d8280152d05b376fe0804ba86a2969008200d81858248258205791d28f92fe4aa76fcb14e74cd97723f1b1d67181eee3a625ee3b7e89c6b19600ff9f8282d818582183581ce92a816a8fb033ec09a7a5b0c7d5fd47da91af9bacb3ab4bd8fd4254a0001aacc422631b000000012a05f2008282d818584283581c5370e18238171004f9a547323fb17f447e02baf98051a22b94f0d500a101581e581c78ee04194d1b7531fab575961ed8715cc1b18a198496560598d9b465001a904f4d0f1b00000004b6e247faffa0828200d81858858258408404c44d61c9d4d9d9708a34076ab979a3ff4e1c08179412e55a0d7ec6350762238a130eb0e563e8958f8fff5a76e52e16700e4bc887f363e6aae62b82595dc75840a5eab373676f5b5b56e3ac3cbf0ae72c37ae1f14ea993a6f7bae01224bb44a2675df09b17b4e78274be94875178dc3e1a62bad817cd987806fe4af15e9da690c8200d81858858258408404c44d61c9d4d9d9708a34076ab979a3ff4e1c08179412e55a0d7ec6350762238a130eb0e563e8958f8fff5a76e52e16700e4bc887f363e6aae62b82595dc75840a5eab373676f5b5b56e3ac3cbf0ae72c37ae1f14ea993a6f7bae01224bb44a2675df09b17b4e78274be94875178dc3e1a62bad817cd987806fe4af15e9da690cff8203d90102809fff82809fff81a0"

// The txProof merkle root this block carries on chain. It is a chain
// artifact, not a value recomputed by either implementation here.
const testByronThreeTxMerkleRootHex = "334a7b973b88e88f4c29d3bc55572995c64d990bc2df2d1748c29648e6491452"

func decodeThreeTxBlock(t *testing.T, blockHex string) *byron.ByronMainBlock {
	t.Helper()
	blockBytes, err := hex.DecodeString(blockHex)
	require.NoError(t, err)
	// Decoding normally runs ValidateBodyProof; skip it so the consensus
	// package's own validator is what the assertions below observe.
	block, err := byron.NewByronMainBlockFromCbor(
		blockBytes,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	return block
}

// TestValidateBodyHashThreeTransactionBlock is the regression for a genuine
// three-transaction mainnet block being rejected by ValidateBodyHash.
func TestValidateBodyHashThreeTransactionBlock(t *testing.T) {
	t.Parallel()
	block := decodeThreeTxBlock(t, testByronMainBlockThreeTxHex)
	require.Len(t, block.Body.TxPayload, 3)

	proof, err := parseByronBodyProof(block.BlockHeader.BodyProof)
	require.NoError(t, err)
	require.Equal(
		t,
		testByronThreeTxMerkleRootHex,
		proof.TxProof.TxBodyMerkleRoot.String(),
	)

	// ledger/byron.MerkleRoot implements cardano-ledger's
	// Cardano.Chain.Common.Merkle; reproducing the on-chain root from the
	// preserved transaction-body CBOR is what pins it to the reference.
	bodies := make([][]byte, 0, len(block.Body.TxPayload))
	for i := range block.Body.TxPayload {
		bodies = append(bodies, block.Body.TxPayload[i].Body.Cbor())
	}
	assert.Equal(
		t,
		proof.TxProof.TxBodyMerkleRoot,
		byron.MerkleRoot(bodies),
	)

	require.NoError(t, ValidateBodyHash(block))
	// The ledger package's independent validator must agree.
	require.NoError(t, block.ValidateBodyProof())
}

// TestValidateBodyHashRejectsWrongTxMerkleRoot is the negative control for
// the test above: the same block with a different txProof merkle root must
// be rejected, so that accepting the genuine block is not merely a validator
// that accepts everything.
func TestValidateBodyHashRejectsWrongTxMerkleRoot(t *testing.T) {
	t.Parallel()
	const wrongRootHex = "0000000000000000000000000000000000000000000000000000000000000001"
	require.Equal(
		t,
		1,
		strings.Count(
			testByronMainBlockThreeTxHex,
			testByronThreeTxMerkleRootHex,
		),
	)
	patched := strings.Replace(
		testByronMainBlockThreeTxHex,
		testByronThreeTxMerkleRootHex,
		wrongRootHex,
		1,
	)
	block := decodeThreeTxBlock(t, patched)

	err := ValidateBodyHash(block)
	require.Error(t, err)
	var valErr *common.ValidationError
	require.ErrorAs(t, err, &valErr)
	require.NotNil(t, valErr)
	assert.Equal(t, common.ValidationErrorTypeBodyHash, valErr.Type)
	assert.Equal(t, "transaction body merkle root mismatch", valErr.Message)
}
