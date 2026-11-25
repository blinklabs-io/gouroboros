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
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
)

// sentEraQuery runs a block query against a version 23 node in era and
// returns the query the client sent.
func sentEraQuery(
	t *testing.T,
	era byte,
	call func(*localstatequery.Client) error,
) []byte {
	t.Helper()
	conversation := conversationEraQueryAt(23, era, []byte{0x81, 0x80})
	var mu sync.Mutex
	var sent []byte
	// The capture hands the decoded message back as the expected one, so that
	// the mock accepts it and the bytes recorded are the client's own.
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
	runTest(t, conversation, func(t *testing.T, oConn *ouroboros.Connection) {
		// The reply is a placeholder; only the request is under test.
		_ = call(oConn.LocalStateQuery().Client)
	})
	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, sent, "the client sent no query")
	var msg struct {
		cbor.StructAsArray
		Type  uint
		Query cbor.RawMessage
	}
	_, err := cbor.Decode(sent, &msg)
	require.NoError(t, err)
	return msg.Query
}

// TestEraFilterSetEncoding pins the filters that the reference writes with
// the era's own encoding (LC.toEraCBOR @era in encodeShelleyQuery,
// ouroboros-consensus Shelley/Ledger/Query.hs). Before Conway the era's
// protocol version is below 9, where a Set is a plain array and the node's
// decoder (decodeSet in cardano-ledger-binary Decoder.hs) refuses tag 258.
// From Conway the set carries tag 258. The expected bytes are the
// ouroboros-consensus ec239d57f618 goldens
// QueryVersion3/CardanoNodeToClientVersion19/Query_<Era>_<Query> with the
// top-level BlockQuery wrapper [0, q] added; their arguments are empty sets.
func TestEraFilterSetEncoding(t *testing.T) {
	queries := []struct {
		name string
		tag  string
		call func(*localstatequery.Client) error
	}{
		{
			name: "GetUTxOByAddress",
			tag:  "06",
			call: func(c *localstatequery.Client) error {
				_, err := c.GetUTxOByAddress([]ledger.Address{})
				return err
			},
		},
		{
			name: "GetUTxOByAddress nil",
			tag:  "06",
			call: func(c *localstatequery.Client) error {
				_, err := c.GetUTxOByAddress(nil)
				return err
			},
		},
		{
			name: "GetFilteredDelegationsAndRewardAccounts",
			tag:  "0a",
			call: func(c *localstatequery.Client) error {
				_, err := c.GetFilteredDelegationsAndRewardAccounts(nil)
				return err
			},
		},
		{
			name: "GetUTxOByTxIn",
			tag:  "0f",
			call: func(c *localstatequery.Client) error {
				_, err := c.GetUTxOByTxIn([]ledger.TransactionInput{})
				return err
			},
		},
		{
			name: "GetUTxOByTxIn nil",
			tag:  "0f",
			call: func(c *localstatequery.Client) error {
				_, err := c.GetUTxOByTxIn(nil)
				return err
			},
		},
	}
	eras := []string{"", "Shelley", "Allegra", "Mary", "Alonzo", "Babbage", "Conway", "Dijkstra"}
	for _, q := range queries {
		for era := 1; era < len(eras); era++ {
			t.Run(q.name+"/"+eras[era], func(t *testing.T) {
				set := "80"
				if era >= 6 {
					set = "d9010280"
				}
				want := fmt.Sprintf("820082008%d82%s%s", 200+era, q.tag, set)
				got := sentEraQuery(t, byte(era), q.call)
				require.Equal(t, want, hex.EncodeToString(got))
			})
		}
	}
}
