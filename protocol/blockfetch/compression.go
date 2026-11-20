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
	"fmt"
	"io"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/klauspost/compress/zstd"
)

// MessageTypeCompressedBlocks carries a CBOR sequence of wrapped blocks.
const MessageTypeCompressedBlocks = 6

// CompressionRaw and CompressionZstandard are the CIP-0150 encoding identifiers.
const (
	CompressionRaw uint64 = iota
	CompressionZstandard
)

// MaxDecompressedBatchBytes bounds allocation independently of compression ratio.
const MaxDecompressedBatchBytes = 64 << 20

// MsgCompressedBlocks transfers concatenated wrapped-block CBOR items.
type MsgCompressedBlocks struct {
	protocol.MessageBase
	Encoding    uint64
	EncodedData []byte
}

func enableCompressedTransition(states protocol.StateMap) {
	entry := states[StateStreaming]
	entry.Transitions = append(entry.Transitions, protocol.StateTransition{
		MsgType: MessageTypeCompressedBlocks, NewState: StateStreaming,
	})
	states[StateStreaming] = entry
}

func compressBlocks(raw []byte) (*MsgCompressedBlocks, error) {
	if len(raw) == 0 || len(raw) > MaxDecompressedBatchBytes {
		return nil, fmt.Errorf(
			"%s: invalid compression batch size",
			ProtocolName,
		)
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return nil, err
	}
	defer encoder.Close()
	encoded := encoder.EncodeAll(raw, nil)
	return &MsgCompressedBlocks{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeCompressedBlocks,
		},
		Encoding: CompressionZstandard, EncodedData: encoded,
	}, nil
}

func (m *MsgCompressedBlocks) decompressed(limit uint64) ([]byte, error) {
	if limit > MaxDecompressedBatchBytes {
		limit = MaxDecompressedBatchBytes
	}
	if len(m.EncodedData) == 0 ||
		len(m.EncodedData) > MaxDecompressedBatchBytes {
		return nil, ErrRangeByteLimit
	}
	switch m.Encoding {
	case CompressionRaw:
		if uint64(len(m.EncodedData)) > limit {
			return nil, ErrRangeByteLimit
		}
		return m.EncodedData, nil
	case CompressionZstandard:
		decoder, err := zstd.NewReader(bytes.NewReader(m.EncodedData),
			zstd.WithDecoderConcurrency(1),
			zstd.WithDecoderMaxMemory(MaxDecompressedBatchBytes),
			zstd.WithDecoderMaxWindow(MaxDecompressedBatchBytes),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: compressed batch: %w",
				ProtocolName,
				err,
			)
		}
		defer decoder.Close()
		// LimitReader bounds the actual output, including frames without a size.
		raw, err := io.ReadAll(io.LimitReader(decoder, int64(limit)+1))
		if err != nil {
			return nil, fmt.Errorf(
				"%s: compressed batch: %w",
				ProtocolName,
				err,
			)
		}
		if uint64(len(raw)) > limit {
			return nil, ErrRangeByteLimit
		}
		if len(raw) == 0 {
			return nil, fmt.Errorf("%s: empty compressed batch", ProtocolName)
		}
		return raw, nil
	default:
		return nil, fmt.Errorf(
			"%s: unsupported compression encoding %d",
			ProtocolName,
			m.Encoding,
		)
	}
}

func (c *Client) handleCompressedBlocks(generic protocol.Message) error {
	msg, ok := generic.(*MsgCompressedBlocks)
	if !ok || !c.config.CompressionEnabled {
		return fmt.Errorf("%s: compressed blocks disabled", ProtocolName)
	}
	c.queueMutex.Lock()
	req, err := c.headForResponse("CompressedBlocks", true)
	if err != nil {
		c.queueMutex.Unlock()
		return err
	}
	// Charge the outer wire envelope separately; each expanded block is charged
	// by handleBlock, so compression cannot bypass either range byte bound.
	wireBytes := uint64(len(msg.Cbor()))
	if wireBytes > c.maxRangeBytes()-min(req.bytesReceived, c.maxRangeBytes()) {
		c.queueMutex.Unlock()
		return c.failRequest(req, ErrRangeByteLimit)
	}
	req.bytesReceived += wireBytes
	remaining := c.maxRangeBytes() - req.bytesReceived
	if !req.deadline.IsZero() && !time.Now().Before(req.deadline) {
		c.queueMutex.Unlock()
		return c.failRequest(req, ErrRangeTimeout)
	}
	c.queueMutex.Unlock()
	raw, err := msg.decompressed(remaining)
	if err != nil {
		return c.failRequest(req, err)
	}
	count := uint64(0)
	for len(raw) > 0 {
		select {
		case <-c.StopChan():
			return protocol.ErrProtocolShuttingDown
		default:
		}
		// Match ordinary wrapped blocks: ledger CBOR permits deeper nesting
		// than the outer protocol envelope.
		var item cbor.RawMessage
		n, err := cbor.Decode(raw, &item)
		if err != nil {
			return c.failRequest(
				req,
				fmt.Errorf(
					"%s: invalid compressed block sequence: %w",
					ProtocolName,
					err,
				),
			)
		}
		if n <= 0 {
			return c.failRequest(
				req,
				fmt.Errorf("%s: empty block item", ProtocolName),
			)
		}
		count++
		if count > c.maxBlocksPerRange() {
			return c.failRequest(req, ErrRangeBlockLimit)
		}
		block := NewMsgBlock(item)
		encoded, err := block.MarshalCBOR()
		if err != nil {
			return c.failRequest(req, err)
		}
		block.SetCbor(encoded)
		if err := c.handleBlock(block); err != nil {
			return err
		}
		raw = raw[n:]
	}
	return nil
}
