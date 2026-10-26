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

package blockfetch

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	ledgerbyron "github.com/blinklabs-io/gouroboros/ledger/byron"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// blockLayoutRepresentable reports whether the generic type decoder can read
// this block at all, independent of any validation verdict.
//
// This is the raw fallback's actual precondition, and decoding with
// validation disabled isolates it. If that succeeds, the decoder understood
// the block and the original failure was a rejection -- a body-hash mismatch,
// a nil header, or any other check behind the validation gate. Delivering
// those bytes anyway would let a peer keep a valid header, replace the body,
// and have the tampered block reach the raw callback while header-only range
// correlation still passed, so the fallback must not engage. If it still
// fails, nothing could be read, which is the case the raw callback exists
// for: the consumer that asked for raw blocks is the one equipped to decode
// and validate that layout.
//
// Asking the decoder beats matching on error types, which every era would
// have to keep in sync: the body-hash checks return a typed
// common.ValidationError but the nil-header checks beside them return a plain
// error, and a new era adding a third shape would silently widen the
// fallback.
//
// Only reached once a decode has already failed and a raw callback is
// configured, so the second decode is off the healthy path.
func blockLayoutRepresentable(blockType uint, raw []byte) bool {
	_, err := ledger.NewBlockFromCbor(
		blockType,
		raw,
		lcommon.VerifyConfig{
			SkipBodyHashValidation: true,
			SkipHeaderValidation:   true,
		},
	)
	return err == nil
}

// blockHeaderBodyMinFields is the number of leading block-header-body fields
// this file reads: block_number, slot, prev_hash. Every Shelley-family era
// starts its header body with those three, whatever era-specific fields
// follow.
const blockHeaderBodyMinFields = 3

// rawBlockHeaderInfo holds the header facts BlockFetch needs to correlate a
// received block with the requested range.
type rawBlockHeaderInfo struct {
	point    pcommon.Point
	prevHash []byte
}

func rawArrayDecoder(
	data []byte,
	name string,
) (*cbor.StreamDecoder, int, error) {
	decoder, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return nil, 0, fmt.Errorf("decode %s: %w", name, err)
	}
	fieldCount, _, _, err := decoder.DecodeArrayHeader()
	if err != nil {
		return nil, 0, fmt.Errorf("decode %s: %w", name, err)
	}
	return decoder, fieldCount, nil
}

func nextRawArrayItem(
	decoder *cbor.StreamDecoder,
	name string,
) ([]byte, error) {
	offset, length, err := decoder.Skip()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	return decoder.RawBytes(offset, length), nil
}

// rawBlockHeaderInfoFromCbor reads a block's point and previous-block hash
// straight from its CBOR, without a typed era decode.
//
// Every supported block is a [header, ...] array. Shelley-family headers are
// [header_body, signature], with block_number, slot, prev_hash leading the
// body. Byron headers have five fields and carry epoch/slot in consensus data.
// Hashes use the original header CBOR bytes, matching the typed headers.
//
// This exists so range correlation survives a payload whose full wire layout
// the generic type decoder cannot represent. It reads only the three fields
// correlation needs and makes no claim about the rest of the block; decoding
// that is the raw callback or block pipeline's job.
func rawBlockHeaderInfoFromCbor(
	data []byte,
	byronSlotsPerEpoch uint64,
) (rawBlockHeaderInfo, error) {
	blockDecoder, blockFields, err := rawArrayDecoder(data, "raw block")
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	if blockFields == 0 {
		return rawBlockHeaderInfo{}, errors.New("raw block has no header")
	}
	headerCbor, err := nextRawArrayItem(blockDecoder, "raw block header")
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	for i := 1; i < blockFields; i++ {
		if _, err := nextRawArrayItem(blockDecoder, "raw block field"); err != nil {
			return rawBlockHeaderInfo{}, err
		}
	}
	if !blockDecoder.EOF() {
		return rawBlockHeaderInfo{}, errors.New("raw block has trailing data")
	}
	headerDecoder, headerFields, err := rawArrayDecoder(
		headerCbor,
		"raw block header",
	)
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	if headerFields == 0 {
		return rawBlockHeaderInfo{}, errors.New(
			"raw block header has no body",
		)
	}
	if headerFields == 5 {
		return rawByronBlockHeaderInfo(
			headerCbor,
			headerDecoder,
			byronSlotsPerEpoch,
		)
	}
	if headerFields != 2 {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"raw block header has %d fields, expected 2 or 5",
			headerFields,
		)
	}
	bodyCbor, err := nextRawArrayItem(headerDecoder, "raw block header body")
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	bodyDecoder, bodyFields, err := rawArrayDecoder(
		bodyCbor,
		"raw block header body",
	)
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	if bodyFields < blockHeaderBodyMinFields {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"raw block header body has %d fields, expected at least %d",
			bodyFields,
			blockHeaderBodyMinFields,
		)
	}
	if _, err := nextRawArrayItem(bodyDecoder, "raw block number"); err != nil {
		return rawBlockHeaderInfo{}, err
	}
	var slot uint64
	if _, _, err := bodyDecoder.Decode(&slot); err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"decode raw block header slot: %w",
			err,
		)
	}
	prevHashCbor, err := nextRawArrayItem(bodyDecoder, "raw block previous hash")
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	prevHash, err := decodeRawPrevHash(prevHashCbor)
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	blockHash := lcommon.Blake2b256Hash(headerCbor)
	return rawBlockHeaderInfo{
		point:    pcommon.NewPoint(slot, blockHash.Bytes()),
		prevHash: prevHash,
	}, nil
}

