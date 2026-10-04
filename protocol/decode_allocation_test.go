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

package protocol_test

import (
	"encoding/binary"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosvotes"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/perasvotes"
	"github.com/stretchr/testify/require"
)

func TestLargeLocalUTxOWholeResponse(t *testing.T) {
	const count = 131073
	data := []byte{0x81, 0xba, 0, 2, 0, 1}
	for i := range uint32(count) {
		data = append(data, 0x82, 0x58, 0x20)
		hash := make([]byte, 32)
		binary.BigEndian.PutUint32(hash[len(hash)-4:], i)
		data = append(data, hash...)
		data = append(data, 0, 0x82, 0x58, 0x1d, 0x60)
		data = append(data, make([]byte, 28)...)
		data = append(data, 1)
	}
	wire, err := cbor.Encode(localstatequery.NewMsgResult(data))
	require.NoError(t, err)
	message, err := localstatequery.NewMsgFromCbor(
		localstatequery.MessageTypeResult,
		wire,
	)
	require.NoError(t, err)
	result := message.(*localstatequery.MsgResult)
	require.Equal(t, data, []byte(result.Result))
	var utxos localstatequery.UTxOWholeResult
	consumed, err := cbor.Decode(result.Result, &utxos)
	require.NoError(t, err)
	require.Equal(t, len(data), consumed)
	require.Len(t, utxos.Results, count)
	for _, output := range utxos.Results {
		require.Equal(t, uint64(1), output.OutputAmount.Amount)
		break
	}
	t.Logf("%d valid UTxOs consume %d encoded response bytes", count, len(wire))
}

func TestMalformedMessageCollectionsDoNotAllocateDeclaredCounts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		kind   uint
		decode func(uint, []byte) (protocol.Message, error)
	}{
		{"local result", localstatequery.MessageTypeResult, localstatequery.NewMsgFromCbor},
		{"local transaction", localtxmonitor.MessageTypeReplyNextTx, localtxmonitor.NewMsgFromCbor},
		{"local rejection", localtxsubmission.MessageTypeRejectTx, localtxsubmission.NewMsgFromCbor},
		{"leios transactions", leiosfetch.MessageTypeBlockTxs, leiosfetch.NewMsgFromCbor},
		{"leios votes", leiosfetch.MessageTypeVotes, leiosfetch.NewMsgFromCbor},
		{"leios vote", leiosvotes.MessageTypeVote, leiosvotes.NewMsgFromCbor},
		{"peras objects", uint(perasvotes.MessageTypeReplyObjects), perasvotes.NewMsgFromCbor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := []byte{0x82, byte(tc.kind), 0x9a, 0, 2, 0, 0}
			result := testing.Benchmark(func(b *testing.B) {
				for range b.N {
					_, err := tc.decode(tc.kind, wire)
					if err == nil {
						b.Fatal("truncated collection accepted")
					}
				}
			})
			t.Logf(
				"%d encoded bytes: %d allocated bytes/op",
				len(wire),
				result.AllocedBytesPerOp(),
			)
			require.Less(t, result.AllocedBytesPerOp(), int64(32*1024))
		})
	}
}
