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

package leiosnotify

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// NOTE: these are dummy message IDs and will probably need to be changed
const (
	MessageTypeNotificationRequestNext = 0
	MessageTypeBlockAnnouncement       = 1
	MessageTypeBlockOffer              = 2
	MessageTypeBlockTxsOffer           = 3
	MessageTypeVotesOffer              = 4
	MessageTypeDone                    = 5
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeNotificationRequestNext:
		ret = &MsgNotificationRequestNext{}
	case MessageTypeBlockAnnouncement:
		ret = &MsgBlockAnnouncement{}
	case MessageTypeBlockOffer:
		ret = &MsgBlockOffer{}
	case MessageTypeBlockTxsOffer:
		ret = &MsgBlockTxsOffer{}
	case MessageTypeVotesOffer:
		ret = &MsgVotesOffer{}
	case MessageTypeDone:
		ret = &MsgDone{}
	default:
		return nil, fmt.Errorf("%s: unknown message type %d", ProtocolName, msgType)
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	// Store the raw message CBOR
	ret.SetCbor(data)
	return ret, nil
}

type MsgNotificationRequestNext struct {
	protocol.MessageBase
}

func NewMsgNotificationRequestNext() *MsgNotificationRequestNext {
	m := &MsgNotificationRequestNext{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNotificationRequestNext,
		},
	}
	return m
}

type MsgBlockAnnouncement struct {
	protocol.MessageBase
	BlockHeaderRaw cbor.RawMessage
}

func NewMsgBlockAnnouncement(
	blockHeader cbor.RawMessage,
) *MsgBlockAnnouncement {
	m := &MsgBlockAnnouncement{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockAnnouncement,
		},
		BlockHeaderRaw: blockHeader,
	}
	return m
}

type MsgBlockOffer struct {
	protocol.MessageBase
	Point pcommon.Point
	Size  uint64
}

func NewMsgBlockOffer(point pcommon.Point, size uint64) *MsgBlockOffer {
	m := &MsgBlockOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockOffer,
		},
		Point: point,
		Size:  size,
	}
	return m
}

type MsgBlockTxsOffer struct {
	protocol.MessageBase
	Point pcommon.Point
}

func NewMsgBlockTxsOffer(point pcommon.Point) *MsgBlockTxsOffer {
	m := &MsgBlockTxsOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxsOffer,
		},
		Point: point,
	}
	return m
}

// MsgVotesOffer carries vote information over leios-notify (tag 4). It is
// lenient about the per-vote element shape so dingo can interoperate with the
// IOG Leios prototype, which diffuses vote data inline over this message
// rather than over a standalone leios-votes mini-protocol:
//
//   - Votes holds vote IDs ([slot, voter_id], 2 elements) when the peer offers
//     IDs to be fetched (dingo's offer-and-fetch design).
//   - FullVotes holds complete votes ([slot, eb_hash, voter_id, signature], 4
//     elements) when the peer pushes votes directly (the prototype dialect).
//
// After decoding, exactly one of Votes / FullVotes is populated depending on
// the known per-vote CBOR array length. Encoding prefers FullVotes when
// present.
type MsgVotesOffer struct {
	protocol.MessageBase
	Votes     []MsgVotesOfferVote
	FullVotes []lcommon.LeiosVote
}

type MsgVotesOfferVote = lcommon.LeiosVoteId

func NewMsgVotesOffer(votes []MsgVotesOfferVote) *MsgVotesOffer {
	m := &MsgVotesOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotesOffer,
		},
		Votes: votes,
	}
	return m
}

// NewMsgVotesOfferFull builds a votes offer carrying full pushed votes, as the
// Leios prototype diffuses them inline over leios-notify.
func NewMsgVotesOfferFull(votes []lcommon.LeiosVote) *MsgVotesOffer {
	m := &MsgVotesOffer{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotesOffer,
		},
		FullVotes: votes,
	}
	return m
}

func (m *MsgVotesOffer) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if len(m.FullVotes) > 0 {
		return cbor.Encode([]any{m.MessageType, m.FullVotes})
	}
	return cbor.Encode([]any{m.MessageType, m.Votes})
}

