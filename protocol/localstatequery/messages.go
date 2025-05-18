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

package localstatequery

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

// Message types
const (
	MessageTypeAcquire               = 0
	MessageTypeAcquired              = 1
	MessageTypeFailure               = 2
	MessageTypeQuery                 = 3
	MessageTypeResult                = 4
	MessageTypeRelease               = 5
	MessageTypeReacquire             = 6
	MessageTypeDone                  = 7
	MessageTypeAcquireVolatileTip    = 8
	MessageTypeReacquireVolatileTip  = 9
	MessageTypeAcquireImmutableTip   = 10
	MessageTypeReacquireImmutableTip = 11
)

// Acquire failure reasons
const (
	AcquireFailurePointTooOld     = 0
	AcquireFailurePointNotOnChain = 1
)

// NewMsgFromCbor parses a LocalStateQuery message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeAcquire:
		ret = &MsgAcquire{}
	case MessageTypeAcquired:
		ret = &MsgAcquired{}
	case MessageTypeFailure:
		ret = &MsgFailure{}
	case MessageTypeQuery:
		ret = &MsgQuery{}
	case MessageTypeResult:
		ret = &MsgResult{}
	case MessageTypeRelease:
		ret = &MsgRelease{}
	case MessageTypeReacquire:
		ret = &MsgReAcquire{}
	case MessageTypeAcquireVolatileTip:
		ret = &MsgAcquireVolatileTip{}
	case MessageTypeReacquireVolatileTip:
		ret = &MsgReAcquireVolatileTip{}
	case MessageTypeAcquireImmutableTip:
		ret = &MsgAcquireImmutableTip{}
	case MessageTypeReacquireImmutableTip:
		ret = &MsgReAcquireImmutableTip{}
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

type MsgAcquire struct {
	protocol.MessageBase
	Point common.Point
}

func NewMsgAcquire(point common.Point) *MsgAcquire {
	m := &MsgAcquire{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcquire,
		},
		Point: point,
	}
	return m
}

type MsgAcquireVolatileTip struct {
	protocol.MessageBase
}

func NewMsgAcquireVolatileTip() *MsgAcquireVolatileTip {
	m := &MsgAcquireVolatileTip{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcquireVolatileTip,
		},
	}
	return m
}

type MsgAcquireImmutableTip struct {
	protocol.MessageBase
}

func NewMsgAcquireImmutableTip() *MsgAcquireImmutableTip {
	m := &MsgAcquireImmutableTip{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcquireImmutableTip,
		},
	}
	return m
}

type MsgAcquired struct {
	protocol.MessageBase
}

func NewMsgAcquired() *MsgAcquired {
	m := &MsgAcquired{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcquired,
		},
	}
	return m
}

type MsgFailure struct {
	protocol.MessageBase
	Failure uint8
}

func NewMsgFailure(failure uint8) *MsgFailure {
	m := &MsgFailure{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeFailure,
		},
		Failure: failure,
	}
	return m
}

type MsgQuery struct {
	protocol.MessageBase
	Query QueryWrapper
}

func NewMsgQuery(query any) *MsgQuery {
	m := &MsgQuery{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeQuery,
		},
		Query: QueryWrapper{
			Query: query,
		},
	}
	return m
}

type MsgResult struct {
	protocol.MessageBase
	Result cbor.RawMessage
}

func NewMsgResult(resultCbor []byte) *MsgResult {
	m := &MsgResult{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeResult,
		},
		Result: cbor.RawMessage(resultCbor),
	}
	return m
}

type MsgRelease struct {
	protocol.MessageBase
}

func NewMsgRelease() *MsgRelease {
	m := &MsgRelease{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRelease,
		},
	}
	return m
}

type MsgReAcquire struct {
	protocol.MessageBase
	Point common.Point
}

func NewMsgReAcquire(point common.Point) *MsgReAcquire {
	m := &MsgReAcquire{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReacquire,
		},
		Point: point,
	}
	return m
}

type MsgReAcquireVolatileTip struct {
	protocol.MessageBase
}

func NewMsgReAcquireVolatileTip() *MsgReAcquireVolatileTip {
	m := &MsgReAcquireVolatileTip{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReacquireVolatileTip,
		},
	}
	return m
}

type MsgReAcquireImmutableTip struct {
	protocol.MessageBase
}

func NewMsgReAcquireImmutableTip() *MsgReAcquireImmutableTip {
	m := &MsgReAcquireImmutableTip{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReacquireImmutableTip,
		},
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
