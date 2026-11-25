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
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/require"
)

// TestDecodeGetFuturePParamsQuery decodes GetFuturePParams (Shelley block
// query 33) as the server does. The message carries the ouroboros-consensus
// ec239d57f618 goldens
// QueryVersion3/CardanoNodeToClientVersion19/Query_Conway_GetFuturePParams
// and Query_Dijkstra_GetFuturePParams inside the top-level BlockQuery
// wrapper; the reference decodes the query from Conway
// (decodeShelleyQuery (1, 33) in Shelley/Ledger/Query.hs).
func TestDecodeGetFuturePParamsQuery(t *testing.T) {
	for _, tc := range []struct {
		name  string
		era   uint
		query string
	}{
		{name: "Conway", era: 6, query: "82008206811821"},
		{name: "Dijkstra", era: 7, query: "82008207811821"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// MsgQuery: [3, [0, golden]]
			msgBytes, err := hex.DecodeString("82038200" + tc.query)
			require.NoError(t, err)
			msg, err := localstatequery.NewMsgFromCbor(
				localstatequery.MessageTypeQuery,
				msgBytes,
			)
			require.NoError(t, err)
			query := msg.(*localstatequery.MsgQuery).Query
			block, ok := query.Query.(*localstatequery.BlockQuery)
			require.True(t, ok, "%T", query.Query)
			shelley, ok := block.Query.(*localstatequery.ShelleyQuery)
			require.True(t, ok, "%T", block.Query)
			require.Equal(t, tc.era, shelley.Era)
			require.IsType(
				t,
				&localstatequery.ShelleyFuturePParamsQuery{},
				shelley.Query,
			)
			reencoded, err := cbor.Encode(&query)
			require.NoError(t, err)
			require.Equal(t, "8200"+tc.query, hex.EncodeToString(reencoded))
		})
	}
}
