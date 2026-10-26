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
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// oversizedReply is a well-formed array of 1000 one-byte items, far beyond the
// one or two elements any of the queried results carries.
func oversizedReply() []byte {
	return append([]byte{0x99, 0x03, 0xe8}, make([]byte, 1000)...)
}

func TestFixedShapeClientRepliesRejectOversizedArrays(t *testing.T) {
	// Not t.Parallel: runTest registers goleak.VerifyNone, a process-wide
	// goroutine check that sibling parallel tests would perturb.
	tests := []struct {
		name         string
		conversation []ouroboros_mock.ConversationEntry
		query        func(*localstatequery.Client) error
	}{
		{
			name:         "chain block number",
			conversation: conversationHandshakeAcquire,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetChainBlockNo()
				return err
			},
		},
		{
			name:         "epoch number",
			conversation: conversationCurrentEra,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetEpochNo()
				return err
			},
		},
		{
			name:         "current protocol parameters",
			conversation: conversationCurrentEra,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetCurrentProtocolParams()
				return err
			},
		},
		{
			name:         "genesis config",
			conversation: conversationCurrentEra,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetGenesisConfig()
				return err
			},
		},
		{
			name:         "non-myopic member rewards",
			conversation: conversationCurrentEra,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetNonMyopicMemberRewards([]any{uint64(1)})
				return err
			},
		},
		{
			name:         "stake snapshots",
			conversation: conversationCurrentEra,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetStakeSnapshots(nil)
				return err
			},
		},
		{
			name:         "DRep state",
			conversation: conversationConwayEra,
			query: func(c *localstatequery.Client) error {
				_, err := c.GetDRepState(nil)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conversation := append(
				append(
					[]ouroboros_mock.ConversationEntry{},
					tt.conversation...),
				ouroboros_mock.ConversationEntryInput{
					ProtocolId:  localstatequery.ProtocolId,
					MessageType: localstatequery.MessageTypeQuery,
				},
				ouroboros_mock.ConversationEntryOutput{
					ProtocolId: localstatequery.ProtocolId,
					IsResponse: true,
					Messages: []protocol.Message{
						localstatequery.NewMsgResult(oversizedReply()),
					},
				},
			)
			runTest(
				t,
				conversation,
				func(t *testing.T, oConn *ouroboros.Connection) {
					err := tt.query(oConn.LocalStateQuery().Client)
					require.Error(t, err)
					require.Contains(t, err.Error(), "maximum is")
				},
			)
		})
	}
}
