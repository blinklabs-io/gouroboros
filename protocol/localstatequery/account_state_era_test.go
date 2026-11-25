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
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// TestGetAccountStateEveryShelleyEra checks GetAccountState is sent in every
// Shelley-based era. The reference answers it in all of them
// (ouroboros-consensus Shelley/Ledger/Query.hs: decodeShelleyQuery (1, 29)
// without requireCG, blockQueryIsSupportedOnVersion const True, answered by
// queryChainAccountState), and the ChainAccountState reply is
// [treasury, reserves] in every era. The expected query is the
// ouroboros-consensus ec239d57f618 golden
// QueryVersion3/CardanoNodeToClientVersion19/Query_<Era>_GetAccountState
// inside MsgQuery.
func TestGetAccountStateEveryShelleyEra(t *testing.T) {
	eras := []string{"", "Shelley", "Allegra", "Mary", "Alonzo", "Babbage", "Conway", "Dijkstra"}
	for era := 1; era < len(eras); era++ {
		t.Run(eras[era], func(t *testing.T) {
			conversation := conversationEraQueryAt(23, byte(era), nil)[:6]
			var mu sync.Mutex
			var sent []byte
			expected := localstatequery.NewMsgQuery(nil)
			conversation = append(
				conversation,
				ouroboros_mock.ConversationEntryInput{
					ProtocolId: localstatequery.ProtocolId,
					Message:    expected,
					MsgFromCborFunc: func(
						msgType uint,
						data []byte,
					) (protocol.Message, error) {
						mu.Lock()
						sent = append([]byte{}, data...)
						mu.Unlock()
						return expected, nil
					},
				},
				ouroboros_mock.ConversationEntryOutput{
					ProtocolId: localstatequery.ProtocolId,
					IsResponse: true,
					Messages: []protocol.Message{
						// [[treasury 7, reserves 9]]
						localstatequery.NewMsgResult([]byte{0x81, 0x82, 0x07, 0x09}),
					},
				},
			)
			runTest(t, conversation, func(t *testing.T, oConn *ouroboros.Connection) {
				result, err := oConn.LocalStateQuery().Client.GetAccountState()
				require.NoError(t, err)
				require.Equal(t, int64(7), result.State.Treasury)
				require.Equal(t, int64(9), result.State.Reserves)
			})
			mu.Lock()
			defer mu.Unlock()
			want := fmt.Sprintf("8203820082008%d81181d", 200+era)
			require.Equal(t, want, hex.EncodeToString(sent))
		})
	}
}
