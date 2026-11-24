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

package chainsync

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

// rollForwardTip is the tip used by the roll-forward tests and its wire
// encoding, written out byte by byte: [[slot, hash], blockNumber].
var (
	rollForwardTip = Tip{
		Point:       pcommon.Point{Slot: 0x1234, Hash: bytes.Repeat([]byte{0xab}, 32)},
		BlockNumber: 7,
	}
	rollForwardTipCbor = append(
		append([]byte{0x82, 0x82, 0x19, 0x12, 0x34, 0x58, 0x20}, bytes.Repeat([]byte{0xab}, 32)...),
		0x07,
	)
)

// cborHead returns a CBOR initial byte and argument for the given major type.
func cborHead(major byte, n int) []byte {
	m := major << 5
	switch {
	case n < 24:
		return []byte{m | byte(n)}
	case n < 1<<8:
		return []byte{m | 24, byte(n)}
	case n < 1<<16:
		return []byte{m | 25, byte(n >> 8), byte(n)}
	default:
		return []byte{m | 26, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
}

// tag24Bytes is #6.24(bytes .cbor payload): d8 18 followed by a byte string.
func tag24Bytes(payload []byte) []byte {
	out := []byte{0xd8, 0x18}
	out = append(out, cborHead(2, len(payload))...)
	return append(out, payload...)
}

func concatBytes(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

// blockHeaderBytes returns the raw first element of a block array.
func blockHeaderBytes(t *testing.T, block []byte) []byte {
	t.Helper()
	var parts []cbor.RawMessage
	_, err := cbor.Decode(block, &parts)
	require.NoError(t, err)
	require.NotEmpty(t, parts)
	return parts[0]
}

// serveRollForward starts a chainsync server in the given mode, has the
// client send RequestNext, and returns the raw bytes of the message the
// server produced from Server.RollForward, plus the RollForward error.
func serveRollForward(
	t *testing.T,
	mode protocol.ProtocolMode,
	blockType uint,
	block []byte,
	tip Tip,
) (raw []byte, msg protocol.Message, rollForwardErr error) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	clientMuxer := muxer.New(clientConn)
	serverMuxer := muxer.New(serverConn)
	t.Cleanup(clientMuxer.Stop)
	t.Cleanup(serverMuxer.Stop)

	protoId := ProtocolIdNtC
	fromCbor := NewMsgFromCborNtC
	stateMap := StateMapNtC
	if mode == protocol.ProtocolModeNodeToNode {
		protoId = ProtocolIdNtN
		fromCbor = NewMsgFromCborNtN
		stateMap = StateMapNtN
	}

	serverErrors := make(chan error, 4)
	rollForwardDone := make(chan error, 1)
	server := NewServer(
		protocol.ProtocolOptions{
			Muxer:     serverMuxer,
			ErrorChan: serverErrors,
			Mode:      mode,
		},
		&Config{
			RequestNextFunc: func(ctx CallbackContext) error {
				rollForwardDone <- ctx.Server.RollForward(
					blockType,
					block,
					tip,
				)
				return nil
			},
		},
	)

	clientErrors := make(chan error, 4)
	received := make(chan protocol.Message, 1)
	client := protocol.New(protocol.ProtocolConfig{
		Name:       "chainsync-rollforward-client",
		ProtocolId: protoId,
		ErrorChan:  clientErrors,
		Muxer:      clientMuxer,
		Mode:       mode,
		Role:       protocol.ProtocolRoleClient,
		MessageHandlerFunc: func(m protocol.Message) error {
			received <- m
			return nil
		},
		MessageFromCborFunc: fromCbor,
		StateMap:            stateMap,
		InitialState:        stateIdle,
	})

	server.Start()
	t.Cleanup(func() { _ = server.Stop(context.Background()) })
	client.Start()
	t.Cleanup(func() { _ = client.Stop(context.Background()) })
	serverMuxer.Start()
	clientMuxer.Start()

	require.NoError(t, client.SendMessage(NewMsgRequestNext()))

	timeout := time.After(10 * time.Second)
	select {
	case rollForwardErr = <-rollForwardDone:
	case err := <-serverErrors:
		t.Fatalf("server error: %s", err)
	case <-timeout:
		t.Fatal("timed out waiting for RollForward")
	}
	if rollForwardErr != nil {
		return nil, nil, rollForwardErr
	}
	select {
	case msg = <-received:
	case err := <-clientErrors:
		t.Fatalf("client error: %s", err)
	case err := <-serverErrors:
		t.Fatalf("server error: %s", err)
	case <-timeout:
		t.Fatal("timed out waiting for RollForward at client")
	}
	return msg.Cbor(), msg, nil
}

// byronNtnExpected lays out the message as ouroboros-consensus encodes a
// Byron SerialisedHeader for node-to-node version 2 (encodeDisk of the
// hard-fork NS): [2, [0, [[ctx, size], #6.24(header)]], tip], where ctx is 0
// for a boundary header and 1 for a regular header, size is the length of the
// [tag, block] disk encoding of the block (the block plus its two-byte
// envelope), and header is the annotated header bytes.
func byronNtnExpected(t *testing.T, ctx byte, block []byte) []byte {
	t.Helper()
	header := blockHeaderBytes(t, block)
	return concatBytes(
		[]byte{0x83, 0x02},
		[]byte{0x82, 0x00},
		[]byte{0x82},
		[]byte{0x82, ctx},
		cborHead(0, len(block)+2),
		tag24Bytes(header),
		rollForwardTipCbor,
	)
}

func TestServerRollForwardNtNByron(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		blockType uint
		file      string
		ctx       byte
	}{
		{
			name:      "ebb",
			blockType: ledger.BlockTypeByronEbb,
			file:      "testdata/byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
			ctx:       0,
		},
		{
			name:      "main",
			blockType: ledger.BlockTypeByronMain,
			file:      "testdata/byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
			ctx:       1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			block := hexDecode(string(readFile(tc.file)))
			raw, msg, err := serveRollForward(
				t,
				protocol.ProtocolModeNodeToNode,
				tc.blockType,
				block,
				rollForwardTip,
			)
			require.NoError(t, err)
			require.Equal(t, byronNtnExpected(t, tc.ctx, block), raw)

			// The NtN client decoder reads the same bytes back.
			rf, ok := msg.(*MsgRollForwardNtN)
			require.True(t, ok, "unexpected message type %T", msg)
			require.Equal(t, uint(ledger.BlockHeaderTypeByron), rf.WrappedHeader.Era)
			require.Equal(t, tc.blockType, rf.WrappedHeader.ByronType())
			require.Equal(
				t,
				[]byte(blockHeaderBytes(t, block)),
				rf.WrappedHeader.HeaderCbor(),
			)
			require.Equal(t, rollForwardTip, rf.Tip)
		})
	}
}

