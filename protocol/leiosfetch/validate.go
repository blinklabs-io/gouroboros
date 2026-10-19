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
//   - BlockTxs: only the message type; the transaction count is checked when
//     the reply is decoded.
//   - Votes: requested (slot, voter) pairs, each no more often than requested,
//     and no more votes than requested.
//   - Range: every block must be a well-formed endorser block, each of its
//     transactions must match the reference at the same position (the hash of
//     its CBOR and its encoded size), and no block may repeat. The requested
//     endpoints are ranking-block points; a reply carries only the endorser
//     block they certify, so the endpoints cannot be compared to it.
//
// For a range reply it returns the endorser block hash, which the caller
// records to detect a repeat across the stream. The request is the one the slot
// was acquired for and is never nil while a caller waits.
func validateResponse(
	request, resp protocol.Message,
) (*lcommon.Blake2b256, error) {
	switch req := request.(type) {
	case *MsgBlockRequest:
		r, ok := resp.(*MsgBlock)
		if !ok {
			return nil, unexpectedResponse(request, resp, "wrong message type")
		}
		return nil, validateBlock(req, r)
	case *MsgBlockTxsRequest:
		if _, ok := resp.(*MsgBlockTxs); !ok {
			return nil, unexpectedResponse(request, resp, "wrong message type")
		}
		// The transaction count is checked against the request's bitmaps when
		// the reply is decoded (Client.messageFromCbor), so it is not repeated.
		return nil, nil
	case *MsgVotesRequest:
		r, ok := resp.(*MsgVotes)
		if !ok {
			return nil, unexpectedResponse(request, resp, "wrong message type")
		}
		return nil, validateVotes(req, r)
	case *MsgBlockRangeRequest:
		switch r := resp.(type) {
		case *MsgNextBlockAndTxsInRange:
			return validateRangeBlock(req, resp, r.BlockRaw, r.TxsRaw)
		case *MsgLastBlockAndTxsInRange:
			return validateRangeBlock(req, resp, r.BlockRaw, r.TxsRaw)
		}
		return nil, unexpectedResponse(request, resp, "wrong message type")
	default:
		// Fail closed: a slot that is waiting always holds one of the requests
		// above, so this only guards a future request type added without a rule.
		return nil, fmt.Errorf(
			"%w: no validation rule for message type %d",
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

type voteKey struct {
	slotNo  uint64
	voterId uint64
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
	// A repeated ID in the request is answerable that many times, so count the
	// occurrences instead of treating the IDs as a set.
	remaining := make(map[voteKey]int, len(req.VoteIds))
	for _, id := range req.VoteIds {
		remaining[voteKey{id.SlotNo, id.VoterId}]++
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
		key := voteKey{vote.SlotNo, vote.VoterId}
		if remaining[key] == 0 {
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
		remaining[key]--
	}
	return nil
}

func validateRangeBlock(
	req *MsgBlockRangeRequest,
	resp protocol.Message,
	blockRaw []byte,
	txsRaw []cbor.RawMessage,
) (*lcommon.Blake2b256, error) {
	eb, err := decodeEndorserBlock(blockRaw)
	if err != nil {
		return nil, unexpectedResponse(
			req,
			resp,
			fmt.Sprintf("decode endorser block: %s", err),
		)
	}
	if len(txsRaw) != len(eb.TransactionReferences) {
		return nil, unexpectedResponse(
			req,
			resp,
			fmt.Sprintf(
				"%d transactions for %d references",
				len(txsRaw),
				len(eb.TransactionReferences),
			),
		)
	}
	// An endorser block lists its transactions by the hash of their CBOR and
	// their encoded size, in the order they are sent.
	for idx, ref := range eb.TransactionReferences {
		tx := txsRaw[idx]
		if uint64(len(tx)) != uint64(ref.TransactionSize) {
			return nil, unexpectedResponse(
				req,
				resp,
				fmt.Sprintf(
					"transaction %d is %d bytes, reference says %d",
					idx,
					len(tx),
					ref.TransactionSize,
				),
			)
		}
		if lcommon.Blake2b256Hash(tx) != ref.TransactionHash {
			return nil, unexpectedResponse(
				req,
				resp,
				fmt.Sprintf(
					"transaction %d does not match its reference hash",
					idx,
				),
			)
		}
	}
	hash := eb.Hash()
	return &hash, nil
}
