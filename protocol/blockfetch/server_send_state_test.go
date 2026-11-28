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

package blockfetch_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// The server's send methods queue whatever they are given, but nothing
// reaches the wire in a state that does not allow it: a block before
// MsgStartBatch, or one left over after MsgBatchDone, stops the server with
// a protocol error instead, and only the legal sequence is written.
func TestServerWritesBlockOnlyInStreaming(t *testing.T) {
	for _, test := range []struct {
		name string
		// answer is the server's RequestRangeFunc for the first range.
		answer func(blockfetch.CallbackContext) error
		// written are the messages the peer must receive, in order.
		written []uint8
		// secondRange sends another MsgRequestRange after written.
		secondRange bool
		refused     bool
	}{
		{
			name: "start, block, done",
			answer: func(ctx blockfetch.CallbackContext) error {
				return errors.Join(
					ctx.Server.StartBatch(),
					ctx.Server.Block(6, []byte{0x80}),
					ctx.Server.BatchDone(),
				)
			},
			written: []uint8{
				blockfetch.MessageTypeStartBatch,
				blockfetch.MessageTypeBlock,
				blockfetch.MessageTypeBatchDone,
			},
		},
		{
			name: "block in Busy",
			answer: func(ctx blockfetch.CallbackContext) error {
				return ctx.Server.Block(6, []byte{0x80})
			},
			refused: true,
		},
		{
			name: "block left over in Idle",
			answer: func(ctx blockfetch.CallbackContext) error {
				return errors.Join(
					ctx.Server.StartBatch(),
					ctx.Server.Block(6, []byte{0x80}),
					ctx.Server.BatchDone(),
					ctx.Server.Block(6, []byte{0x80}),
				)
			},
			written: []uint8{
				blockfetch.MessageTypeStartBatch,
				blockfetch.MessageTypeBlock,
				blockfetch.MessageTypeBatchDone,
			},
			secondRange: true,
			refused:     true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			h, opts := newModelHarness(t, protocol.ProtocolRoleServer)
			first := true
			cfg, err := blockfetch.NewConfig(
				blockfetch.WithBatchStartTimeout(time.Minute),
				blockfetch.WithRequestRangeFunc(func(
					ctx blockfetch.CallbackContext, _, _ pcommon.Point,
				) error {
					if !first {
						return nil
					}
					first = false
					return test.answer(ctx)
				}),
			)
			require.NoError(t, err)
			server := blockfetch.NewServer(opts, &cfg)
			server.Start()
			t.Cleanup(func() { _ = server.Stop(context.Background()) })
			range1 := kindMessage(t, kindRequestRange, blockfetch.StateIdle)
			h.sendRaw(t, range1)
			for _, want := range test.written {
				h.requireReceive(t, want)
			}
			if test.secondRange {
				// The leftover block waits without agency in Idle.
				h.requireQuiet(t)
				h.sendRaw(t, range1)
			}
			if test.refused {
				h.requireViolation(t, "not allowed in current protocol state")
			}
			h.requireQuiet(t)
		})
	}
}