func rawByronBlockHeaderInfo(
	headerCbor []byte,
	headerDecoder *cbor.StreamDecoder,
	slotsPerEpoch uint64,
) (rawBlockHeaderInfo, error) {
	if slotsPerEpoch == 0 {
		slotsPerEpoch = ledgerbyron.ByronSlotsPerEpoch
	}
	if _, err := nextRawArrayItem(headerDecoder, "raw Byron protocol magic"); err != nil {
		return rawBlockHeaderInfo{}, err
	}
	prevHashCbor, err := nextRawArrayItem(
		headerDecoder,
		"raw Byron previous hash",
	)
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	var prevHash lcommon.Blake2b256
	if _, err := cbor.Decode(prevHashCbor, &prevHash); err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"decode raw Byron previous hash: %w",
			err,
		)
	}
	if _, err := nextRawArrayItem(headerDecoder, "raw Byron body proof"); err != nil {
		return rawBlockHeaderInfo{}, err
	}
	consensusCbor, err := nextRawArrayItem(
		headerDecoder,
		"raw Byron consensus data",
	)
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	consensusDecoder, consensusFields, err := rawArrayDecoder(
		consensusCbor,
		"raw Byron consensus data",
	)
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	var blockType uint
	var epoch uint64
	var slotInEpoch uint64
	switch consensusFields {
	case 2:
		blockType = ledger.BlockTypeByronEbb
		if _, _, err := consensusDecoder.Decode(&epoch); err != nil {
			return rawBlockHeaderInfo{}, fmt.Errorf(
				"decode raw Byron EBB epoch: %w",
				err,
			)
		}
	case 4:
		blockType = ledger.BlockTypeByronMain
		slotIdCbor, err := nextRawArrayItem(
			consensusDecoder,
			"raw Byron slot ID",
		)
		if err != nil {
			return rawBlockHeaderInfo{}, err
		}
		slotIdDecoder, slotIdFields, err := rawArrayDecoder(
			slotIdCbor,
			"raw Byron slot ID",
		)
		if err != nil {
			return rawBlockHeaderInfo{}, err
		}
		if slotIdFields != 2 {
			return rawBlockHeaderInfo{}, fmt.Errorf(
				"raw Byron slot ID has %d fields, expected 2",
				slotIdFields,
			)
		}
		if _, _, err := slotIdDecoder.Decode(&epoch); err != nil {
			return rawBlockHeaderInfo{}, fmt.Errorf(
				"decode raw Byron epoch: %w",
				err,
			)
		}
		if _, _, err := slotIdDecoder.Decode(&slotInEpoch); err != nil {
			return rawBlockHeaderInfo{}, fmt.Errorf(
				"decode raw Byron slot: %w",
				err,
			)
		}
	default:
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"raw Byron consensus data has %d fields, expected 2 or 4",
			consensusFields,
		)
	}
	slot, err := ledgerbyron.SlotNumberFromEpochAndSlot(
		epoch,
		slotInEpoch,
		slotsPerEpoch,
	)
	if err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf("convert raw Byron slot: %w", err)
	}
	hashInput := make([]byte, 0, len(headerCbor)+2)
	hashInput = append(hashInput, 0x82, byte(blockType))
	hashInput = append(hashInput, headerCbor...)
	blockHash := lcommon.Blake2b256Hash(hashInput)
	return rawBlockHeaderInfo{
		point:    pcommon.NewPoint(slot, blockHash.Bytes()),
		prevHash: prevHash.Bytes(),
	}, nil
}

// decodeRawPrevHash decodes a header body's prev_hash field. Origin headers
// encode it as null, which the typed header decoders turn into the zero hash
// (see babbage.BabbageBlockHeaderBody.UnmarshalCBOR); mirror that here so the
// two paths agree.
func decodeRawPrevHash(data []byte) ([]byte, error) {
	if len(data) == 1 && data[0] == 0xf6 {
		var zero lcommon.Blake2b256
		return zero.Bytes(), nil
	}
	var prevHash lcommon.Blake2b256
	if _, err := cbor.Decode(data, &prevHash); err != nil {
		return nil, fmt.Errorf("decode raw block header prev hash: %w", err)
	}
	return prevHash.Bytes(), nil
}