func (m *MsgVotesOffer) UnmarshalCBOR(data []byte) error {
	// Decode the outer [msgType, [vote, ...]] envelope, capturing each vote
	// as raw CBOR so we can branch on its element count without committing to
	// a single per-vote shape.
	var envelope struct {
		cbor.StructAsArray
		MessageType uint8
		Votes       []cbor.RawMessage
	}
	if _, err := cbor.Decode(data, &envelope); err != nil {
		return err
	}
	m.MessageType = envelope.MessageType
	m.Votes = nil
	m.FullVotes = nil
	for idx, voteRaw := range envelope.Votes {
		// Peek at the element count to distinguish a vote ID
		// ([slot, voter_id]) from a full vote
		// ([slot, eb_hash, voter_id, signature]).
		var elems []cbor.RawMessage
		if _, err := cbor.Decode(voteRaw, &elems); err != nil {
			return fmt.Errorf(
				"%s: votes offer: decode vote %d: %w",
				ProtocolName,
				idx,
				err,
			)
		}
		switch len(elems) {
		case 2:
			var id MsgVotesOfferVote
			if _, err := cbor.Decode(voteRaw, &id); err != nil {
				return fmt.Errorf(
					"%s: votes offer: decode vote id %d: %w",
					ProtocolName,
					idx,
					err,
				)
			}
			m.Votes = append(m.Votes, id)
		case 4:
			var vote lcommon.LeiosVote
			if _, err := cbor.Decode(voteRaw, &vote); err != nil {
				return fmt.Errorf(
					"%s: votes offer: decode full vote %d: %w",
					ProtocolName,
					idx,
					err,
				)
			}
			m.FullVotes = append(m.FullVotes, vote)
		case 3:
			// prototype-2026w27 (the deployed musashi relay) diffuses a vote as
			// a 3-element array [endorser_block_hash (hash32), voter_id (uint),
			// signature (bytes .size 48)] — the 4-element full vote with the
			// leading slot field dropped. Verified against live captures from
			// leios-node.play.dev.cardano.org:3001 (network magic 164). Decode
			// the three fields; SlotNo is left zero because the wire omits it.
			var ebHash []byte
			if _, err := cbor.Decode(elems[0], &ebHash); err != nil {
				return fmt.Errorf(
					"%s: votes offer: decode vote %d endorser block hash: %w",
					ProtocolName, idx, err,
				)
			}
			if len(ebHash) != lcommon.Blake2b256Size {
				return fmt.Errorf(
					"%s: votes offer: vote %d endorser block hash is %d bytes, expected %d",
					ProtocolName, idx, len(ebHash), lcommon.Blake2b256Size,
				)
			}
			var voterId uint64
			if _, err := cbor.Decode(elems[1], &voterId); err != nil {
				return fmt.Errorf(
					"%s: votes offer: decode vote %d voter id: %w",
					ProtocolName, idx, err,
				)
			}
			var sig []byte
			if _, err := cbor.Decode(elems[2], &sig); err != nil {
				return fmt.Errorf(
					"%s: votes offer: decode vote %d signature: %w",
					ProtocolName, idx, err,
				)
			}
			if len(sig) != lcommon.LeiosBlsSignatureSize {
				return fmt.Errorf(
					"%s: votes offer: vote %d signature is %d bytes, expected %d",
					ProtocolName, idx, len(sig), lcommon.LeiosBlsSignatureSize,
				)
			}
			m.FullVotes = append(m.FullVotes, lcommon.LeiosVote{
				EndorserBlockHash: lcommon.NewBlake2b256(ebHash),
				VoterId:           voterId,
				VoteSignature:     sig,
			})
		default:
			return fmt.Errorf(
				"%s: votes offer: vote %d unexpected element count %d",
				ProtocolName,
				idx,
				len(elems),
			)
		}
	}
	m.SetCbor(data)
	return nil
}

type MsgDone struct {
	protocol.MessageBase
}

func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}
	return m
}
