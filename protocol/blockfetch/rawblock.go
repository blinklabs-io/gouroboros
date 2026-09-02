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
		lcommon.VerifyConfig{SkipBodyHashValidation: true},
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

// rawBlockHeaderInfoFromCbor reads a block's point and previous-block hash
// straight from its CBOR, without a typed era decode.
//
// Every Shelley-family block (Shelley through Dijkstra) is a [header, ...]
// array whose header is [header_body, signature] and whose header body begins
// with block_number, slot, prev_hash. The block hash is Blake2b-256 over the
// header's original CBOR bytes, the same definition the typed headers use --
// see babbage.BabbageBlockHeader.Hash.
//
// This exists so range correlation survives a payload whose full wire layout
// the generic type decoder cannot represent. It reads only the three fields
// correlation needs and makes no claim about the rest of the block; decoding
// that is the raw callback's job. Byron blocks use a different header shape
// and are not supported here.
func rawBlockHeaderInfoFromCbor(data []byte) (rawBlockHeaderInfo, error) {
	var blockElems []cbor.RawMessage
	if _, err := cbor.Decode(data, &blockElems); err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"decode raw block: %w",
			err,
		)
	}
	if len(blockElems) == 0 {
		return rawBlockHeaderInfo{}, errors.New("raw block has no header")
	}
	headerCbor := []byte(blockElems[0])
	var headerElems []cbor.RawMessage
	if _, err := cbor.Decode(headerCbor, &headerElems); err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"decode raw block header: %w",
			err,
		)
	}
	if len(headerElems) == 0 {
		return rawBlockHeaderInfo{}, errors.New(
			"raw block header has no body",
		)
	}
	var bodyElems []cbor.RawMessage
	if _, err := cbor.Decode(headerElems[0], &bodyElems); err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"decode raw block header body: %w",
			err,
		)
	}
	if len(bodyElems) < blockHeaderBodyMinFields {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"raw block header body has %d fields, expected at least %d",
			len(bodyElems),
			blockHeaderBodyMinFields,
		)
	}
	var slot uint64
	if _, err := cbor.Decode(bodyElems[1], &slot); err != nil {
		return rawBlockHeaderInfo{}, fmt.Errorf(
			"decode raw block header slot: %w",
			err,
		)
	}
	prevHash, err := decodeRawPrevHash(bodyElems[2])
	if err != nil {
		return rawBlockHeaderInfo{}, err
	}
	blockHash := lcommon.Blake2b256Hash(headerCbor)
	return rawBlockHeaderInfo{
		point:    pcommon.NewPoint(slot, blockHash.Bytes()),
		prevHash: prevHash,
	}, nil
}

// decodeRawPrevHash decodes a header body's prev_hash field. Origin headers
// encode it as null, which the typed header decoders turn into the zero hash
// (see babbage.BabbageBlockHeaderBody.UnmarshalCBOR); mirror that here so the
// two paths agree.
func decodeRawPrevHash(data cbor.RawMessage) ([]byte, error) {
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
