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

package localmessagenotification

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMsgRequestMessagesNonBlocking tests MsgRequestMessages non-blocking variant
func TestMsgRequestMessagesNonBlocking(t *testing.T) {
	msg := &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		IsBlocking: false,
	}

	assert.Equal(t, uint8(MessageTypeRequestMessages), msg.Type())
	assert.False(t, msg.IsBlocking)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeRequestMessages), data)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgRequestMessages)
	assert.True(t, ok)
	assert.Equal(t, msg.IsBlocking, decoded.IsBlocking)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgRequestMessagesBlocking tests MsgRequestMessages blocking variant
func TestMsgRequestMessagesBlocking(t *testing.T) {
	msg := &MsgRequestMessages{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeRequestMessages,
		},
		IsBlocking: true,
	}

	assert.Equal(t, uint8(MessageTypeRequestMessages), msg.Type())
	assert.True(t, msg.IsBlocking)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeRequestMessages), data)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgRequestMessages)
	assert.True(t, ok)
	assert.Equal(t, msg.IsBlocking, decoded.IsBlocking)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessagesNonBlocking tests MsgReplyMessagesNonBlocking struct creation
func TestMsgReplyMessagesNonBlocking(t *testing.T) {
	messages := []pcommon.DmqMessage{
		{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("id1"),
				MessageBody: []byte("body1"),
				KESPeriod:   100,
				ExpiresAt:   2000000000,
			},
			KESSignature: make([]byte, 448),
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: make([]byte, 32),
				IssueNumber:        1,
				KESPeriod:          100,
				ColdSignature:      make([]byte, 64),
			},
			ColdVerificationKey: make([]byte, 32),
		},
	}

	msg := &MsgReplyMessagesNonBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesNonBlocking,
		},
		Messages: messages,
		HasMore:  true,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessagesNonBlocking), msg.Type())
	assert.Equal(t, 1, len(msg.Messages))
	assert.True(t, msg.HasMore)
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(
		uint(MessageTypeReplyMessagesNonBlocking),
		data,
	)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessagesNonBlocking)
	assert.True(t, ok)
	assert.Equal(t, len(msg.Messages), len(decoded.Messages))
	assert.Equal(t, msg.HasMore, decoded.HasMore)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

// TestMsgReplyMessagesNonBlockingEmpty tests empty reply struct with HasMore flag
func TestMsgReplyMessagesNonBlockingEmpty(t *testing.T) {
	msg := &MsgReplyMessagesNonBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesNonBlocking,
		},
		Messages: []pcommon.DmqMessage{},
		HasMore:  false,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessagesNonBlocking), msg.Type())
	assert.Equal(t, 0, len(msg.Messages))
	assert.False(t, msg.HasMore)
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(
		uint(MessageTypeReplyMessagesNonBlocking),
		data,
	)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessagesNonBlocking)
	assert.True(t, ok)
	assert.Equal(t, msg.HasMore, decoded.HasMore)
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

func TestNewMsgReplyMessagesNonBlockingNilRoundTrip(t *testing.T) {
	msg := NewMsgReplyMessagesNonBlocking(nil, false)
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.Equal(t, []byte{0x83, 0x01, 0x80, 0xf4}, data)
	parsed, err := NewMsgFromCbor(
		uint(MessageTypeReplyMessagesNonBlocking),
		data,
	)
	require.NoError(t, err)
	decoded := parsed.(*MsgReplyMessagesNonBlocking)
	require.NotNil(t, decoded.Messages)
	require.Empty(t, decoded.Messages)
}

