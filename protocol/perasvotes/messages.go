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

package perasvotes

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

const (
	// MessageTypeInit starts object diffusion.
	MessageTypeInit uint8 = iota
	// MessageTypeRequestObjectIDs asks for object identifiers and acknowledges
	// previously announced identifiers.
	MessageTypeRequestObjectIDs
	// MessageTypeReplyObjectIDs announces object identifiers.
	MessageTypeReplyObjectIDs
	// MessageTypeRequestObjects asks for the payloads of announced identifiers.
	MessageTypeRequestObjects
	// MessageTypeReplyObjects returns requested payloads.
	MessageTypeReplyObjects
	// MessageTypeDone terminates object diffusion.
	MessageTypeDone
)

// VoteID is the reference vote key: [round number, seat index].
type VoteID struct {
	RoundNo   uint64
	SeatIndex uint16
}

// MarshalCBOR encodes the identifier as [round, seat index].
func (id VoteID) MarshalCBOR() ([]byte, error) {
	return cbor.Encode([]any{
		id.RoundNo,
		id.SeatIndex,
	})
}

// UnmarshalCBOR decodes a definite two-field Peras vote identifier.
func (id *VoteID) UnmarshalCBOR(data []byte) error {
	fields, err := decodeArray(data, 2, false)
	if err != nil {
		return fmt.Errorf("decode Peras vote ID: %w", err)
	}
	var tmp VoteID
	if _, err := cbor.Decode(fields[0], &tmp.RoundNo); err != nil {
		return fmt.Errorf("decode Peras vote round: %w", err)
	}
	if _, err := cbor.Decode(fields[1], &tmp.SeatIndex); err != nil {
		return fmt.Errorf("decode Peras vote seat index: %w", err)
	}
	*id = tmp
	return nil
}

// VoteObject contains one encoded Peras V1 vote; the transport keeps its
// consensus-specific proof and signature fields opaque.
type VoteObject cbor.RawMessage

// MarshalCBOR preserves the vote's original CBOR encoding.
func (v VoteObject) MarshalCBOR() ([]byte, error) {
	if _, err := v.VoteID(); err != nil {
		return nil, err
	}
	return []byte(v), nil
}

// UnmarshalCBOR stores a valid five-field Peras V1 vote encoding.
func (v *VoteObject) UnmarshalCBOR(data []byte) error {
	if _, err := VoteObject(data).VoteID(); err != nil {
		return err
	}
	*v = append((*v)[:0], data...)
	return nil
}

// VoteID returns the round and seat index encoded in the vote.
func (v VoteObject) VoteID() (VoteID, error) {
	fields, err := decodeArray([]byte(v), 5, false)
	if err != nil {
		return VoteID{}, fmt.Errorf("decode Peras vote object: %w", err)
	}
	var id VoteID
	if _, err := cbor.Decode(fields[0], &id.RoundNo); err != nil {
		return VoteID{}, fmt.Errorf("decode Peras vote round: %w", err)
	}
	if _, err := cbor.Decode(fields[2], &id.SeatIndex); err != nil {
		return VoteID{}, fmt.Errorf("decode Peras vote seat index: %w", err)
	}
	return id, nil
}

// MsgInit starts an ObjectDiffusion session.
type MsgInit struct{ protocol.MessageBase }

// NewMsgInit creates an initialization message.
func NewMsgInit() *MsgInit {
	return &MsgInit{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeInit},
	}
}

// MsgRequestObjectIDs requests object identifiers and acknowledges prior IDs.
type MsgRequestObjectIDs struct {
	protocol.MessageBase
	Blocking     bool
	AckCount     uint16
	RequestCount uint16
}

// NewMsgRequestObjectIDs creates an object ID request.
func NewMsgRequestObjectIDs(
	blocking bool,
	ack, request uint16,
) *MsgRequestObjectIDs {
	return &MsgRequestObjectIDs{
		MessageBase:  protocol.MessageBase{MessageType: MessageTypeRequestObjectIDs},
		Blocking:     blocking,
		AckCount:     ack,
		RequestCount: request,
	}
}

