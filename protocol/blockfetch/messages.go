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
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

const (
	MessageTypeRequestRange = 0
	MessageTypeClientDone   = 1
	MessageTypeStartBatch   = 2
	MessageTypeNoBlocks     = 3
	MessageTypeBlock        = 4
	MessageTypeBatchDone    = 5
)

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

type MsgRequestRange struct {
	protocol.MessageBase
	Start common.Point
	End   common.Point
}

func NewMsgRequestRange(start common.Point, end common.Point) *MsgRequestRange {
	m := &MsgRequestRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestRange,
		},
		Start: start,
		End:   end,
	}
	return m
}

type MsgClientDone struct {
	protocol.MessageBase
}

func NewMsgClientDone() *MsgClientDone {
	m := &MsgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeClientDone,
		},
	}
	return m
}

type MsgStartBatch struct {
	protocol.MessageBase
}

func NewMsgStartBatch() *MsgStartBatch {
	m := &MsgStartBatch{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeStartBatch,
		},
	}
	return m
}

type MsgNoBlocks struct {
	protocol.MessageBase
}

func NewMsgNoBlocks() *MsgNoBlocks {
	m := &MsgNoBlocks{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNoBlocks,
		},
	}
	return m
}

type MsgBlock struct {
	protocol.MessageBase
	WrappedBlock []byte
}

func NewMsgBlock(wrappedBlock []byte) *MsgBlock {
	m := &MsgBlock{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlock,
		},
		WrappedBlock: wrappedBlock,
	}
	return m
}

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

type MsgBatchDone struct {
	protocol.MessageBase
}

func NewMsgBatchDone() *MsgBatchDone {
	m := &MsgBatchDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBatchDone,
		},
	}
	return m
}

type WrappedBlock struct {
	cbor.StructAsArray
	Type     uint
	RawBlock cbor.RawMessage
}
