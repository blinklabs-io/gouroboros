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

package handshake

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"reflect"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestHandshakeCollectionsRejectBeforeAllocation(t *testing.T) {
	for _, tc := range []struct {
		name      string
		kind      uint
		wire      []byte
		maxAlloc  int64
		errorText string
	}{
		{
			name: "duplicate version map",
			kind: MessageTypeProposeVersions,
			wire: append([]byte{0x82, MessageTypeProposeVersions, 0xb9, 0x09, 0xc4},
				bytes.Repeat([]byte{0, 0}, 2500)...),
			maxAlloc:  128 << 10,
			errorText: "duplicate handshake version",
		},
		{
			name: "refusal field count",
			kind: MessageTypeRefuse,
			wire: append([]byte{0x82, MessageTypeRefuse, 0x99, 0x13, 0x88},
				bytes.Repeat([]byte{0}, 5000)...),
			maxAlloc:  64 << 10,
			errorText: "maximum is 3",
		},
		{
			name: "version mismatch supported-version count",
			kind: MessageTypeRefuse,
			wire: []byte{
				0x82, MessageTypeRefuse, 0x82,
				byte(RefuseReasonVersionMismatch),
				0x9a, 0x00, 0x01, 0x00, 0x00,
			},
			maxAlloc:  64 << 10,
			errorText: "supported versions",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _ = NewMsgFromCbor(MessageTypeProposeVersions, []byte{0x82, MessageTypeProposeVersions, 0xa0})
			result := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					msg, err := NewMsgFromCbor(tc.kind, tc.wire)
					if err == nil || msg != nil {
						b.Fatal("invalid handshake collection accepted")
					}
				}
			})
			require.LessOrEqual(t, result.AllocedBytesPerOp(), tc.maxAlloc)
			_, err := NewMsgFromCbor(tc.kind, tc.wire)
			require.ErrorContains(t, err, tc.errorText)
		})
	}
}

func TestHandshakePreservesLargeValidCollections(t *testing.T) {
	versions := make(map[uint16]cbor.RawMessage, 1000)
	for idx := range uint16(1000) {
		versions[idx] = cbor.RawMessage{0xf4}
	}
	wire, err := cbor.Encode([]any{MessageTypeProposeVersions, versions})
	require.NoError(t, err)
	require.LessOrEqual(t, len(wire), MaxPendingMessageBytes)
	msg, err := NewMsgFromCbor(MessageTypeProposeVersions, wire)
	require.NoError(t, err)
	require.Len(t, msg.(*MsgProposeVersions).VersionMap, len(versions))

	supported := make([]uint16, 1000)
	for idx := range supported {
		supported[idx] = uint16(idx)
	}
	wire, err = cbor.Encode([]any{
		MessageTypeRefuse,
		[]any{RefuseReasonVersionMismatch, supported},
	})
	require.NoError(t, err)
	require.LessOrEqual(t, len(wire), MaxPendingMessageBytes)
	msg, err = NewMsgFromCbor(MessageTypeRefuse, wire)
	require.NoError(t, err)
	require.Len(t, msg.(*MsgRefuse).Reason, 2)
}

func TestHandshakeExtendedHeaderCountsUseTheirEncodedSize(t *testing.T) {
	versionCount := maxHandshakeVersions + 1
	versionWire := []byte{0x82, MessageTypeProposeVersions, 0xb9, byte(versionCount >> 8), byte(versionCount)}
	_, err := NewMsgFromCbor(MessageTypeProposeVersions, versionWire)
	require.ErrorContains(t, err, fmt.Sprintf("maximum is %d", maxHandshakeVersions))

	supportedCount := maxRefusalSupportedVersions + 1
	supportedWire := []byte{0x82, MessageTypeRefuse, 0x82, byte(RefuseReasonVersionMismatch), 0x99, byte(supportedCount >> 8), byte(supportedCount)}
	_, err = NewMsgFromCbor(MessageTypeRefuse, supportedWire)
	require.ErrorContains(t, err, fmt.Sprintf("maximum is %d", maxRefusalSupportedVersions))
}

type testDefinition struct {
	CborHex     string
	Message     protocol.Message
	MessageType uint
}

