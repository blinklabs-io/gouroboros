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

package ouroboros_mock

import (
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
)

const (
	MockNetworkMagic       uint32 = 999999
	MockProtocolVersionNtC uint16 = 14
)

type EntryType int

const (
	EntryTypeNone   EntryType = 0
	EntryTypeInput  EntryType = 1
	EntryTypeOutput EntryType = 2
	EntryTypeClose  EntryType = 3
)

type ConversationEntry struct {
	Type             EntryType
	ProtocolId       uint16
	IsResponse       bool
	OutputMessages   []protocol.Message
	InputMessage     protocol.Message
	InputMessageType uint
	MsgFromCborFunc  protocol.MessageFromCborFunc
}

// ConversationEntryHandshakeRequestGeneric is a pre-defined conversation event that matches a generic
// handshake request from a client
var ConversationEntryHandshakeRequestGeneric = ConversationEntry{
	Type:             EntryTypeInput,
	ProtocolId:       handshake.ProtocolId,
	InputMessageType: handshake.MessageTypeProposeVersions,
}

// ConversationEntryHandshakeResponse is a pre-defined conversation entry for a server NtC handshake response
var ConversationEntryHandshakeResponse = ConversationEntry{
	Type:       EntryTypeOutput,
	ProtocolId: handshake.ProtocolId,
	IsResponse: true,
	OutputMessages: []protocol.Message{
		handshake.NewMsgAcceptVersion(MockProtocolVersionNtC, MockNetworkMagic),
	},
}
