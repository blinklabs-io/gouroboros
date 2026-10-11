// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
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
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

func (m *MsgVotes) unmarshalCBORWithLimits(
	data []byte,
	maxCount int,
	maxBytes int,
) error {
	if len(data) > maxBytes {
		return fmt.Errorf(
			"votes response size %d exceeds maximum %d bytes",
			len(data),
			maxBytes,
		)
	}
	fields, err := cborpreflight.ArrayItems(data, 2, "votes response")
	if err != nil {
		return err
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"votes response has %d fields, expected 2",
			len(fields),
		)
	}
	if err := cborpreflight.ValidateItemDepth(
		fields[0],
		0,
		"votes response message type",
	); err != nil {
		return err
	}
	var messageType uint8
	if _, err := cbor.Decode(fields[0], &messageType); err != nil {
		return err
	}
	if messageType != MessageTypeVotes {
		return fmt.Errorf(
			"votes response message type is %d, expected %d",
			messageType,
			MessageTypeVotes,
		)
	}
	votes, err := cborpreflight.ArrayItems(
		fields[1],
		maxCount,
		"votes response vote list",
	)
	if err != nil {
		return err
	}
	m.MessageType = messageType
	m.VotesRaw = make([]cbor.RawMessage, len(votes))
	for idx, vote := range votes {
		if err := cborpreflight.ValidateItemDepth(
			vote,
			1,
			fmt.Sprintf("votes response vote %d", idx),
		); err != nil {
			return err
		}
		m.VotesRaw[idx] = append(cbor.RawMessage(nil), vote...)
	}
	m.SetCbor(data)
	return nil
}

func decodeBlockAndTxsInRange(
	msgType uint,
	data []byte,
	maxTxCount int,
	maxBytes int,
) (protocol.Message, error) {
	if len(data) > maxBytes {
		return nil, fmt.Errorf(
			"block range response size %d exceeds maximum %d bytes",
			len(data),
			maxBytes,
		)
	}
	fields, err := cborpreflight.ArrayItems(
		data,
		3,
		"block range response",
	)
	if err != nil {
		return nil, err
	}
	if len(fields) != 3 {
		return nil, fmt.Errorf(
			"block range response has %d fields, expected 3",
			len(fields),
		)
	}
	if err := cborpreflight.ValidateItemDepth(
		fields[0],
		0,
		"block range response message type",
	); err != nil {
		return nil, err
	}
	var encodedType uint8
	if _, err := cbor.Decode(fields[0], &encodedType); err != nil {
		return nil, err
	}
	if uint(encodedType) != msgType {
		return nil, fmt.Errorf(
			"block range response message type is %d, expected %d",
			encodedType,
			msgType,
		)
	}
	var txs [][]byte
	if len(fields[2]) != 1 || (fields[2][0] != 0xf6 && fields[2][0] != 0xf7) {
		txs, err = cborpreflight.ArrayItems(
			fields[2],
			maxTxCount,
			"block range response transactions",
		)
		if err != nil {
			return nil, err
		}
	}
	block := append(cbor.RawMessage(nil), fields[1]...)
	txRaws := make([]cbor.RawMessage, len(txs))
	for idx, tx := range txs {
		txRaws[idx] = append(cbor.RawMessage(nil), tx...)
	}
	var ret protocol.Message
	switch msgType {
	case MessageTypeNextBlockAndTxsInRange:
		ret = NewMsgNextBlockAndTxsInRange(block, txRaws)
	case MessageTypeLastBlockAndTxsInRange:
		ret = NewMsgLastBlockAndTxsInRange(block, txRaws)
	default:
		return nil, fmt.Errorf("unexpected block range response type %d", msgType)
	}
	ret.SetCbor(data)
	return ret, nil
}
