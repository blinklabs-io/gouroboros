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

package common_test

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/stretchr/testify/require"
)

func TestPointMessageDecodersPreserveValidFormsAndRejectMalformed(t *testing.T) {
	t.Parallel()
	decoders := []struct {
		name   string
		kind   uint
		decode func(uint, []byte) (protocol.Message, error)
		body   func(any) []any
	}{
		{"chainsync_ntn", chainsync.MessageTypeFindIntersect,
			func(kind uint, data []byte) (protocol.Message, error) {
				return chainsync.NewMsgFromCbor(protocol.ProtocolModeNodeToNode, kind, data)
			},
			func(p any) []any { return []any{uint64(4), []any{p}} }},
		{"chainsync_ntc", chainsync.MessageTypeFindIntersect,
			func(kind uint, data []byte) (protocol.Message, error) {
				return chainsync.NewMsgFromCbor(protocol.ProtocolModeNodeToClient, kind, data)
			},
			func(p any) []any { return []any{uint64(4), []any{p}} }},
		{"blockfetch", blockfetch.MessageTypeRequestRange,
			blockfetch.NewMsgFromCbor,
			func(p any) []any { return []any{uint64(0), p, p} }},
		{"localstatequery", localstatequery.MessageTypeAcquire,
			localstatequery.NewMsgFromCbor,
			func(p any) []any { return []any{uint64(0), p} }},
	}
	for _, decoder := range decoders {
		t.Run(decoder.name, func(t *testing.T) {
			t.Parallel()
			for _, point := range []any{
				[]any{},
				[]any{uint64(1), bytes.Repeat([]byte{0xab}, 32)},
			} {
				data, err := cbor.Encode(decoder.body(point))
				require.NoError(t, err)
				msg, err := decoder.decode(decoder.kind, data)
				require.NoError(t, err)
				require.NotNil(t, msg)
			}
			for _, tc := range []struct {
				name  string
				point any
			}{
				{"null", nil},
				{"one element", []any{uint64(1)}},
				{"short hash", []any{uint64(1), []byte{0xab}}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					data, err := cbor.Encode(decoder.body(tc.point))
					require.NoError(t, err)
					_, err = decoder.decode(decoder.kind, data)
					require.Error(t, err)
				})
			}
		})
	}
}