// TestMsgReplyMessagesBlocking tests MsgReplyMessagesBlocking struct creation
func TestMsgReplyMessagesBlocking(t *testing.T) {
	messages := []pcommon.DmqMessage{
		{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("id1"),
				MessageBody: []byte("body1"),
				KESPeriod:   100,
				ExpiresAt:   2000000000,
			},
			KESSignature: make([]byte, 448),
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: make([]byte, 32),
				IssueNumber:        1,
				KESPeriod:          100,
				ColdSignature:      make([]byte, 64),
			},
			ColdVerificationKey: make([]byte, 32),
		},
		{
			Payload: pcommon.DmqMessagePayload{
				MessageID:   []byte("id2"),
				MessageBody: []byte("body2"),
				KESPeriod:   100,
				ExpiresAt:   2000000000,
			},
			KESSignature: make([]byte, 448),
			OperationalCertificate: pcommon.OperationalCertificate{
				KESVerificationKey: make([]byte, 32),
				IssueNumber:        1,
				KESPeriod:          100,
				ColdSignature:      make([]byte, 64),
			},
			ColdVerificationKey: make([]byte, 32),
		},
	}

	msg := &MsgReplyMessagesBlocking{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeReplyMessagesBlocking,
		},
		Messages: messages,
	}

	assert.Equal(t, uint8(MessageTypeReplyMessagesBlocking), msg.Type())
	assert.Equal(t, 2, len(msg.Messages))
	// CBOR round-trip and field preservation
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeReplyMessagesBlocking), data)
	require.NoError(t, err)
	decoded, ok := parsed.(*MsgReplyMessagesBlocking)
	assert.True(t, ok)
	assert.Equal(t, len(msg.Messages), len(decoded.Messages))
	reencoded, err := cbor.Encode(decoded)
	assert.NoError(t, err)
	assert.Equal(t, data, reencoded)
}

func TestNewMsgReplyMessagesBlockingNilRoundTrip(t *testing.T) {
	msg := NewMsgReplyMessagesBlocking(nil)
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.Equal(t, []byte{0x82, 0x02, 0x80}, data)
	parsed, err := NewMsgFromCbor(
		uint(MessageTypeReplyMessagesBlocking),
		data,
	)
	require.NoError(t, err)
	decoded := parsed.(*MsgReplyMessagesBlocking)
	require.NotNil(t, decoded.Messages)
	require.Empty(t, decoded.Messages)
}

func TestServerDrainValidMessagesDropsExpired(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)
	now := time.Unix(100, 0)
	expired := &pcommon.DmqMessage{
		MessageID: []byte("expired"),
		Payload: pcommon.DmqMessagePayload{
			ExpiresAt: 99,
		},
	}
	valid := &pcommon.DmqMessage{
		MessageID: []byte("valid"),
		Payload: pcommon.DmqMessagePayload{
			ExpiresAt: 101,
		},
	}

	server.lock.Lock()
	server.messageQueue = append(server.messageQueue, expired, valid)
	messages := server.drainValidMessagesLocked(now)
	server.lock.Unlock()

	require.Len(t, messages, 1)
	assert.Equal(t, []byte("valid"), messages[0].ID())
	_, expiredAcked := server.acknowledgedIDs[string(expired.ID())]
	_, validAcked := server.acknowledgedIDs[string(valid.ID())]
	assert.False(t, expiredAcked)
	assert.True(t, validAcked)
	assert.Empty(t, server.messageQueue)
}

// TestMsgClientDoneEncoding tests MsgClientDone encoding
func TestMsgClientDoneEncoding(t *testing.T) {
	msg := &MsgClientDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeClientDone,
		},
	}

	assert.Equal(t, uint8(MessageTypeClientDone), msg.Type())
	data, err := cbor.Encode(msg)
	assert.NoError(t, err)
	parsed, err := NewMsgFromCbor(uint(MessageTypeClientDone), data)
	require.NoError(t, err)
	assert.Equal(t, uint8(MessageTypeClientDone), parsed.Type())
}

