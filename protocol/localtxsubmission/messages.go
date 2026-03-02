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

package localtxsubmission

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeSubmitTx = 0
	MessageTypeAcceptTx = 1
	MessageTypeRejectTx = 2
	MessageTypeDone     = 3
)

// NewMsgFromCbor parses a LocalTxSubmission message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeSubmitTx:
		ret = &MsgSubmitTx{}
	case MessageTypeAcceptTx:
		ret = &MsgAcceptTx{}
	case MessageTypeRejectTx:
		ret = &MsgRejectTx{}
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

type MsgSubmitTx struct {
	protocol.MessageBase
	Transaction MsgSubmitTxTransaction
}

type MsgSubmitTxTransaction struct {
	cbor.StructAsArray
	EraId uint16
	Raw   cbor.Tag
}

func NewMsgSubmitTx(eraId uint16, tx []byte) *MsgSubmitTx {
	m := &MsgSubmitTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeSubmitTx,
		},
		Transaction: MsgSubmitTxTransaction{
			EraId: eraId,
			Raw: cbor.Tag{
				// Wrapped CBOR
				Number:  24,
				Content: tx,
			},
		},
	}
	return m
}

type MsgAcceptTx struct {
	protocol.MessageBase
}

func NewMsgAcceptTx() *MsgAcceptTx {
	m := &MsgAcceptTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptTx,
		},
	}
	return m
}

type MsgRejectTx struct {
	protocol.MessageBase
	// We use RawMessage here because the failure reason can be numerous different
	// structures, and we'll need to do further processing
	Reason cbor.RawMessage
}

func NewMsgRejectTx(reasonCbor []byte) *MsgRejectTx {
	m := &MsgRejectTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRejectTx,
		},
		Reason: cbor.RawMessage(reasonCbor),
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
