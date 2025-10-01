// Copyright 2025 Blink Labs Software
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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// NOTE: these are dummy message IDs and will probably need to be changed
const (
	MessageTypeBlockRequest           = 0
	MessageTypeBlock                  = 1
	MessageTypeBlockTxsRequest        = 2
	MessageTypeBlockTxs               = 3
	MessageTypeVotesRequest           = 4
	MessageTypeVotes                  = 5
	MessageTypeBlockRangeRequest      = 6
	MessageTypeLastBlockAndTxsInRange = 7
	MessageTypeNextBlockAndTxsInRange = 8
	MessageTypeDone                   = 9
)

func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeBlockRequest:
		ret = &MsgBlockRequest{}
	case MessageTypeBlock:
		ret = &MsgBlock{}
	case MessageTypeBlockTxsRequest:
		ret = &MsgBlockTxsRequest{}
	case MessageTypeBlockTxs:
		ret = &MsgBlockTxs{}
	case MessageTypeVotesRequest:
		ret = &MsgVotesRequest{}
	case MessageTypeVotes:
		ret = &MsgVotes{}
	case MessageTypeBlockRangeRequest:
		ret = &MsgBlockRangeRequest{}
	case MessageTypeLastBlockAndTxsInRange:
		ret = &MsgLastBlockAndTxsInRange{}
	case MessageTypeNextBlockAndTxsInRange:
		ret = &MsgNextBlockAndTxsInRange{}
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

type MsgBlockRequest struct {
	protocol.MessageBase
	// TODO
}

func NewMsgBlockRequest() *MsgBlockRequest {
	m := &MsgBlockRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockRequest,
		},
		// TODO
	}
	return m
}

type MsgBlock struct {
	protocol.MessageBase
	// TODO
}

func NewMsgBlock() *MsgBlock {
	m := &MsgBlock{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlock,
		},
		// TODO
	}
	return m
}

type MsgBlockTxsRequest struct {
	protocol.MessageBase
	// TODO
}

func NewMsgBlockTxsRequest() *MsgBlockTxsRequest {
	m := &MsgBlockTxsRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxsRequest,
		},
		// TODO
	}
	return m
}

type MsgBlockTxs struct {
	protocol.MessageBase
	// TODO
}

func NewMsgBlockTxs() *MsgBlockTxs {
	m := &MsgBlockTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockTxs,
		},
		// TODO
	}
	return m
}

type MsgVotesRequest struct {
	protocol.MessageBase
	// TODO
}

func NewMsgVotesRequest() *MsgVotesRequest {
	m := &MsgVotesRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotesRequest,
		},
		// TODO
	}
	return m
}

type MsgVotes struct {
	protocol.MessageBase
	// TODO
}

func NewMsgVotes() *MsgVotes {
	m := &MsgVotes{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeVotes,
		},
		// TODO
	}
	return m
}

type MsgBlockRangeRequest struct {
	protocol.MessageBase
	// TODO
}

func NewMsgBlockRangeRequest() *MsgBlockRangeRequest {
	m := &MsgBlockRangeRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeBlockRangeRequest,
		},
		// TODO
	}
	return m
}

type MsgNextBlockAndTxsInRange struct {
	protocol.MessageBase
	// TODO
}

func NewMsgNextBlockAndTxsInRange() *MsgNextBlockAndTxsInRange {
	m := &MsgNextBlockAndTxsInRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNextBlockAndTxsInRange,
		},
		// TODO
	}
	return m
}

type MsgLastBlockAndTxsInRange struct {
	protocol.MessageBase
	// TODO
}

func NewMsgLastBlockAndTxsInRange() *MsgLastBlockAndTxsInRange {
	m := &MsgLastBlockAndTxsInRange{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeLastBlockAndTxsInRange,
		},
		// TODO
	}
	return m
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