// TestNewMsgFromCborUnknownType tests handling of unknown message type
func TestNewMsgFromCborUnknownType(t *testing.T) {
	_, err := NewMsgFromCbor(99, []byte{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown message type")
}

func TestReplyMessageCollectionBudget(t *testing.T) {
	over := DefaultMaxReplyMessages + 1
	for _, msgType := range []uint{
		MessageTypeReplyMessagesNonBlocking,
		MessageTypeReplyMessagesBlocking,
	} {
		t.Run(fmt.Sprintf("message_%d", msgType), func(t *testing.T) {
			// array header with a two-byte count
			wire := []byte{0x82, byte(msgType), 0x99, byte(over >> 8), byte(over)}
			if msgType == MessageTypeReplyMessagesNonBlocking {
				wire[0] = 0x83
			}
			wire = append(wire, bytes.Repeat([]byte{0x80}, over)...)
			if msgType == MessageTypeReplyMessagesNonBlocking {
				wire = append(wire, 0xf4)
			}
			msg, err := NewMsgFromCbor(msgType, wire)
			require.ErrorContains(t, err, "maximum is 1000")
			require.Nil(t, msg)
		})
	}
}

// The reference server replies with up to 1000 messages, so a client must
// accept that many whatever queue size a server of this library is given, and
// refuse one more.
func TestClientReplyLimitIsIndependentOfServerQueueSize(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		valid bool
	}{
		{name: "reference batch", count: 1000, valid: true},
		{name: "above reference batch", count: 1001},
	} {
		t.Run(tc.name, func(t *testing.T) {
			connA, connB := net.Pipe()
			t.Cleanup(func() {
				_ = connA.Close()
				_ = connB.Close()
			})
			m := muxer.New(connA)
			t.Cleanup(m.Stop)
			replies := make(chan int, 1)
			errs := make(chan error, 4)
			cfg := NewConfig(
				WithMaxQueueSize(1),
				WithReplyMessagesFunc(func(
					_ CallbackContext,
					messages []pcommon.DmqMessage,
					_ bool,
				) {
					replies <- len(messages)
				}),
			)
			client := NewClient(protocol.ProtocolOptions{
				Muxer:     m,
				ErrorChan: errs,
			}, &cfg)
			client.Start()
			t.Cleanup(func() { client.Protocol.Stop() })
			m.Start()

			require.NoError(t, client.RequestMessagesBlocking())
			require.NoError(
				t,
				connB.SetReadDeadline(time.Now().Add(2*time.Second)),
			)
			header := muxer.SegmentHeader{}
			require.NoError(t, binary.Read(connB, binary.BigEndian, &header))
			_, err := io.CopyN(io.Discard, connB, int64(header.PayloadLength))
			require.NoError(t, err)

			payload, err := cbor.Encode(
				NewMsgReplyMessagesBlocking(
					make([]pcommon.DmqMessage, tc.count),
				),
			)
			require.NoError(t, err)
			segment := muxer.NewSegment(ProtocolID, payload, true)
			buf := &bytes.Buffer{}
			require.NoError(
				t,
				binary.Write(buf, binary.BigEndian, segment.SegmentHeader),
			)
			buf.Write(segment.Payload)
			_, err = connB.Write(buf.Bytes())
			require.NoError(t, err)

			if tc.valid {
				select {
				case count := <-replies:
					require.Equal(t, tc.count, count)
				case <-time.After(2 * time.Second):
					t.Fatal("reply did not reach the client callback")
				}
				return
			}
			select {
			case err := <-errs:
				require.ErrorContains(t, err, "maximum is 1000")
			case <-time.After(2 * time.Second):
				t.Fatal("oversized reply was not rejected")
			}
		})
	}
}

func TestReplyMessageCollectionUsesConfiguredLimit(t *testing.T) {
	wire := []byte{0x82, MessageTypeReplyMessagesBlocking, 0x82, 0x80, 0x80}
	err := validateReplyCollection(
		MessageTypeReplyMessagesBlocking,
		wire,
		1,
	)
	require.ErrorContains(t, err, "maximum is 1")
	require.NoError(t, validateReplyCollection(
		MessageTypeReplyMessagesBlocking,
		wire,
		2,
	))
}
