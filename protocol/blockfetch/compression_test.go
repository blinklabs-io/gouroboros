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

package blockfetch

import (
	"bytes"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
)

func TestCompressedBlocksWireAndRoundTrip(t *testing.T) {
	t.Parallel()
	// Two concatenated wrapped blocks, with no enclosing array.
	raw := []byte{0x82, 0x06, 0x82, 0x01, 0x02, 0x82, 0x06, 0x82, 0x03, 0x04}
	msg := &MsgCompressedBlocks{Encoding: CompressionRaw, EncodedData: raw}
	msg.MessageType = MessageTypeCompressedBlocks
	wire, err := cbor.Encode(msg)
	require.NoError(t, err)
	require.Equal(t, append([]byte{0x83, 0x06, 0x00, 0x4a}, raw...), wire)
	decoded, err := NewMsgFromCbor(MessageTypeCompressedBlocks, wire)
	require.NoError(t, err)
	expanded, err := decoded.(*MsgCompressedBlocks).decompressed(1024)
	require.NoError(t, err)
	require.Equal(t, raw, expanded)
	compressed, err := compressBlocks(raw)
	require.NoError(t, err)
	expanded, err = compressed.decompressed(1024)
	require.NoError(t, err)
	require.Equal(t, raw, expanded)
}

func TestCompressedBlocksRejectInvalidAndOversizeData(t *testing.T) {
	t.Parallel()
	enc, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	require.NoError(t, err)
	defer enc.Close()
	bomb := enc.EncodeAll(bytes.Repeat([]byte{0}, 1024*1024), nil)
	for _, tc := range []struct {
		name     string
		encoding uint64
		payload  []byte
		limit    uint64
	}{
		{"unknown", 2, []byte{1}, 1024},
		{"empty", 0, nil, 1024},
		{"invalid zstd", 1, []byte{1, 2, 3}, 1024},
		{"expanded limit", 1, bomb, 4096},
		{"raw limit", 0, []byte{1, 2}, 1},
		{"truncated", 1, bomb[:len(bomb)-1], 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := (&MsgCompressedBlocks{Encoding: tc.encoding, EncodedData: tc.payload}).decompressed(
				tc.limit,
			)
			require.Error(t, err)
		})
	}
}

func TestCompressedBlocksDecodedBudget(t *testing.T) {
	t.Parallel()
	raw := []byte{0x82, 0x06, 0x80}
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderConcurrency(1), zstd.WithEncoderPadding(1024))
	require.NoError(t, err)
	defer encoder.Close()
	encoded := encoder.EncodeAll(raw, nil)
	require.Greater(t, len(encoded), len(raw)+1)
	for _, encoding := range []uint64{CompressionRaw, CompressionZstandard} {
		payload := raw
		if encoding == CompressionZstandard {
			payload = encoded
		}
		msg := &MsgCompressedBlocks{Encoding: encoding, EncodedData: payload}
		for _, limit := range []uint64{0, 2, 3, 4} {
			expanded, err := msg.decompressed(limit)
			if limit < uint64(len(raw)) {
				require.ErrorIs(t, err, ErrRangeByteLimit)
			} else {
				require.NoError(t, err)
				require.Equal(t, raw, expanded)
			}
		}
	}
}

func TestCompressionTransitionsAreOptInAndInstanceOwned(t *testing.T) {
	t.Parallel()
	for _, enabled := range []bool{false, true} {
		cfg, err := NewConfig(WithCompressionEnabled(enabled))
		require.NoError(t, err)
		states := serverStateMap(&cfg)
		found := false
		for _, transition := range states[StateStreaming].Transitions {
			if transition.MsgType == MessageTypeCompressedBlocks {
				found = true
			}
		}
		require.Equal(t, enabled, found)
		client := &Client{config: &cfg}
		found = false
		for _, transition := range client.protocolStateMap()[StateStreaming].Transitions {
			if transition.MsgType == MessageTypeCompressedBlocks {
				found = true
			}
		}
		require.Equal(t, enabled, found)
	}
}

func FuzzCompressedBlocks(f *testing.F) {
	f.Add(uint64(0), []byte{0x82, 0x06, 0x80})
	f.Add(uint64(1), []byte{0x28, 0xb5, 0x2f, 0xfd})
	f.Fuzz(func(t *testing.T, encoding uint64, payload []byte) {
		if len(payload) > 65536 {
			t.Skip()
		}
		raw, err := (&MsgCompressedBlocks{Encoding: encoding, EncodedData: payload}).decompressed(
			65536,
		)
		if err == nil {
			require.LessOrEqual(t, len(raw), 65536)
		}
	})
}
