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

package localtxmonitor

import (
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeDone          = 0
	MessageTypeAcquire       = 1
	MessageTypeAcquired      = 2
	MessageTypeRelease       = 3
	MessageTypeNextTx        = 5
	MessageTypeReplyNextTx   = 6
	MessageTypeHasTx         = 7
	MessageTypeReplyHasTx    = 8
	MessageTypeGetSizes      = 9
	MessageTypeReplyGetSizes = 10
)

// NewMsgFromCbor parses a LocalTxMonitor message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeDone:
		ret = &MsgDone{}
	case MessageTypeAcquire:
		ret = &MsgAcquire{}
	case MessageTypeAcquired:
		ret = &MsgAcquired{}
	case MessageTypeRelease:
		ret = &MsgRelease{}
	case MessageTypeNextTx:
		ret = &MsgNextTx{}
	case MessageTypeReplyNextTx:
		ret = &MsgReplyNextTx{}
	case MessageTypeHasTx:
		ret = &MsgHasTx{}
	case MessageTypeReplyHasTx:
		ret = &MsgReplyHasTx{}
	case MessageTypeGetSizes:
		ret = &MsgGetSizes{}
	case MessageTypeReplyGetSizes:
		ret = &MsgReplyGetSizes{}
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

type MsgAcquire struct {
	protocol.MessageBase
}

func NewMsgAcquire() *MsgAcquire {
	m := &MsgAcquire{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcquire,
		},
	}
	return m
}

type MsgAcquired struct {
	protocol.MessageBase
	SlotNo uint64
}

func NewMsgAcquired(slotNo uint64) *MsgAcquired {
	m := &MsgAcquired{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcquired,
		},
		SlotNo: slotNo,
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

type MsgNextTx struct {
	protocol.MessageBase
}

func NewMsgNextTx() *MsgNextTx {
	m := &MsgNextTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeNextTx,
		},
	}
	return m
}

type MsgReplyNextTx struct {
	protocol.MessageBase
	Transaction MsgReplyNextTxTransaction
}

type MsgReplyNextTxTransaction struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_     struct{} `cbor:",toarray"`
	EraId uint8
	Tx    []byte
}

func NewMsgReplyNextTx(eraId uint8, tx []byte) *MsgReplyNextTx {
	m := &MsgReplyNextTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyNextTx,
		},
		Transaction: MsgReplyNextTxTransaction{
			EraId: eraId,
			Tx:    tx,
		},
	}
	return m
}

func (m *MsgReplyNextTx) UnmarshalCBOR(data []byte) error {
	var tmp []interface{}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	// We know what the value will be, but it doesn't hurt to use the actual value from the message
	m.MessageType = uint8(tmp[0].(uint64))
	// The ReplyNextTx message has a variable number of arguments
	if len(tmp) > 1 {
		txWrapper := tmp[1].([]interface{})
		m.Transaction = MsgReplyNextTxTransaction{
			EraId: uint8(txWrapper[0].(uint64)),
			Tx:    txWrapper[1].(cbor.WrappedCbor).Bytes(),
		}
	}
	return nil
}

func (m *MsgReplyNextTx) MarshalCBOR() ([]byte, error) {
	tmp := []interface{}{m.MessageType}
	if m.Transaction.Tx != nil {
		type tmpTxObj struct {
			// Tells the CBOR decoder to convert to/from a struct and a CBOR array
			_     struct{} `cbor:",toarray"`
			EraId uint8
			Tx    cbor.Tag
		}
		tmpTx := tmpTxObj{
			EraId: m.Transaction.EraId,
			Tx: cbor.Tag{
				// Magic number for a wrapped bytestring
				Number:  0x18,
				Content: m.Transaction.Tx,
			},
		}
		tmp = append(tmp, tmpTx)
	}
	data, err := cbor.Encode(tmp)
	if err != nil {
		return nil, err
	}
	return data, nil
}

type MsgHasTx struct {
	protocol.MessageBase
	TxId []byte
}

func NewMsgHasTx(txId []byte) *MsgHasTx {
	m := &MsgHasTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeHasTx,
		},
		TxId: txId,
	}
	return m
}

type MsgReplyHasTx struct {
	protocol.MessageBase
	Result bool
}

func NewMsgReplyHasTx(result bool) *MsgReplyHasTx {
	m := &MsgReplyHasTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyHasTx,
		},
		Result: result,
	}
	return m
}

type MsgGetSizes struct {
	protocol.MessageBase
}

func NewMsgGetSizes() *MsgGetSizes {
	m := &MsgGetSizes{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeGetSizes,
		},
	}
	return m
}

type MsgReplyGetSizes struct {
	protocol.MessageBase
	Result MsgReplyGetSizesResult
}

type MsgReplyGetSizesResult struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_           struct{} `cbor:",toarray"`
	Capacity    uint32
	Size        uint32
	NumberOfTxs uint32
}

func NewMsgReplyGetSizes(
	capacity uint32,
	size uint32,
	numberOfTxs uint32,
) *MsgReplyGetSizes {
	m := &MsgReplyGetSizes{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyGetSizes,
		},
		Result: MsgReplyGetSizesResult{
			Capacity:    capacity,
			Size:        size,
			NumberOfTxs: numberOfTxs,
		},
	}
	return m
}
