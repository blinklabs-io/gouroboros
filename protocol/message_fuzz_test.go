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

//go:build go1.18

package protocol_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	gprotocol "github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagenotification"
	"github.com/blinklabs-io/gouroboros/protocol/localmessagesubmission"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/messagesubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

type protocolMessageDecoder struct {
	name       string
	maxMsgType uint
	decode     func(uint, []byte) (gprotocol.Message, error)
}

var protocolMessageDecoders = []protocolMessageDecoder{
	{
		name:       "blockfetch",
		maxMsgType: blockfetch.MessageTypeBatchDone,
		decode:     blockfetch.NewMsgFromCbor,
	},
	{
		name:       "chainsync-ntn",
		maxMsgType: chainsync.MessageTypeDone,
		decode: func(msgType uint, data []byte) (gprotocol.Message, error) {
			return chainsync.NewMsgFromCbor(
				gprotocol.ProtocolModeNodeToNode,
				msgType,
				data,
			)
		},
	},
	{
		name:       "chainsync-ntc",
		maxMsgType: chainsync.MessageTypeDone,
		decode: func(msgType uint, data []byte) (gprotocol.Message, error) {
			return chainsync.NewMsgFromCbor(
				gprotocol.ProtocolModeNodeToClient,
				msgType,
				data,
			)
		},
	},
	{
		name:       "handshake",
		maxMsgType: handshake.MessageTypeQueryReply,
		decode:     handshake.NewMsgFromCbor,
	},
	{
		name:       "keepalive",
		maxMsgType: keepalive.MessageTypeDone,
		decode:     keepalive.NewMsgFromCbor,
	},
	{
		name:       "leiosfetch",
		maxMsgType: leiosfetch.MessageTypeDone,
		decode:     leiosfetch.NewMsgFromCbor,
	},
	{
		name:       "leiosnotify",
		maxMsgType: leiosnotify.MessageTypeDone,
		decode:     leiosnotify.NewMsgFromCbor,
	},
	{
		name:       "localmessagenotification",
		maxMsgType: localmessagenotification.MessageTypeClientDone,
		decode:     localmessagenotification.NewMsgFromCbor,
	},
	{
		name:       "localmessagesubmission",
		maxMsgType: localmessagesubmission.MessageTypeDone,
		decode:     localmessagesubmission.NewMsgFromCbor,
	},
	{
		name:       "localstatequery",
		maxMsgType: localstatequery.MessageTypeReacquireImmutableTip,
		decode:     localstatequery.NewMsgFromCbor,
	},
	{
		name:       "localtxmonitor",
		maxMsgType: localtxmonitor.MessageTypeReplyGetSizes,
		decode:     localtxmonitor.NewMsgFromCbor,
	},
	{
		name:       "localtxsubmission",
		maxMsgType: localtxsubmission.MessageTypeDone,
		decode:     localtxsubmission.NewMsgFromCbor,
	},
	{
		name:       "messagesubmission",
		maxMsgType: messagesubmission.MessageTypeDone,
		decode:     messagesubmission.NewMsgFromCbor,
	},
	{
		name:       "peersharing",
		maxMsgType: peersharing.MessageTypeDone,
		decode:     peersharing.NewMsgFromCbor,
	},
	{
		name:       "txsubmission",
		maxMsgType: txsubmission.MessageTypeInit,
		decode:     txsubmission.NewMsgFromCbor,
	},
}

func FuzzProtocolMessageDecoders(f *testing.F) {
	for decoderIdx, decoder := range protocolMessageDecoders {
		for msgType := uint(0); msgType <= decoder.maxMsgType; msgType++ {
			f.Add(uint8(decoderIdx), msgType, mustEncodeFuzzSeed([]any{msgType}))
			f.Add(
				uint8(decoderIdx),
				msgType,
				mustEncodeFuzzSeed([]any{msgType, uint(0)}),
			)
			f.Add(
				uint8(decoderIdx),
				msgType,
				mustEncodeFuzzSeed([]any{msgType, []any{}}),
			)
		}
		f.Add(uint8(decoderIdx), uint(0), []byte{})
		f.Add(uint8(decoderIdx), uint(0), []byte{0xff})
	}

	f.Fuzz(func(t *testing.T, decoderIdx uint8, msgType uint, data []byte) {
		decoder := protocolMessageDecoders[int(decoderIdx)%len(protocolMessageDecoders)]
		msgType %= decoder.maxMsgType + 1
		_, _ = decoder.decode(msgType, data)
		// Should not panic - malformed messages should return decode errors.
	})
}

func mustEncodeFuzzSeed(v any) []byte {
	data, err := cbor.Encode(v)
	if err != nil {
		panic(err)
	}
	return data
}
