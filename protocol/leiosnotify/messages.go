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
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
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
// the per-vote CBOR array length. Encoding prefers FullVotes when present.
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
		default:
			return fmt.Errorf(
				"%s: votes offer: vote %d has unexpected element count %d",
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
