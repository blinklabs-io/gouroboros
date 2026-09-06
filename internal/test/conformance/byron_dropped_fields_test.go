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

package conformance

import (
	"math/big"
	"path"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// byronGoldenDir is the CardanoNodeToNodeVersion2 golden directory in the
// embedded upstream fixtures. Each Block_* golden is
// #6.24(bytes .cbor [block_type, block]).
const byronGoldenDir = "upstream/ouroboros-consensus/ouroboros-consensus-cardano/golden/cardano/CardanoNodeToNodeVersion2"

// readByronGoldenBlock returns the node-to-client block type and the block
// CBOR carried by an upstream Block_Byron_* golden.
func readByronGoldenBlock(t *testing.T, name string) (uint, []byte) {
	t.Helper()
	raw, err := fixtures.EmbeddedFixtures().
		ReadFile(path.Join(byronGoldenDir, name))
	require.NoError(t, err, "read golden %s", name)
	var wrapper cbor.Tag
	_, err = cbor.Decode(raw, &wrapper)
	require.NoError(t, err, "decode golden wrapper %s", name)
	inner, ok := wrapper.Content.([]byte)
	require.Truef(t, ok, "golden %s tag content is %T, want []byte", name, wrapper.Content)
	var envelope struct {
		cbor.StructAsArray
		BlockType uint
		Block     cbor.RawMessage
	}
	_, err = cbor.Decode(inner, &envelope)
	require.NoError(t, err, "decode golden envelope %s", name)
	return envelope.BlockType, []byte(envelope.Block)
}

// setNested replaces the CBOR element reached by walking indices through
// nested definite-length arrays, leaving every other byte untouched.
func setNested(
	t *testing.T,
	data []byte,
	indices []int,
	value cbor.RawMessage,
) []byte {
	t.Helper()
	require.NotEmpty(t, indices)
	var elements []cbor.RawMessage
	_, err := cbor.Decode(data, &elements)
	require.NoError(t, err, "decode array at %v", indices)
	require.Greaterf(
		t, len(elements), indices[0],
		"array has %d elements, need index %d", len(elements), indices[0],
	)
	if len(indices) == 1 {
		elements[indices[0]] = value
	} else {
		elements[indices[0]] = setNested(
			t, elements[indices[0]], indices[1:], value,
		)
	}
	encoded, err := cbor.Encode(elements)
	require.NoError(t, err)
	return encoded
}

func mustEncodeCbor(t *testing.T, value any) cbor.RawMessage {
	t.Helper()
	encoded, err := cbor.Encode(value)
	require.NoError(t, err)
	return encoded
}

// TestByronGoldenBlockDroppedHeaderFields covers the Byron main block header
// fields cardano-ledger drops or decodes wider than gouroboros did.
//
// decCBORBlockVersions ends with dropBytes, so the extra data proof is a
// byte string of any length that is never interpreted
// (Cardano/Chain/Block/Header.hs:392-395; dropBytes = void decodeBytes,
// Cardano/Ledger/Binary/Decoding/Drop.hs:28-29). The slot within the epoch
// is SlotCount, a Word64 newtype with a derived DecCBOR, and toSlotNumber
// adds it to the flattened epoch with no bound of its own
// (Cardano/Chain/Slotting/SlotCount.hs:15-19,
// Cardano/Chain/Slotting/EpochAndSlotCount.hs:38-41, :71-77).
func TestByronGoldenBlockDroppedHeaderFields(t *testing.T) {
	blockType, blockCbor := readByronGoldenBlock(t, "Block_Byron_regular")
	require.Equal(t, uint(byron.BlockTypeByronMain), blockType)
	require.NotNil(t, mustDecodeByronBlock(t, blockType, blockCbor))

	t.Run("extra data proof of any length", func(t *testing.T) {
		// header[4] is ExtraData; extraData[3] is the dropped proof.
		mutated := setNested(
			t, blockCbor, []int{0, 4, 3},
			mustEncodeCbor(t, make([]byte, common.Blake2b256Size-1)),
		)
		block := mustDecodeByronBlock(t, blockType, mutated)
		header, ok := block.Header().(*byron.ByronMainBlockHeader)
		require.True(t, ok)
		assert.Len(t, header.ExtraData.ExtraProof, common.Blake2b256Size-1)
	})

	t.Run("extra data proof must still be a byte string", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, []int{0, 4, 3}, mustEncodeCbor(t, uint64(0)),
		)
		_, err := ledger.NewBlockFromCbor(blockType, mutated)
		require.Error(t, err)
	})

	t.Run("slot within epoch above 2^16", func(t *testing.T) {
		// header[3] is ConsensusData; consensusData[0] is SlotId;
		// slotId[1] is the slot count within the epoch.
		const slot = uint64(70000)
		mutated := setNested(
			t, blockCbor, []int{0, 3, 0, 1}, mustEncodeCbor(t, slot),
		)
		block := mustDecodeByronBlock(t, blockType, mutated)
		header, ok := block.Header().(*byron.ByronMainBlockHeader)
		require.True(t, ok)
		assert.Equal(t, slot, header.ConsensusData.SlotId.Slot)
		assert.Equal(
			t,
			header.ConsensusData.SlotId.Epoch*byron.ByronSlotsPerEpoch+slot,
			header.SlotNumber(),
		)
	})

	t.Run("slot within epoch must still be unsigned", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, []int{0, 3, 0, 1},
			cbor.RawMessage{0x20}, // -1
		)
		_, err := ledger.NewBlockFromCbor(blockType, mutated)
		require.Error(t, err)
	})
}