// MsgReplyObjectIDs announces object identifiers to the client.
type MsgReplyObjectIDs struct {
	protocol.MessageBase
	ObjectIDs []VoteID
}

// NewMsgReplyObjectIDs creates an object ID reply.
func NewMsgReplyObjectIDs(ids []VoteID) *MsgReplyObjectIDs {
	return &MsgReplyObjectIDs{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeReplyObjectIDs},
		ObjectIDs:   append([]VoteID(nil), ids...),
	}
}

// MarshalCBOR encodes the ID list using the ObjectDiffusion wire format.
func (m MsgReplyObjectIDs) MarshalCBOR() ([]byte, error) {
	items := make(cbor.IndefLengthList, len(m.ObjectIDs))
	for idx := range m.ObjectIDs {
		items[idx] = m.ObjectIDs[idx]
	}
	return cbor.Encode([]any{MessageTypeReplyObjectIDs, items})
}

// MsgRequestObjects requests payloads for announced identifiers.
type MsgRequestObjects struct {
	protocol.MessageBase
	ObjectIDs []VoteID
}

// NewMsgRequestObjects creates a payload request.
func NewMsgRequestObjects(ids []VoteID) *MsgRequestObjects {
	return &MsgRequestObjects{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeRequestObjects},
		ObjectIDs:   append([]VoteID(nil), ids...),
	}
}

// MarshalCBOR encodes the ID list using the ObjectDiffusion wire format.
func (m MsgRequestObjects) MarshalCBOR() ([]byte, error) {
	items := make(cbor.IndefLengthList, len(m.ObjectIDs))
	for idx := range m.ObjectIDs {
		items[idx] = m.ObjectIDs[idx]
	}
	return cbor.Encode([]any{MessageTypeRequestObjects, items})
}

// MsgReplyObjects returns requested vote payloads.
type MsgReplyObjects struct {
	protocol.MessageBase
	Objects []VoteObject
}

// NewMsgReplyObjects creates a payload reply.
func NewMsgReplyObjects(objects []VoteObject) *MsgReplyObjects {
	return &MsgReplyObjects{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeReplyObjects},
		Objects:     append([]VoteObject(nil), objects...),
	}
}

// MarshalCBOR encodes the payload list using the ObjectDiffusion wire format.
func (m MsgReplyObjects) MarshalCBOR() ([]byte, error) {
	items := make(cbor.IndefLengthList, len(m.Objects))
	for idx := range m.Objects {
		items[idx] = m.Objects[idx]
	}
	return cbor.Encode([]any{MessageTypeReplyObjects, items})
}

// MsgDone terminates an ObjectDiffusion session.
type MsgDone struct{ protocol.MessageBase }

// NewMsgDone creates a termination message.
func NewMsgDone() *MsgDone {
	return &MsgDone{
		MessageBase: protocol.MessageBase{MessageType: MessageTypeDone},
	}
}

