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
	"strings"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// TestPoolSetQueriesSendCanonicalSets pins the pool set each pool query puts
// on the wire. The node's set decoder requires tag 258, a definite-length
// array and strictly ascending members, and otherwise drops the connection,
// so a caller's unsorted pools, or more than 23 of them, must still go out in
// that form.
func TestPoolSetQueriesSendCanonicalSets(t *testing.T) {
	const (
		poolCount = 30
		conwayEra = 6
		version   = 23
	)
	descending := make([]ledger.PoolId, 0, poolCount)
	ascending := make([]ledger.PoolId, 0, poolCount)
	for b := poolCount; b >= 1; b-- {
		descending = append(descending, ledger.PoolId(ledger.NewBlake2b224(
			[]byte(strings.Repeat(string([]byte{byte(b)}), 28)),
		)))
	}
	for i := len(descending) - 1; i >= 0; i-- {
		ascending = append(ascending, descending[i])
	}
	anyPools := make([]any, 0, len(descending))
	for _, pool := range descending {
		anyPools = append(anyPools, pool)
	}
	array, err := cbor.Encode(ascending)
	require.NoError(t, err)
	canonical, err := cbor.Encode(
		cbor.RawTag{Number: cbor.CborTagSet, Content: array},
	)
	require.NoError(t, err)
	require.Equal(t, []byte{0xd9, 0x01, 0x02, 0x98, poolCount}, canonical[:5])

	// The filtered queries carry a StrictMaybe, a one-element list holding the
	// set; the others carry the set itself.
	filter := []any{cbor.RawMessage(canonical)}
	set := cbor.RawMessage(canonical)
	cases := []struct {
		name   string
		typeID int
		arg    any
		call   func(*localstatequery.Client) error
	}{
		{
			name:   "GetPoolState",
			typeID: localstatequery.QueryTypeShelleyPoolState,
			arg:    filter,
			call: func(c *localstatequery.Client) error {
				_, err := c.GetPoolState(anyPools)
				return err
			},
		},
		{
			name:   "GetPoolDistr",
			typeID: localstatequery.QueryTypeShelleyPoolDistr,
			arg:    filter,
			call: func(c *localstatequery.Client) error {
				_, err := c.GetPoolDistr(anyPools)
				return err
			},
		},
		{
			name:   "GetPoolDistr2",
			typeID: localstatequery.QueryTypeShelleyPoolDistr2,
			arg:    filter,
			call: func(c *localstatequery.Client) error {
				_, err := c.GetPoolDistr2(descending)
				return err
			},
		},
		{
			name:   "GetStakeSnapshots",
			typeID: localstatequery.QueryTypeShelleyStakeSnapshots,
			arg:    filter,
			call: func(c *localstatequery.Client) error {
				_, err := c.GetStakeSnapshots(anyPools)
				return err
			},
		},
		{
			name:   "GetStakePoolParams",
			typeID: localstatequery.QueryTypeShelleyStakePoolParams,
			arg:    set,
			call: func(c *localstatequery.Client) error {
				_, err := c.GetStakePoolParams(descending)
				return err
			},
		},
		{
			name:   "GetSPOStakeDistr",
			typeID: localstatequery.QueryTypeShelleySPOStakeDistr,
			arg:    set,
			call: func(c *localstatequery.Client) error {
				_, err := c.GetSPOStakeDistr(descending)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := cbor.Encode([]any{
				localstatequery.MessageTypeQuery,
				[]any{
					localstatequery.QueryTypeBlock,
					[]any{
						localstatequery.QueryTypeShelley,
						[]any{conwayEra, []any{tc.typeID, tc.arg}},
					},
				},
			})
			require.NoError(t, err)
			expected, err := localstatequery.NewMsgFromCbor(
				localstatequery.MessageTypeQuery,
				wire,
			)
			require.NoError(t, err)
			expected.SetCbor(nil)

			// Everything up to the final query is the shared handshake,
			// acquire and era query; the last query is replaced by one that
			// must match the expected bytes exactly.
			setup := conversationEraQueryAt(version, conwayEra, nil)
			conversation := append(
				append([]ouroboros_mock.ConversationEntry{},
					setup[:len(setup)-2]...),
				ouroboros_mock.ConversationEntryInput{
					ProtocolId: localstatequery.ProtocolId,
					Message:    expected,
					MsgFromCborFunc: func(
						msgType uint,
						data []byte,
					) (protocol.Message, error) {
						return localstatequery.NewMsgFromCbor(msgType, data)
					},
				},
				ouroboros_mock.ConversationEntryOutput{
					ProtocolId: localstatequery.ProtocolId,
					IsResponse: true,
					Messages: []protocol.Message{
						localstatequery.NewMsgResult([]byte{0x80}),
					},
				},
			)
			runTest(
				t,
				conversation,
				func(t *testing.T, oConn *ouroboros.Connection) {
					// The reply is not a valid result; only the request
					// bytes are under test.
					_ = tc.call(oConn.LocalStateQuery().Client)
				},
			)
		})
	}
}
