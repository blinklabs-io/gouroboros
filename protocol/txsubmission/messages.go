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

package txsubmission

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeRequestTxIds = 0
	MessageTypeReplyTxIds   = 1
	MessageTypeRequestTxs   = 2
	MessageTypeReplyTxs     = 3
	MessageTypeDone         = 4
	MessageTypeInit         = 6
)

// NewMsgFromCbor parses a TxSubmission message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeRequestTxIds:
		ret = &MsgRequestTxIds{}
	case MessageTypeReplyTxIds:
		ret = &MsgReplyTxIds{}
	case MessageTypeRequestTxs:
		ret = &MsgRequestTxs{}
	case MessageTypeReplyTxs:
		ret = &MsgReplyTxs{}
	case MessageTypeDone:
		ret = &MsgDone{}
	case MessageTypeInit:
		ret = &MsgInit{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %s", ProtocolName, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type MsgRequestTxIds struct {
	protocol.MessageBase
	Blocking bool
	Ack      uint16
	Req      uint16
}

func NewMsgRequestTxIds(
	blocking bool,
	ack uint16,
	req uint16,
) *MsgRequestTxIds {
	m := &MsgRequestTxIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestTxIds,
		},
		Blocking: blocking,
		Ack:      ack,
		Req:      req,
	}
	return m
}

type MsgReplyTxIds struct {
	protocol.MessageBase
	TxIds []TxIdAndSize
}

func (m *MsgReplyTxIds) MarshalCBOR() ([]byte, error) {
	items := []any{}
	for _, txId := range m.TxIds {
		items = append(items, txId)
	}
	tmp := []any{
		MessageTypeReplyTxIds,
		cbor.IndefLengthList{
			Items: items,
		},
	}
	return cbor.Encode(tmp)
}

func NewMsgReplyTxIds(txIds []TxIdAndSize) *MsgReplyTxIds {
	m := &MsgReplyTxIds{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyTxIds,
		},
		TxIds: txIds,
	}
	return m
}

type MsgRequestTxs struct {
	protocol.MessageBase
	TxIds []TxId
}

func (m *MsgRequestTxs) MarshalCBOR() ([]byte, error) {
	items := []any{}
	for _, txId := range m.TxIds {
		items = append(items, txId)
	}
	tmp := []any{
		MessageTypeRequestTxs,
		cbor.IndefLengthList{
			Items: items,
		},
	}
	return cbor.Encode(tmp)
}

func NewMsgRequestTxs(txIds []TxId) *MsgRequestTxs {
	m := &MsgRequestTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestTxs,
		},
		TxIds: txIds,
	}
	return m
}

type MsgReplyTxs struct {
	protocol.MessageBase
	Txs []TxBody
}

func (m *MsgReplyTxs) MarshalCBOR() ([]byte, error) {
	items := []any{}
	for _, tx := range m.Txs {
		items = append(items, tx)
	}
	tmp := []any{
		MessageTypeReplyTxs,
		cbor.IndefLengthList{
			Items: items,
		},
	}
	return cbor.Encode(tmp)
}

func NewMsgReplyTxs(txs []TxBody) *MsgReplyTxs {
	m := &MsgReplyTxs{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyTxs,
		},
		Txs: txs,
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

type MsgInit struct {
	protocol.MessageBase
}

func NewMsgInit() *MsgInit {
	m := &MsgInit{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeInit,
		},
	}
	return m
}

type TxId struct {
	cbor.StructAsArray
	EraId uint16
	TxId  [32]byte
}

type TxBody struct {
	cbor.StructAsArray
	EraId  uint16
	TxBody []byte
}

func (t *TxBody) MarshalCBOR() ([]byte, error) {
	tmp := []any{
		t.EraId,
		cbor.Tag{
			// Wrapped CBOR
			Number:  24,
			Content: t.TxBody,
		},
	}
	return cbor.Encode(&tmp)
}

type TxIdAndSize struct {
	cbor.StructAsArray
	TxId TxId
	Size uint32
}