// TestByronGoldenEbbDroppedFields covers the Byron epoch boundary block
// fields cardano-ledger drops. decCBORABoundaryHeader opens with dropInt32,
// which accepts the full signed 32-bit range and never interprets the value
// (Cardano/Chain/Block/Header.hs:613-616), and dropBoundaryBody is
// dropList dropBytes, so each body entry is a byte string of any length
// (Cardano/Chain/Block/Boundary.hs:73-74).
func TestByronGoldenEbbDroppedFields(t *testing.T) {
	blockType, blockCbor := readByronGoldenBlock(t, "Block_Byron_EBB")
	require.Equal(t, uint(byron.BlockTypeByronEbb), blockType)
	require.NotNil(t, mustDecodeByronBlock(t, blockType, blockCbor))

	// The body proof binds the body bytes to the header, so a mutated body
	// needs that check skipped to reach the field decode under test.
	skipBodyHash := common.VerifyConfig{SkipBodyHashValidation: true}

	t.Run("body entries of any length", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, []int{1},
			mustEncodeCbor(t, [][]byte{make([]byte, common.Blake2b256Size)}),
		)
		decoded, err := ledger.NewBlockFromCbor(blockType, mutated, skipBodyHash)
		require.NoError(t, err)
		ebb, ok := decoded.(*byron.ByronEpochBoundaryBlock)
		require.True(t, ok)
		require.Len(t, ebb.Body, 1)
		assert.Len(t, ebb.Body[0], common.Blake2b256Size)
	})

	t.Run("body entries must still be byte strings", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, []int{1}, mustEncodeCbor(t, []uint64{1}),
		)
		_, err := ledger.NewBlockFromCbor(blockType, mutated, skipBodyHash)
		require.Error(t, err)
	})

	t.Run("negative protocol magic", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, []int{0, 0}, mustEncodeCbor(t, int64(-5)),
		)
		decoded, err := ledger.NewBlockFromCbor(blockType, mutated)
		require.NoError(t, err)
		ebb, ok := decoded.(*byron.ByronEpochBoundaryBlock)
		require.True(t, ok)
		assert.Equal(t, int32(-5), ebb.BlockHeader.ProtocolMagic)
	})

	t.Run("protocol magic must still fit int32", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, []int{0, 0},
			mustEncodeCbor(t, uint64(1)<<32),
		)
		_, err := ledger.NewBlockFromCbor(blockType, mutated)
		require.Error(t, err)
	})
}

func mustDecodeByronBlock(
	t *testing.T,
	blockType uint,
	blockCbor []byte,
) common.Block {
	t.Helper()
	block, err := ledger.NewBlockFromCbor(blockType, blockCbor)
	require.NoError(t, err)
	return block
}

