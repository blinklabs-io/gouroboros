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
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// TestFilteredPoolQueryClientWire pins the MsgQuery the client sends for the
// filtered pool queries: the pool filter is a StrictMaybe (Set PoolId), so
// "all pools" is an empty list and a filter is a one-element list holding a
// tag-258 set.
func TestFilteredPoolQueryClientWire(t *testing.T) {
	pool := ledger.PoolId(ledger.NewBlake2b224(make([]byte, 28)))
	const conwayEra = 6
	cases := []struct {
		name   string
		typeID int
		call   func(*localstatequery.Client, []any) error
	}{
		{
			name:   "GetPoolState",
			typeID: localstatequery.QueryTypeShelleyPoolState,
			call: func(c *localstatequery.Client, ids []any) error {
				_, err := c.GetPoolState(ids)
				return err
			},
		},
		{
			name:   "GetPoolDistr",
			typeID: localstatequery.QueryTypeShelleyPoolDistr,
			call: func(c *localstatequery.Client, ids []any) error {
				_, err := c.GetPoolDistr(ids)
				return err
			},
		},
	}
	for _, tc := range cases {
		for _, filter := range []struct {
			name string
			ids  []any
			wire []any
		}{
			{name: "all pools", wire: []any{}},
			{
				name: "one pool",
				ids:  []any{pool},
				wire: []any{cbor.NewSetType([]any{pool}, true)},
			},
		} {
			t.Run(tc.name+"/"+filter.name, func(t *testing.T) {
				wire, err := cbor.Encode([]any{
					localstatequery.MessageTypeQuery,
					[]any{
						localstatequery.QueryTypeBlock,
						[]any{
							localstatequery.QueryTypeShelley,
							[]any{
								conwayEra,
								[]any{tc.typeID, filter.wire},
							},
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

				conversation := append(
					append([]ouroboros_mock.ConversationEntry{},
						conversationConwayEra...),
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
						// The reply is not a valid result; only the
						// request bytes are under test.
						_ = tc.call(oConn.LocalStateQuery().Client, filter.ids)
					},
				)
			})
		}
	}
}
