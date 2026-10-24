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

package localtxmonitor

import (
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
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
	default:
		return nil, fmt.Errorf(
			"%s: unknown message type: %d",
			ProtocolName,
			msgType,
		)
	}
	var decodeErr error
	if reply, ok := ret.(*MsgReplyNextTx); ok {
		decodeErr = reply.UnmarshalCBOR(data)
	} else {
		_, decodeErr = cbor.Decode(data, ret)
	}
	if decodeErr != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, decodeErr)
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
	cbor.StructAsArray
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
	if err := validateReplyNextTx(data); err != nil {
		return err
	}
	return decodeReplyNextTx(data, m)
}

func decodeReplyNextTx(data []byte, m *MsgReplyNextTx) error {
	var tmp []any
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	if len(tmp) == 0 {
		return nil
	}
	messageType64, ok := tmp[0].(uint64)
	if !ok {
		return fmt.Errorf("message type must be uint64, got %T", tmp[0])
	}
	if messageType64 > math.MaxUint8 {
		return errors.New("message type integer overflow")
	}
	// We know what the value will be, but it doesn't hurt to use the actual value from the message
	m.MessageType = uint8(messageType64)
	// The ReplyNextTx message has a variable number of arguments
	if len(tmp) > 1 {
		txWrapper, ok := tmp[1].([]any)
		if !ok {
			return fmt.Errorf(
				"transaction wrapper must be []any, got %T",
				tmp[1],
			)
		}
		if len(txWrapper) < 2 {
			return errors.New(
				"transaction wrapper must have at least 2 elements",
			)
		}
		eraId64, ok := txWrapper[0].(uint64)
		if !ok {
			return fmt.Errorf("era ID must be uint64, got %T", txWrapper[0])
		}
		if eraId64 > math.MaxUint8 {
			return errors.New("era id integer overflow")
		}
		txBytes, ok := txWrapper[1].(cbor.WrappedCbor)
		if !ok {
			return fmt.Errorf(
				"transaction bytes must be WrappedCbor, got %T",
				txWrapper[1],
			)
		}
		m.Transaction = MsgReplyNextTxTransaction{
			EraId: uint8(eraId64),
			Tx:    txBytes.Bytes(),
		}
	}
	return nil
}

func (m *MsgReplyNextTx) MarshalCBOR() ([]byte, error) {
	tmp := []any{m.MessageType}
	if m.Transaction.Tx != nil {
		type tmpTxObj struct {
			cbor.StructAsArray
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
	EraId  uint8
	TxId   []byte
	eraSet bool
}

var errHasTxEraRequired = errors.New(
	"local-tx-monitor: HasTx requires an era ID",
)

// NewMsgHasTx constructs an incomplete HasTx message from an untagged
// transaction ID. The message cannot be encoded because the node-to-client
// protocol requires the transaction's era.
//
// Deprecated: use NewMsgHasTxForEra.
func NewMsgHasTx(txId []byte) *MsgHasTx {
	return &MsgHasTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeHasTx,
		},
		TxId: txId,
	}
}

func NewMsgHasTxForEra(eraId uint8, txId []byte) *MsgHasTx {
	m := &MsgHasTx{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeHasTx,
		},
		EraId:  eraId,
		TxId:   txId,
		eraSet: true,
	}
	return m
}

func (m *MsgHasTx) MarshalCBOR() ([]byte, error) {
	if !m.eraSet {
		return nil, errHasTxEraRequired
	}
	return cbor.Encode([]any{
		m.MessageType,
		[]any{m.EraId, m.TxId},
	})
}

// UnmarshalCBOR decodes a HasTx message with the hard-fork era wrapper and
// requires a transaction ID of exactly lcommon.Blake2b256Size bytes, so an
// invalid ID is refused before it is formatted or looked up.
func (m *MsgHasTx) UnmarshalCBOR(data []byte) error {
	var tmp struct {
		cbor.StructAsArray
		MessageType uint8
		Transaction struct {
			cbor.StructAsArray
			EraId uint8
			TxId  []byte
		}
	}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	if err := validateTxId(tmp.Transaction.TxId); err != nil {
		return err
	}
	m.MessageType = tmp.MessageType
	m.EraId = tmp.Transaction.EraId
	m.TxId = tmp.Transaction.TxId
	m.eraSet = true
	return nil
}

func validateTxId(txId []byte) error {
	if len(txId) != lcommon.Blake2b256Size {
		return fmt.Errorf(
			"%s: transaction ID must be %d bytes, got %d",
			ProtocolName,
			lcommon.Blake2b256Size,
			len(txId),
		)
	}
	return nil
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
	cbor.StructAsArray
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
