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

package leiosfetch

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/require"
)

func TestVotesRequestRejectsScalarListBeforeAllocation(t *testing.T) {
	wire := append([]byte{0x82, 4, 0x9a, 0, 0x10, 0, 0},
		bytes.Repeat([]byte{0}, 1<<20)...)
	// This is a complete CBOR message, so syntax validation alone accepts it.
	var raw cbor.RawMessage
	_, err := cbor.Decode(wire, &raw)
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	require.Nil(t, msg)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d allocated=%d", len(wire), allocated)
	require.LessOrEqual(t, allocated, uint64(64<<10))
}

func TestVotesRequestLargeValidAllocationBudget(t *testing.T) {
	const count = 131073
	ids := make([]MsgVotesRequestVoteId, count)
	for idx := range ids {
		ids[idx].SlotNo = uint64(idx)
		ids[idx].VoterId = 1
	}
	wire, err := cbor.Encode(NewMsgVotesRequest(ids))
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.Equal(t, ids, msg.(*MsgVotesRequest).VoteIds)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d ids=%d allocated=%d", len(wire), count, allocated)
	// Two uint64 fields consume 16 bytes per ID; allow decoder overhead too.
	require.LessOrEqual(t, allocated, uint64(64*count+64<<10))
}

func TestVotesRequestRejectsLateInvalidIDBeforeAllocation(t *testing.T) {
	// The declared count fits the bytes and every preceding ID has valid shape.
	wire := append([]byte{0x82, 4, 0x9a, 0, 2, 0, 1},
		bytes.Repeat([]byte{0x82, 0, 0}, 131072)...)
	wire = append(wire, 0x83, 0, 0, 0)
	var raw cbor.RawMessage
	_, err := cbor.Decode(wire, &raw)
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	require.Nil(t, msg)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d allocated=%d", len(wire), allocated)
	require.LessOrEqual(t, allocated, uint64(64<<10))
}

func TestVotesRequestRejectsWrongMessageTypeBeforeAllocation(t *testing.T) {
	wire := append([]byte{0x82, 5, 0x9a, 0, 2, 0, 1},
		bytes.Repeat([]byte{0x82, 0, 0}, 131073)...)
	var raw cbor.RawMessage
	_, err := cbor.Decode(wire, &raw)
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	require.Nil(t, msg)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d allocated=%d", len(wire), allocated)
	require.LessOrEqual(t, allocated, uint64(64<<10))
	require.ErrorContains(t, err, "vote request message type must be 4")
}

func TestVotesRequestRejectsNullIDBeforeAllocation(t *testing.T) {
	wire := append([]byte{0x82, 4, 0x9a, 0, 2, 0, 1},
		bytes.Repeat([]byte{0x82, 0xf6, 0xf7}, 131073)...)
	var raw cbor.RawMessage
	_, err := cbor.Decode(wire, &raw)
	require.NoError(t, err)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
	runtime.ReadMemStats(&after)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("wire=%d allocated=%d", len(wire), allocated)
	require.LessOrEqual(t, allocated, uint64(64<<10))
	require.Error(t, err)
	require.Nil(t, msg)
	require.ErrorContains(t, err, "CBOR field is not unsigned")
}

func TestVotesRequestPreservesTypedScalarAndArrayForms(t *testing.T) {
	for _, wire := range [][]byte{
		{0x82, 4, 0xf6},
		{0x82, 4, 0x80},
		{0x9f, 4, 0x9f, 0x9f, 0x18, 1, 0x19, 0, 2, 0xff, 0xff, 0xff},
		{0x82, 4, 0x81, 0x82, 0xc2, 0x41, 1, 2},
		{0x82, 4, 0x81, 0x82, 0xd8, 100, 1, 2},
	} {
		msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
		require.NoError(t, err, "%x", wire)
		require.Equal(t, wire, msg.Cbor())
	}
	for _, wire := range [][]byte{
		{0x82, 4, 0x81, 0x82, 1, 0x20},
		{0x82, 4, 0x82, 0x82, 1, 2, 0},
		{0x82, 4, 0x81, 0x83, 1, 2, 3},
		{0x9f, 4, 0x9f, 0x9f, 1, 2, 3, 0xff, 0xff, 0xff},
	} {
		_, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
		require.Error(t, err, "%x", wire)
	}
}

func TestVotesRequestPreservesTaggedArrays(t *testing.T) {
	testCases := map[string][]byte{
		"outer message": {0xd8, 100, 0x82, 4, 0x80},
		"vote ID list":  {0x82, 4, 0xd8, 100, 0x80},
		"vote ID":       {0x82, 4, 0x81, 0xd8, 100, 0x82, 1, 2},
	}
	for name, wire := range testCases {
		t.Run(name, func(t *testing.T) {
			msg, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
			require.NoError(t, err)
			require.Equal(t, wire, msg.Cbor())
		})
	}
}

func TestVotesRequestRejectsDeepIDBeforeTypedDecode(t *testing.T) {
	id := append(bytes.Repeat([]byte{0x81}, 64), 0)
	wire := append([]byte{0x82, MessageTypeVotesRequest, 0x81}, id...)

	_, err := NewMsgFromCbor(MessageTypeVotesRequest, wire)
	require.ErrorContains(t, err, "vote request: CBOR nesting exceeds maximum depth 4")
}