// TestByronUpdateProposalParametersAreNatural covers the five update
// proposal protocol parameters cardano-ledger types as Maybe Natural, an
// unbounded non-negative integer decoded by a bare decCBOR
// (Cardano/Chain/Update/ProtocolParametersUpdate.hs:36-42, :135-152).
// decodeNatural accepts any integer and rejects only negatives
// (Cardano/Ledger/Binary/Decoding/Decoder.hs:1463-1469).
//
// No upstream golden carries a non-empty Byron update payload, so the
// fourteen-element ProtocolParametersUpdate encoding is constructed here
// from that field order, with each Maybe as the zero- or one-element list
// the Byron encoder writes.
func TestByronUpdateProposalParametersAreNatural(t *testing.T) {
	// 2^64, one past the widest value the previous uint64 fields held.
	twoPow64 := new(big.Int).Lsh(big.NewInt(1), 64)

	encodeMod := func(t *testing.T, maxBlockSize cbor.RawMessage) []byte {
		t.Helper()
		fields := make([]any, 14)
		for i := range fields {
			fields[i] = []any{}
		}
		fields[2] = maxBlockSize
		encoded, err := cbor.Encode(fields)
		require.NoError(t, err)
		return encoded
	}

	t.Run("value above 2^64", func(t *testing.T) {
		var mod byron.ByronUpdateProposalBlockVersionMod
		_, err := cbor.Decode(
			encodeMod(t, mustEncodeCbor(t, []*big.Int{twoPow64})), &mod,
		)
		require.NoError(t, err)
		require.Len(t, mod.MaxBlockSize, 1)
		assert.Zero(t, mod.MaxBlockSize[0].Cmp(twoPow64))
	})

	t.Run("ordinary value", func(t *testing.T) {
		var mod byron.ByronUpdateProposalBlockVersionMod
		_, err := cbor.Decode(
			encodeMod(t, mustEncodeCbor(t, []uint64{2000000})), &mod,
		)
		require.NoError(t, err)
		require.Len(t, mod.MaxBlockSize, 1)
		assert.Equal(t, "2000000", mod.MaxBlockSize[0].String())
	})

	t.Run("absent value", func(t *testing.T) {
		var mod byron.ByronUpdateProposalBlockVersionMod
		_, err := cbor.Decode(encodeMod(t, mustEncodeCbor(t, []any{})), &mod)
		require.NoError(t, err)
		assert.Empty(t, mod.MaxBlockSize)
	})

	t.Run("negative value rejected", func(t *testing.T) {
		var mod byron.ByronUpdateProposalBlockVersionMod
		_, err := cbor.Decode(
			encodeMod(t, mustEncodeCbor(t, []int64{-1})), &mod,
		)
		require.Error(t, err)
	})

	t.Run("non-integer value rejected", func(t *testing.T) {
		var mod byron.ByronUpdateProposalBlockVersionMod
		_, err := cbor.Decode(
			encodeMod(t, mustEncodeCbor(t, [][]byte{{0x00}})), &mod,
		)
		require.Error(t, err)
	})
}

// TestByronGoldenBlockSscProofHashSlots covers the ssc_proof hash slots.
// dropSscProof reads each of them with dropBytes -- a byte string of any
// length, never interpreted -- and SscProof is a unit type that compares
// nothing (Cardano/Chain/Ssc.hs:164-188). dropSscProof and dropSscPayload
// are separate decoders over separate fields and never compare their tags
// (:70-95), so neither the hash length nor the proof-against-payload type
// agreement can fail a decode.
func TestByronGoldenBlockSscProofHashSlots(t *testing.T) {
	blockType, blockCbor := readByronGoldenBlock(t, "Block_Byron_regular")

	// header[2] is the body proof; proof[1] is the ssc proof.
	sscProofPath := []int{0, 2, 1}

	t.Run("hash slot of any length", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, append(sscProofPath, 1),
			mustEncodeCbor(t, make([]byte, common.Blake2b256Size-1)),
		)
		mustDecodeByronBlock(t, blockType, mutated)
	})

	t.Run("hash slot must still be a byte string", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, append(sscProofPath, 1),
			mustEncodeCbor(t, uint64(0)),
		)
		_, err := ledger.NewBlockFromCbor(blockType, mutated)
		require.Error(t, err)
	})

	t.Run("unknown proof tag still rejected", func(t *testing.T) {
		mutated := setNested(
			t, blockCbor, append(sscProofPath, 0),
			mustEncodeCbor(t, uint64(4)),
		)
		_, err := ledger.NewBlockFromCbor(blockType, mutated)
		require.Error(t, err)
	})
}
