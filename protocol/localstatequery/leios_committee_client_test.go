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
	"math/big"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/internal/test"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// TestGetLeiosCommittee answers GetLeiosCommittee with cardano-ledger's golden
// QueryResultLeiosCommittee (current and next committee of one keyed and one
// keyless seat), inside the one-element array the consensus layer wraps a
// reply in.
func TestGetLeiosCommittee(t *testing.T) {
	conversation := append(
		conversationCurrentEra,
		ouroboros_mock.ConversationEntryInput{
			ProtocolId:  localstatequery.ProtocolId,
			MessageType: localstatequery.MessageTypeQuery,
		},
		ouroboros_mock.ConversationEntryOutput{
			ProtocolId: localstatequery.ProtocolId,
			IsResponse: true,
			Messages: []protocol.Message{
				localstatequery.NewMsgResult(
					test.DecodeHexString(
						"81828282d81e820308815860a57565542eaa01ef2b910bf0eaba4d98a1e5b8b79cc425db08f8780732d0ea9bc85fc6175f27" +
							"2b2344bb27bc572ebf14022e52689dcedfccf44a00e5bd1aa59db44517217d6b0f21b372169ee761938c28914ddcb9663de5" +
							"4db288e760a8e14f82d81e82010480818282d81e820308815860a57565542eaa01ef2b910bf0eaba4d98a1e5b8b79cc425db" +
							"08f8780732d0ea9bc85fc6175f272b2344bb27bc572ebf14022e52689dcedfccf44a00e5bd1aa59db44517217d6b0f21b372" +
							"169ee761938c28914ddcb9663de54db288e760a8e14f82d81e82010480",
					),
				),
			},
		},
	)
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			result, err := oConn.LocalStateQuery().Client.GetLeiosCommittee()
			require.NoError(t, err)
			require.Len(t, result.Current, 2)
			require.Zero(t, big.NewRat(3, 8).Cmp(result.Current[0].Weight))
			require.Len(t, result.Current[0].VKey, 96)
			require.Zero(t, big.NewRat(1, 4).Cmp(result.Current[1].Weight))
			require.Nil(t, result.Current[1].VKey)
			require.Len(t, result.Next, 2)
			require.Zero(t, big.NewRat(3, 8).Cmp(result.Next[0].Weight))
			require.Equal(t, result.Current[0].VKey, result.Next[0].VKey)
			require.Zero(t, big.NewRat(1, 4).Cmp(result.Next[1].Weight))
			require.Nil(t, result.Next[1].VKey)
		},
	)
}
