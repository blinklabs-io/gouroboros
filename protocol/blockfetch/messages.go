// Copyright 2023 Blink Labs Software
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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

// MessageTypeRequestRange is the message type for requesting a range of blocks.
const MessageTypeRequestRange = 0

// MessageTypeClientDone is the message type for client completion.
const MessageTypeClientDone = 1

// MessageTypeStartBatch is the message type for starting a batch.
const MessageTypeStartBatch = 2

// MessageTypeNoBlocks is the message type for indicating no blocks are available.
const MessageTypeNoBlocks = 3

// MessageTypeBlock is the message type for sending a block.
const MessageTypeBlock = 4

// MessageTypeBatchDone is the message type for indicating the end of a batch.
const MessageTypeBatchDone = 5

// NewMsgFromCbor decodes a protocol message from CBOR data based on the message type.
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeRequestRange:
		ret = &MsgRequestRange{}
	case MessageTypeClientDone:
		ret = &MsgClientDone{}
	case MessageTypeStartBatch:
		ret = &MsgStartBatch{}
	case MessageTypeNoBlocks:
		ret = &MsgNoBlocks{}
	case MessageTypeBlock:
		ret = &MsgBlock{}
	case MessageTypeBatchDone:
		ret = &MsgBatchDone{}
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

// MsgRequestRange represents a request for a range of blocks.
type MsgRequestRange struct {
	protocol.MessageBase
	Start pcommon.Point // Start point of the range
	End   pcommon.Point // End point of the range
}

// NewMsgRequestRange creates a new MsgRequestRange with the given start and end points.
func NewMsgRequestRange(
	start pcommon.Point,
	end pcommon.Point,
) *MsgRequestRange {
	m := &MsgRequestRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestRange,
		},
		Start: start,
		End:   end,
	}
	return m
}

// MsgClientDone indicates the client is done with block fetching.
type MsgClientDone struct {
	protocol.MessageBase
}

// NewMsgClientDone creates a new MsgClientDone message.
func NewMsgClientDone() *MsgClientDone {
	m := &MsgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeClientDone,
		},
	}
	return m
}

// MsgStartBatch indicates the start of a batch of blocks.
type MsgStartBatch struct {
	protocol.MessageBase
}

// NewMsgStartBatch creates a new MsgStartBatch message.
func NewMsgStartBatch() *MsgStartBatch {
	m := &MsgStartBatch{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeStartBatch,
		},
	}
	return m
}

// MsgNoBlocks indicates that no blocks are available for the requested range.
type MsgNoBlocks struct {
	protocol.MessageBase
}

// NewMsgNoBlocks creates a new MsgNoBlocks message.
func NewMsgNoBlocks() *MsgNoBlocks {
	m := &MsgNoBlocks{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNoBlocks,
		},
	}
	return m
}

// MsgBlock contains a block sent from the server to the client.
type MsgBlock struct {
	protocol.MessageBase
	WrappedBlock []byte // CBOR-encoded wrapped block
}

// NewMsgBlock creates a new MsgBlock with the given wrapped block data.
func NewMsgBlock(wrappedBlock []byte) *MsgBlock {
	m := &MsgBlock{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlock,
		},
		WrappedBlock: wrappedBlock,
	}
	return m
}

// MarshalCBOR encodes the MsgBlock as CBOR.
func (m MsgBlock) MarshalCBOR() ([]byte, error) {
	tmp := []any{
		m.MessageType,
		cbor.Tag{
			Number:  cbor.CborTagCbor,
			Content: m.WrappedBlock,
		},
	}
	return cbor.Encode(&tmp)
}

// MsgBatchDone indicates the end of a batch of blocks.
type MsgBatchDone struct {
	protocol.MessageBase
}

// NewMsgBatchDone creates a new MsgBatchDone message.
func NewMsgBatchDone() *MsgBatchDone {
	m := &MsgBatchDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBatchDone,
		},
	}
	return m
}

// WrappedBlock is a CBOR structure containing a block type and raw block data.
type WrappedBlock struct {
	cbor.StructAsArray
	Type     uint            // Block type identifier
	RawBlock cbor.RawMessage // Raw block data
}
