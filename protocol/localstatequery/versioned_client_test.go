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

package localstatequery_test

import (
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
)

// conversationEraQueryAt answers a handshake at the given node-to-client
// version, an acquire, the current-era query with era, and then one query
// with result. The shared mock handshake negotiates version 14, which carries
// none of the queries added since.
func conversationEraQueryAt(
	version uint16,
	era byte,
	result []byte,
) []ouroboros_mock.ConversationEntry {
	queryResult := func(
		data []byte,
	) []ouroboros_mock.ConversationEntry {
		return []ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryInput{
				ProtocolId:  localstatequery.ProtocolId,
				MessageType: localstatequery.MessageTypeQuery,
			},
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: localstatequery.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					localstatequery.NewMsgResult(data),
				},
			},
		}
	}
	conversation := []ouroboros_mock.ConversationEntry{
		ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: handshake.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				handshake.NewMsgAcceptVersion(
					version+protocol.ProtocolVersionNtCOffset,
					protocol.VersionDataNtC15andUp{
						CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
					},
				),
			},
		},
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeAcquireVolatileTip,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgAcquired(),
			},
		},
	}
	conversation = append(conversation, queryResult([]byte{era})...)
	return append(conversation, queryResult(result)...)
}
