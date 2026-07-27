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

package chainsync

import (
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestMsgRollForwardNtCUnmarshalRejectsNonByteTagContent(t *testing.T) {
	data, err := cbor.Encode(
		[]any{
			uint(MessageTypeRollForward),
			cbor.Tag{Number: 24, Content: uint(123)},
			Tip{
				Point:       pcommon.NewPointOrigin(),
				BlockNumber: 0,
			},
		},
	)
	require.NoError(t, err)

	var msg MsgRollForwardNtC
	err = msg.UnmarshalCBOR(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrapped block tag content must be []byte")
}

func TestWrappedHeaderUnmarshalRejectsNonByteTagContent(t *testing.T) {
	data, err := cbor.Encode(
		[]any{
			uint(ledger.BlockHeaderTypeShelley),
			cbor.Tag{Number: 24, Content: uint(1)},
		},
	)
	require.NoError(t, err)

	var wrapped WrappedHeader
	err = wrapped.UnmarshalCBOR(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrapped header tag content must be []byte")
}

func TestWrappedHeaderUnmarshalRejectsNonByteByronTagContent(t *testing.T) {
	data, err := cbor.Encode(
		[]any{
			uint(ledger.BlockHeaderTypeByron),
			[]any{
				[]any{uint(0), uint(0)},
				cbor.Tag{Number: 24, Content: uint(1)},
			},
		},
	)
	require.NoError(t, err)

	var wrapped WrappedHeader
	err = wrapped.UnmarshalCBOR(data)
	require.Error(t, err)
	require.Contains(t, err.Error(), "wrapped byron header tag content must be []byte")
}

func TestHandleRollForwardNtNRejectsUnknownHeaderEra(t *testing.T) {
	callbackCalled := false
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Mode:         protocol.ProtocolModeNodeToNode,
		},
		&Config{
			RollForwardFunc: func(
				CallbackContext,
				uint,
				any,
				Tip,
			) error {
				callbackCalled = true
				return nil
			},
		},
	)
	const unknownEra = uint(999)
	msg := &MsgRollForwardNtN{
		WrappedHeader: WrappedHeader{Era: unknownEra},
	}

	err := client.handleRollForward(msg)

	require.EqualError(t, err, "unknown block header era: 999")
	require.False(t, callbackCalled)
}

func TestHandleRollForwardNtNUnknownEraSkipsRawCallback(t *testing.T) {
	callbackCalled := false
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Mode:         protocol.ProtocolModeNodeToNode,
		},
		&Config{
			RollForwardRawFunc: func(
				CallbackContext,
				uint,
				[]byte,
				Tip,
			) error {
				callbackCalled = true
				return nil
			},
		},
	)
	const unknownEra = uint(999)
	msg := &MsgRollForwardNtN{
		WrappedHeader: WrappedHeader{Era: unknownEra},
	}

	err := client.handleRollForward(msg)

	require.EqualError(t, err, "unknown block header era: 999")
	require.False(t, callbackCalled)
}

func testConnectionId() connection.ConnectionId {
	return connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{},
		RemoteAddr: &net.TCPAddr{},
	}
}
