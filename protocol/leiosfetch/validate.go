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

package leiosfetch

import (
	"bytes"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// validateResponse checks that a reply describes the request it answers.
//
// The state machine only proves the reply has the message type its state
// allows, and the protocol has no request identifier. Replies echo nothing, so
// each is bound through what it contains:
//   - Block: the endorser block must hash to the requested point's hash.
//   - BlockTxs: one transaction per requested bitmap bit.
//   - Votes: distinct, requested (slot, voter) pairs, and no more than asked.
//   - Range: every block must be a well-formed endorser block carrying exactly
//     the transactions it references, and no block may repeat. The requested
//     endpoints are ranking-block points; a reply carries only the endorser
//     block they certify, so the endpoints cannot be compared to it.
//
// seen holds the endorser block hashes already delivered for a range request.
func validateResponse(
	request, resp protocol.Message,
	seen map[lcommon.Blake2b256]struct{},
) error {
	switch req := request.(type) {
	case *MsgBlockRequest:
		r, ok := resp.(*MsgBlock)
		if !ok {
			return unexpectedResponse(request, resp, "wrong message type")
		}
		return validateBlock(req, r)
	case *MsgBlockTxsRequest:
		r, ok := resp.(*MsgBlockTxs)
		if !ok {
			return unexpectedResponse(request, resp, "wrong message type")
		}
		return validateBlockTxs(req, r)
	case *MsgVotesRequest:
		r, ok := resp.(*MsgVotes)
		if !ok {
			return unexpectedResponse(request, resp, "wrong message type")
		}
		return validateVotes(req, r)
	case *MsgBlockRangeRequest:
		switch r := resp.(type) {
		case *MsgNextBlockAndTxsInRange:
			return validateRangeBlock(req, resp, r.BlockRaw, r.TxsRaw, seen)
		case *MsgLastBlockAndTxsInRange:
			return validateRangeBlock(req, resp, r.BlockRaw, r.TxsRaw, seen)
		}
		return unexpectedResponse(request, resp, "wrong message type")
	default:
		return fmt.Errorf(
			"%w: no active request for message type %d",
			ErrUnexpectedResponse,
			resp.Type(),
		)
	}
}

func unexpectedResponse(
	request protocol.Message,
	resp protocol.Message,
	reason string,
) error {
	return fmt.Errorf(
		"%w: message type %d for request type %d: %s",
		ErrUnexpectedResponse,
		resp.Type(),
		request.Type(),
		reason,
	)
}

func decodeEndorserBlock(raw []byte) (*lcommon.LeiosEndorserBlock, error) {
	eb, err := lcommon.NewLeiosEndorserBlockFromCbor(raw)
	if err != nil {
		return nil, err
	}
	if err := eb.Validate(); err != nil {
		return nil, err
	}
	return eb, nil
}

func validateBlock(req *MsgBlockRequest, resp *MsgBlock) error {
	eb, err := decodeEndorserBlock(resp.BlockRaw)
	if err != nil {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf("decode endorser block: %s", err),
		)
	}
	hash := eb.Hash()
	if !bytes.Equal(hash.Bytes(), req.Point.Hash) {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf(
				"endorser block hash %x differs from requested %x",
				hash.Bytes(),
				req.Point.Hash,
			),
		)
	}
	return nil
}

func validateBlockTxs(req *MsgBlockTxsRequest, resp *MsgBlockTxs) error {
	requested := bitmapTxCount(req.Bitmaps)
	if len(resp.TxsRaw) != requested {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf(
				"%d transactions for %d requested",
				len(resp.TxsRaw),
				requested,
			),
		)
	}
	return nil
}

func validateVotes(req *MsgVotesRequest, resp *MsgVotes) error {
	if len(resp.VotesRaw) > len(req.VoteIds) {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf(
				"%d votes for %d requested",
				len(resp.VotesRaw),
				len(req.VoteIds),
			),
		)
	}
	pending := make(map[[2]uint64]struct{}, len(req.VoteIds))
	for _, id := range req.VoteIds {
		pending[[2]uint64{id.SlotNo, id.VoterId}] = struct{}{}
	}
	for idx, raw := range resp.VotesRaw {
		var vote lcommon.LeiosVote
		if _, err := cbor.Decode(raw, &vote); err != nil {
			return unexpectedResponse(
				req,
				resp,
				fmt.Sprintf("decode vote %d: %s", idx, err),
			)
		}
		key := [2]uint64{vote.SlotNo, vote.VoterId}
		// Removing on first sight makes a repeated vote fail this lookup.
		if _, ok := pending[key]; !ok {
			return unexpectedResponse(
				req,
				resp,
				fmt.Sprintf(
					"vote %d (slot %d, voter %d) was not requested or repeats",
					idx,
					vote.SlotNo,
					vote.VoterId,
				),
			)
		}
		delete(pending, key)
	}
	return nil
}

func validateRangeBlock(
	req *MsgBlockRangeRequest,
	resp protocol.Message,
	blockRaw []byte,
	txsRaw []cbor.RawMessage,
	seen map[lcommon.Blake2b256]struct{},
) error {
	eb, err := decodeEndorserBlock(blockRaw)
	if err != nil {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf("decode endorser block: %s", err),
		)
	}
	if len(txsRaw) != len(eb.TransactionReferences) {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf(
				"%d transactions for %d references",
				len(txsRaw),
				len(eb.TransactionReferences),
			),
		)
	}
	hash := eb.Hash()
	if _, dup := seen[hash]; dup {
		return unexpectedResponse(
			req,
			resp,
			fmt.Sprintf("endorser block %x repeats in the range", hash.Bytes()),
		)
	}
	seen[hash] = struct{}{}
	return nil
}
