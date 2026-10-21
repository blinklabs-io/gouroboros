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
	"sort"
	"strings"
	"sync"
	"testing"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stakePoolStateRequestAt runs GetStakePoolState against a node at version 23
// and returns the bytes of the query the client sent.
func stakePoolStateRequestAt(
	t *testing.T,
	poolIds []ledger.PoolId,
) []byte {
	t.Helper()
	const conwayEra = 6
	conversation := conversationEraQueryAt(23, conwayEra, []byte{0x81, 0xa0})
	var mu sync.Mutex
	var sent []byte
	// The request is the third message of the conversation: the acquire, the
	// era query, then this one. The capture hands the decoded message back as
	// the expected one, so that the mock accepts it and the bytes are ours.
	expected := localstatequery.NewMsgQuery(nil)
	conversation[6] = ouroboros_mock.ConversationEntryInput{
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
	}
	runTest(
		t,
		conversation,
		func(t *testing.T, oConn *ouroboros.Connection) {
			_, err := oConn.LocalStateQuery().Client.GetStakePoolState(
				poolIds,
				true,
			)
			require.NoError(t, err)
		},
	)
	mu.Lock()
	defer mu.Unlock()
	return sent
}

// stakePoolStateRequestWire is the MsgQuery bytes for GetStakePoolState in
// Conway with the pool filter written out by hand.
func stakePoolStateRequestWire(t *testing.T, filter []byte) string {
	t.Helper()
	wire, err := cbor.Encode([]any{
		localstatequery.MessageTypeQuery,
		[]any{
			localstatequery.QueryTypeBlock,
			[]any{
				localstatequery.QueryTypeShelley,
				[]any{
					6,
					[]any{
						localstatequery.QueryTypeShelleyStakePoolState,
						[]any{cbor.RawMessage(filter)},
						true,
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return hex.EncodeToString(wire)
}

// TestStakePoolStateClientPoolSetIsCanonical pins the pool set the client
// sends. The node decodes it with decodeSetSkel, which refuses a set that is
// not a tag-258 definite-length array in strictly ascending order of the
// encoded members, and a Set cannot hold a pool twice.
func TestStakePoolStateClientPoolSetIsCanonical(t *testing.T) {
	member := func(b int) string {
		return "581c" + strings.Repeat(fmt.Sprintf("%02x", b), 28)
	}
	poolOf := func(b int) ledger.PoolId {
		raw := make([]byte, 28)
		for i := range raw {
			raw[i] = byte(b)
		}
		return ledger.PoolId(ledger.NewBlake2b224(raw))
	}
	// setOf is the expected filter: tag 258, a definite array header for n
	// members, then the members in ascending order.
	setOf := func(header string, members ...int) []byte {
		sort.Ints(members)
		out := "d90102" + header
		for _, b := range members {
			out += member(b)
		}
		raw, err := hex.DecodeString(out)
		require.NoError(t, err)
		return raw
	}
	many := make([]int, 0, 30)
	for i := 1; i <= 30; i++ {
		many = append(many, i)
	}
	descending := func(members []int) []ledger.PoolId {
		pools := make([]ledger.PoolId, 0, len(members))
		for i := len(members) - 1; i >= 0; i-- {
			pools = append(pools, poolOf(members[i]))
		}
		return pools
	}
	cases := []struct {
		name  string
		pools []ledger.PoolId
		want  []byte
	}{
		{
			name:  "descending_input_is_sorted_ascending",
			pools: []ledger.PoolId{poolOf(3), poolOf(2), poolOf(1)},
			want:  setOf("83", 1, 2, 3),
		},
		{
			name:  "more_than_23_pools_keep_a_definite_length",
			pools: descending(many),
			want:  setOf("981e", many...),
		},
		{
			name:  "duplicates_are_dropped",
			pools: []ledger.PoolId{poolOf(2), poolOf(1), poolOf(2), poolOf(1)},
			want:  setOf("82", 1, 2),
		},
		{
			name:  "single_pool",
			pools: []ledger.PoolId{poolOf(9)},
			want:  setOf("81", 9),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sent := stakePoolStateRequestAt(t, tc.pools)
			assert.Equal(
				t,
				stakePoolStateRequestWire(t, tc.want),
				hex.EncodeToString(sent),
			)
		})
	}
}