// NewMsgFromCbor decodes a typed ObjectDiffusion protocol message.
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	fields, err := decodeArray(data, -1, false)
	if err != nil {
		return nil, fmt.Errorf("%s: decode message: %w", ProtocolName, err)
	}
	if len(fields) == 0 {
		return nil, errors.New("peras-vote-diffusion: empty message")
	}
	var wireType uint8
	if _, err := cbor.Decode(fields[0], &wireType); err != nil {
		return nil, fmt.Errorf("%s: decode message type: %w", ProtocolName, err)
	}
	if uint(wireType) != msgType {
		return nil, fmt.Errorf(
			"%s: message type mismatch: parser received %d, payload contains %d",
			ProtocolName,
			msgType,
			wireType,
		)
	}
	var ret protocol.Message
	switch wireType {
	case MessageTypeInit:
		if len(fields) != 1 {
			return nil, errors.New(
				"peras-vote-diffusion: init message must have 1 field",
			)
		}
		ret = NewMsgInit()
	case MessageTypeRequestObjectIDs:
		if len(fields) != 4 {
			return nil, errors.New(
				"peras-vote-diffusion: request object IDs message must have 4 fields",
			)
		}
		var blocking bool
		var ack, request uint16
		if _, err := cbor.Decode(fields[1], &blocking); err != nil {
			return nil, fmt.Errorf("%s: decode blocking flag: %w", ProtocolName, err)
		}
		if _, err := cbor.Decode(fields[2], &ack); err != nil {
			return nil, fmt.Errorf(
				"%s: decode acknowledgement count: %w",
				ProtocolName,
				err,
			)
		}
		if _, err := cbor.Decode(fields[3], &request); err != nil {
			return nil, fmt.Errorf("%s: decode requested count: %w", ProtocolName, err)
		}
		ret = NewMsgRequestObjectIDs(blocking, ack, request)
	case MessageTypeReplyObjectIDs:
		if len(fields) != 2 {
			return nil, errors.New(
				"peras-vote-diffusion: reply object IDs message must have 2 fields",
			)
		}
		ids, err := decodeVoteIDs(fields[1])
		if err != nil {
			return nil, fmt.Errorf("%s: decode object IDs: %w", ProtocolName, err)
		}
		ret = NewMsgReplyObjectIDs(ids)
	case MessageTypeRequestObjects:
		if len(fields) != 2 {
			return nil, errors.New(
				"peras-vote-diffusion: request objects message must have 2 fields",
			)
		}
		ids, err := decodeVoteIDs(fields[1])
		if err != nil {
			return nil, fmt.Errorf(
				"%s: decode requested object IDs: %w",
				ProtocolName,
				err,
			)
		}
		ret = NewMsgRequestObjects(ids)
	case MessageTypeReplyObjects:
		if len(fields) != 2 {
			return nil, errors.New(
				"peras-vote-diffusion: reply objects message must have 2 fields",
			)
		}
		objects, err := decodeVoteObjects(fields[1])
		if err != nil {
			return nil, fmt.Errorf("%s: decode objects: %w", ProtocolName, err)
		}
		ret = NewMsgReplyObjects(objects)
	case MessageTypeDone:
		if len(fields) != 1 {
			return nil, errors.New(
				"peras-vote-diffusion: done message must have 1 field",
			)
		}
		ret = NewMsgDone()
	default:
		return nil, fmt.Errorf("%s: unknown message type %d", ProtocolName, wireType)
	}
	ret.SetCbor(data)
	return ret, nil
}

func decodeVoteIDs(data []byte) ([]VoteID, error) {
	items, err := decodeArray(data, -1, true)
	if err != nil {
		return nil, err
	}
	ids := make([]VoteID, len(items))
	for idx, item := range items {
		if _, err := cbor.Decode(item, &ids[idx]); err != nil {
			return nil, fmt.Errorf("ID %d: %w", idx, err)
		}
	}
	return ids, nil
}

func decodeVoteObjects(data []byte) ([]VoteObject, error) {
	items, err := decodeArray(data, -1, true)
	if err != nil {
		return nil, err
	}
	objects := make([]VoteObject, len(items))
	for idx, item := range items {
		if err := objects[idx].UnmarshalCBOR(item); err != nil {
			return nil, fmt.Errorf("object %d: %w", idx, err)
		}
	}
	return objects, nil
}

func decodeArray(
	data []byte,
	expectedLen int,
	allowIndefinite bool,
) ([]cbor.RawMessage, error) {
	dec, err := cbor.NewStreamDecoder(data)
	if err != nil {
		return nil, err
	}
	count, headerSize, indefinite := cbor.ArrayInfo(data)
	if count < 0 || (indefinite && !allowIndefinite) {
		return nil, errors.New("expected definite-length array")
	}
	if err := dec.Advance(int(headerSize)); err != nil {
		return nil, err
	}
	maxCount := int(MaxObjectsUnacknowledged) + 2
	if expectedLen >= 0 {
		maxCount = expectedLen
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
		position := dec.Position()
		if indefinite && position < len(data) && data[position] == 0xff {
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
	if expectedLen >= 0 && len(items) != expectedLen {
		return nil, fmt.Errorf(
			"array has %d elements, expected %d",
			len(items),
			expectedLen,
		)
	}
	if !dec.EOF() {
		return nil, errors.New("trailing CBOR data")
	}
	return items, nil
}
