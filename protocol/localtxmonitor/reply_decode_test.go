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

package localtxmonitor

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestReplyNextTxRejectsScalarWrapperBeforeAllocation(t *testing.T) {
	wire := append([]byte{0x82, 6, 0x9a, 0, 0x10, 0, 0},
		bytes.Repeat([]byte{0}, 1<<20)...)
	var raw cbor.RawMessage
	_, err := cbor.Decode(wire, &raw)
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeReplyNextTx, wire)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	require.Nil(t, msg)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d allocated=%d", len(wire), allocated)
	require.LessOrEqual(t, allocated, uint64(64<<10))
}

func TestReplyNextTxLargeValidAllocationBudget(t *testing.T) {
	tx := bytes.Repeat([]byte{0}, 2<<20)
	wire, err := cbor.Encode(NewMsgReplyNextTx(6, tx))
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeReplyNextTx, wire)
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.Equal(t, tx, msg.(*MsgReplyNextTx).Transaction.Tx)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d allocated=%d", len(wire), allocated)
	require.LessOrEqual(t, allocated, uint64(12*len(wire)+64<<10))
}

func TestReplyNextTxArrayAndBytesForms(t *testing.T) {
	for _, wire := range [][]byte{
		{0x81, 6},
		{0x9f, 6, 0xff},
		{0x82, 6, 0x82, 0, 0xd8, 24, 0x40},
		{0x9f, 6, 0x9f, 0x18, 6, 0xd8, 24, 0x5f,
			0x42, 1, 2, 0x41, 3, 0xff, 0xff, 0xff},
	} {
		msg, err := NewMsgFromCbor(MessageTypeReplyNextTx, wire)
		require.NoError(t, err, "%x", wire)
		require.Equal(t, wire, msg.Cbor())
	}
	for _, wire := range [][]byte{
		{0x83, 6, 0x82, 0, 0xd8, 24, 0x40, 0},
		{0x82, 6, 0x83, 0, 0xd8, 24, 0x40, 0},
		{0x82, 6, 0x82, 0, 0x40},
		{0x82, 6, 0x82, 0x19, 1, 0, 0xd8, 24, 0x40},
		{0x82, 6, 0x82, 0, 0xd8, 24, 0x5f, 0x5f, 0xff, 0xff},
	} {
		_, err := NewMsgFromCbor(MessageTypeReplyNextTx, wire)
		require.Error(t, err, "%x", wire)
	}
}