func TestServerRollForwardNtNShelleyUnchanged(t *testing.T) {
	t.Parallel()
	block := hexDecode(string(readFile(
		"testdata/shelley_block_testnet_02b1c561715da9e540411123a6135ee319b02f60b9a11a603d3305556c04329f.hex",
	)))
	raw, _, err := serveRollForward(
		t,
		protocol.ProtocolModeNodeToNode,
		ledger.BlockTypeShelley,
		block,
		rollForwardTip,
	)
	require.NoError(t, err)
	// [2, [1, #6.24(header)], tip]
	require.Equal(
		t,
		concatBytes(
			[]byte{0x83, 0x02, 0x82, 0x01},
			tag24Bytes(blockHeaderBytes(t, block)),
			rollForwardTipCbor,
		),
		raw,
	)
}

func TestServerRollForwardNtNUnknownBlockType(t *testing.T) {
	t.Parallel()
	block := hexDecode(string(readFile(
		"testdata/byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
	)))
	_, _, err := serveRollForward(
		t,
		protocol.ProtocolModeNodeToNode,
		99,
		block,
		rollForwardTip,
	)
	require.ErrorContains(t, err, "unknown block type: 99")
}

func TestServerRollForwardNtCByronNotWrapped(t *testing.T) {
	t.Parallel()
	block := hexDecode(string(readFile(
		"testdata/byron_main_block_testnet_f38aa5e8cf0b47d1ffa8b2385aa2d43882282db2ffd5ac0e3dadec1a6f2ecf08.hex",
	)))
	raw, _, err := serveRollForward(
		t,
		protocol.ProtocolModeNodeToClient,
		ledger.BlockTypeByronMain,
		block,
		rollForwardTip,
	)
	require.NoError(t, err)
	// [2, #6.24([1, block]), tip]: the NtC block is wrapped as a whole and
	// carries no era or Byron header metadata.
	require.Equal(
		t,
		concatBytes(
			[]byte{0x83, 0x02},
			tag24Bytes(concatBytes([]byte{0x82, 0x01}, block)),
			rollForwardTipCbor,
		),
		raw,
	)
}

// TestServerRollForwardNtNByronEbbCaptured compares the server output for a
// Byron EBB against the stored NtN RollForward fixture for the same block.
func TestServerRollForwardNtNByronEbbCaptured(t *testing.T) {
	t.Parallel()
	block := hexDecode(string(readFile(
		"testdata/byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)))
	captured := hexDecode(string(readFile(
		"testdata/rollforward_ntn_byron_ebb_testnet_8f8602837f7c6f8b8867dd1cbc1842cf51a27eaed2c70ef48325d00f8efb320f.hex",
	)))
	raw, _, err := serveRollForward(
		t,
		protocol.ProtocolModeNodeToNode,
		ledger.BlockTypeByronEbb,
		block,
		Tip{
			Point: pcommon.Point{
				Slot: 55740899,
				Hash: hexDecode(
					"C89E652408EC269379751C8B2BF0137297BF9F5D0FB2E76E19ACF63D783C3A66",
				),
			},
			BlockNumber: 3479284,
		},
	)
	require.NoError(t, err)
	require.Equal(t, captured, raw)
}