var tests = []testDefinition{
	{
		CborHex:     "8200a4078202f4088202f4098202f40a8202f4",
		MessageType: MessageTypeProposeVersions,
		Message: NewMsgProposeVersions(
			map[uint16]protocol.VersionData{
				7: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
				8: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
				9: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
				10: protocol.VersionDataNtN7to10{
					CborNetworkMagic:                       2,
					CborInitiatorAndResponderDiffusionMode: false,
				},
			},
		),
	},
	{
		CborHex:     "83010a8202f4",
		MessageType: MessageTypeAcceptVersion,
		Message: NewMsgAcceptVersion(
			10,
			protocol.VersionDataNtN7to10{
				CborNetworkMagic:                       2,
				CborInitiatorAndResponderDiffusionMode: false,
			},
		),
	},
	{
		CborHex:     "82028200840708090a",
		MessageType: MessageTypeRefuse,
		Message: NewMsgRefuse(
			[]any{
				uint64(RefuseReasonVersionMismatch),
				[]any{
					uint64(7),
					uint64(8),
					uint64(9),
					uint64(10),
				},
			},
		),
	},
	{
		CborHex:     "820283010163666f6f",
		MessageType: MessageTypeRefuse,
		Message: NewMsgRefuse(
			[]any{
				uint64(RefuseReasonDecodeError),
				uint64(1),
				"foo",
			},
		),
	},
	{
		CborHex:     "820283020163666f6f",
		MessageType: MessageTypeRefuse,
		Message: NewMsgRefuse(
			[]any{
				uint64(RefuseReasonRefused),
				uint64(1),
				"foo",
			},
		),
	},
}

func TestDecode(t *testing.T) {
	for _, test := range tests {
		cborData, err := hex.DecodeString(test.CborHex)
		if err != nil {
			t.Fatalf("failed to decode CBOR hex: %s", err)
		}
		msg, err := NewMsgFromCbor(test.MessageType, cborData)
		if err != nil {
			t.Fatalf("failed to decode CBOR: %s", err)
		}
		// Set the raw CBOR so the comparison should succeed
		test.Message.SetCbor(cborData)
		if !reflect.DeepEqual(msg, test.Message) {
			t.Fatalf(
				"CBOR did not decode to expected message object\n  got:    %#v\n  wanted: %#v",
				msg,
				test.Message,
			)
		}
	}
}

func TestEncode(t *testing.T) {
	for _, test := range tests {
		cborData, err := cbor.Encode(test.Message)
		if err != nil {
			t.Fatalf("failed to encode message to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != test.CborHex {
			t.Fatalf(
				"message did not encode to expected CBOR\n  got:    %s\n  wanted: %s",
				cborHex,
				test.CborHex,
			)
		}
	}
}

func TestClientHandleAcceptVersionUnsupportedVersion(t *testing.T) {
	cfg := NewConfig(WithFinishedFunc(
		func(CallbackContext, uint16, protocol.VersionData) error {
			t.Fatal("finished callback should not be called")
			return nil
		},
	))
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
	}, &cfg)
	msg := &MsgAcceptVersion{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeAcceptVersion,
		},
		Version: 0xffff,
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("handleAcceptVersion panicked: %v", r)
		}
	}()
	if err := client.handleAcceptVersion(msg); err == nil {
		t.Fatal("expected unsupported version error")
	}
}

func TestNewMsgFromCborUnknownType(t *testing.T) {
	msg, err := NewMsgFromCbor(999, []byte{0x80})
	require.Error(t, err)
	require.Nil(t, msg)
	require.Contains(t, err.Error(), ProtocolName)
	require.Contains(t, err.Error(), "999")
}

func TestHandshakeRejectsDeepVersionDataBeforeTypedDecode(t *testing.T) {
	versionData := append(bytes.Repeat([]byte{0x81}, 64), 0)
	wire := append([]byte{0x82, MessageTypeProposeVersions, 0xa1, 0}, versionData...)

	_, err := NewMsgFromCbor(MessageTypeProposeVersions, wire)
	require.ErrorContains(t, err, "handshake message: CBOR nesting exceeds maximum depth 4")
}

func TestHandshakeRejectsDeepScalarBeforeTypedDecode(t *testing.T) {
	t.Parallel()
	deepTags := bytes.Repeat(
		[]byte{0xd9, 0x03, 0xe8},
		cbor.MaxNestedLevels+1,
	)
	for _, tc := range []struct {
		name    string
		msgType uint
		wire    []byte
		want    string
	}{
		{
			name:    "version key",
			msgType: MessageTypeProposeVersions,
			wire: append(
				append([]byte{0x82, MessageTypeProposeVersions, 0xa1}, deepTags...),
				0x00, 0xf4,
			),
			want: "handshake version must be an unsigned integer",
		},
		{
			name:    "refusal reason",
			msgType: MessageTypeRefuse,
			wire: append(
				append([]byte{0x82, MessageTypeRefuse, 0x82}, deepTags...),
				byte(RefuseReasonVersionMismatch), 0x80,
			),
			want: "handshake refusal reason must be an unsigned integer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewMsgFromCbor(tc.msgType, tc.wire)
			require.ErrorContains(t, err, tc.want)
		})
	}
}
