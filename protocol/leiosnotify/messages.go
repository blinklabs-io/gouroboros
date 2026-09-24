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
	"errors"
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
	// MaxVotesOfferCount bounds vote work from one leios-notify message,
	// independently of the transport frame limit.
	MaxVotesOfferCount = 1000
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
//   - FullVotes holds legacy complete votes
//     ([slot, eb_hash, voter_id, signature], 4 elements).
//   - PrototypeVotes holds current prototype votes
//     ([announcing_rb_hash, voter_id, signature], 3 elements).
//
// After decoding, exactly one of Votes, FullVotes, or PrototypeVotes is
// populated depending on the known per-vote CBOR array length. Encoding
// prefers PrototypeVotes, then FullVotes, when present.
type MsgVotesOffer struct {
	protocol.MessageBase
	Votes          []MsgVotesOfferVote
	FullVotes      []lcommon.LeiosVote
	PrototypeVotes []lcommon.LeiosPrototypeVote
}

type PrototypeVote = lcommon.LeiosPrototypeVote

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

// NewMsgVotesOfferPrototype builds a current-prototype pushed vote offer.
func NewMsgVotesOfferPrototype(votes []PrototypeVote) *MsgVotesOffer {
	return &MsgVotesOffer{
		MessageBase:    protocol.MessageBase{MessageType: MessageTypeVotesOffer},
		PrototypeVotes: votes,
	}
}

func (m *MsgVotesOffer) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if len(m.PrototypeVotes) > 0 {
		if len(m.PrototypeVotes) > MaxVotesOfferCount {
			return nil, fmt.Errorf(
				"%s: votes offer count %d exceeds maximum %d",
				ProtocolName,
				len(m.PrototypeVotes),
				MaxVotesOfferCount,
			)
		}
		return cbor.Encode([]any{m.MessageType, m.PrototypeVotes})
	}
	if len(m.FullVotes) > 0 {
		if len(m.FullVotes) > MaxVotesOfferCount {
			return nil, fmt.Errorf(
				"%s: votes offer count %d exceeds maximum %d",
				ProtocolName,
				len(m.FullVotes),
				MaxVotesOfferCount,
			)
		}
		return cbor.Encode([]any{m.MessageType, m.FullVotes})
	}
	if len(m.Votes) > MaxVotesOfferCount {
		return nil, fmt.Errorf(
			"%s: votes offer count %d exceeds maximum %d",
			ProtocolName,
			len(m.Votes),
			MaxVotesOfferCount,
		)
	}
	return cbor.Encode([]any{m.MessageType, m.Votes})
}

func (m *MsgVotesOffer) UnmarshalCBOR(data []byte) error {
	m.MessageType = 0
	m.Votes = nil
	m.FullVotes = nil
	m.PrototypeVotes = nil
	dec, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return err
	}
	envelopeCount, envelopeIndefinite, err := decodeArrayHeader(dec)
	if err != nil {
		return fmt.Errorf("%s: votes offer: decode envelope: %w", ProtocolName, err)
	}
	if !envelopeIndefinite && envelopeCount != 2 {
		return fmt.Errorf(
			"%s: votes offer: envelope has %d elements, expected 2",
			ProtocolName,
			envelopeCount,
		)
	}
	if envelopeIndefinite && arrayEnded(dec) {
		return fmt.Errorf("%s: votes offer: missing message type", ProtocolName)
	}
	if _, _, err := dec.Decode(&m.MessageType); err != nil {
		return fmt.Errorf(
			"%s: votes offer: decode message type: %w",
			ProtocolName,
			err,
		)
	}
	if envelopeIndefinite && arrayEnded(dec) {
		return fmt.Errorf("%s: votes offer: missing vote list", ProtocolName)
	}
	voteRaws, err := decodeArrayItems(dec, MaxVotesOfferCount)
	if err != nil {
		return fmt.Errorf("%s: votes offer: decode vote list: %w", ProtocolName, err)
	}
	if envelopeIndefinite {
		if !arrayEnded(dec) {
			return fmt.Errorf(
				"%s: votes offer: envelope has extra elements",
				ProtocolName,
			)
		}
		if err := dec.Advance(1); err != nil {
			return err
		}
	}
	if !dec.EOF() {
		return fmt.Errorf("%s: votes offer: trailing CBOR data", ProtocolName)
	}
	for idx, voteRaw := range voteRaws {
		// Peek at the element count to distinguish a vote ID
		// ([slot, voter_id]) from a full vote
		// ([slot, eb_hash, voter_id, signature]).
		elems, err := decodeBoundedArray(voteRaw, 4)
		if err != nil {
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
			// The current prototype signs and diffuses the announcing ranking
			// block hash, not the endorser-block point.
			var rbHash []byte
			if _, err := cbor.Decode(elems[0], &rbHash); err != nil {
				return fmt.Errorf(
					"%s: votes offer: decode vote %d announcing RB hash: %w",
					ProtocolName, idx, err,
				)
			}
			if len(rbHash) != lcommon.Blake2b256Size {
				return fmt.Errorf(
					"%s: votes offer: vote %d announcing RB hash is %d bytes, expected %d",
					ProtocolName, idx, len(rbHash), lcommon.Blake2b256Size,
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
			m.PrototypeVotes = append(m.PrototypeVotes, PrototypeVote{
				AnnouncingRbHash: lcommon.NewBlake2b256(rbHash),
				VoterId:          voterId,
				VoteSignature:    sig,
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

func decodeArrayHeader(dec *cbor.StreamDecoder) (int, bool, error) {
	position := dec.Position()
	if position >= len(dec.Data()) {
		return 0, false, errors.New("unexpected end of CBOR data")
	}
	count, headerSize, indefinite := cbor.ArrayInfo(dec.Data()[position:])
	if count < 0 {
		return 0, false, errors.New("expected array")
	}
	if err := dec.Advance(int(headerSize)); err != nil {
		return 0, false, err
	}
	return count, indefinite, nil
}

func arrayEnded(dec *cbor.StreamDecoder) bool {
	position := dec.Position()
	return position < len(dec.Data()) && dec.Data()[position] == 0xff
}

func decodeArrayItems(
	dec *cbor.StreamDecoder,
	maxCount int,
) ([]cbor.RawMessage, error) {
	count, indefinite, err := decodeArrayHeader(dec)
	if err != nil {
		return nil, err
	}
	if !indefinite && count > maxCount {
		return nil, fmt.Errorf(
			"array has %d elements, maximum is %d",
			count,
			maxCount,
		)
	}
	items := make([]cbor.RawMessage, 0, min(count, maxCount))
	for idx := 0; indefinite || idx < count; idx++ {
		if indefinite && arrayEnded(dec) {
			if err := dec.Advance(1); err != nil {
				return nil, err
			}
			break
		}
		if idx >= maxCount {
			return nil, fmt.Errorf("array exceeds maximum of %d elements", maxCount)
		}
		start, length, err := dec.Skip()
		if err != nil {
			return nil, err
		}
		items = append(items, cbor.RawMessage(dec.RawBytes(start, length)))
	}
	return items, nil
}

func decodeBoundedArray(data []byte, maxCount int) ([]cbor.RawMessage, error) {
	dec, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return nil, err
	}
	items, err := decodeArrayItems(dec, maxCount)
	if err != nil {
		return nil, err
	}
	if !dec.EOF() {
		return nil, errors.New("trailing CBOR data")
	}
	return items, nil
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
