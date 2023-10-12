// Copyright 2023 Blink Labs, LLC.
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
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
)

// WrappedBlock represents a block returned via a NtC RollForward message
type WrappedBlock struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_         struct{} `cbor:",toarray"`
	BlockType uint
	BlockCbor cbor.RawMessage
}

// NewWrappedBlock returns a new WrappedBlock
func NewWrappedBlock(blockType uint, blockCbor []byte) *WrappedBlock {
	return &WrappedBlock{
		BlockType: blockType,
		BlockCbor: blockCbor,
	}
}

// WrappedHeader represents a block header returned via NtN RollForward message
type WrappedHeader struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_          struct{} `cbor:",toarray"`
	Era        uint
	RawMessage cbor.RawMessage
	byronType  uint
	byronSize  uint
	headerCbor []byte
}

// NewWrappedHeader returns a new WrappedHeader
func NewWrappedHeader(era uint, byronType uint, blockCbor []byte) *WrappedHeader {
	w := &WrappedHeader{
		Era:       era,
		byronType: byronType,
	}
	// Record the original block size for Byron blocks
	if era == ledger.BlockHeaderTypeByron {
		// TODO: figure out why we have to add 2 to the length to match official message CBOR
		w.byronSize = uint(len(blockCbor)) + 2
	}
	// Parse block and extract header
	tmp := []cbor.RawMessage{}
	// TODO: figure out a better way to handle an error
	if _, err := cbor.Decode(blockCbor, &tmp); err != nil {
		return nil
	}
	w.headerCbor = tmp[0]
	return w
}

func (w *WrappedHeader) UnmarshalCBOR(data []byte) error {
	var tmpHeader struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_         struct{} `cbor:",toarray"`
		Era       uint
		HeaderRaw cbor.RawMessage
	}
	if _, err := cbor.Decode(data, &tmpHeader); err != nil {
		return err
	}
	w.Era = tmpHeader.Era
	switch w.Era {
	case ledger.BlockHeaderTypeByron:
		var wrappedHeaderByron wrappedHeaderByron
		if _, err := cbor.Decode(tmpHeader.HeaderRaw, &wrappedHeaderByron); err != nil {
			return err
		}
		w.byronType = wrappedHeaderByron.Metadata.Type
		w.byronSize = wrappedHeaderByron.Metadata.Size
		w.headerCbor = wrappedHeaderByron.RawHeader.Content.([]byte)
	default:
		var tag cbor.Tag
		if _, err := cbor.Decode(tmpHeader.HeaderRaw, &tag); err != nil {
			return err
		}
		w.headerCbor = tag.Content.([]byte)
	}
	return nil
}

func (w *WrappedHeader) MarshalCBOR() ([]byte, error) {
	ret := []interface{}{
		w.Era,
	}
	switch w.Era {
	case ledger.BlockHeaderTypeByron:
		tmp := []interface{}{
			[]interface{}{
				w.byronType,
				w.byronSize,
			},
			cbor.Tag{
				Number:  24,
				Content: w.headerCbor,
			},
		}
		ret = append(ret, tmp)
	default:
		tag := cbor.Tag{
			Number:  24,
			Content: w.headerCbor,
		}
		ret = append(ret, tag)
	}
	cborData, err := cbor.Encode(ret)
	if err != nil {
		return nil, err
	}
	return cborData, nil
}

// HeaderCbor returns the header CBOR
func (w *WrappedHeader) HeaderCbor() []byte {
	return w.headerCbor
}

// ByronType returns the block type for Byron blocks
func (w *WrappedHeader) ByronType() uint {
	return w.byronType
}

type wrappedHeaderByron struct {
	// Tells the CBOR decoder to convert to/from a struct and a CBOR array
	_        struct{} `cbor:",toarray"`
	Metadata struct {
		// Tells the CBOR decoder to convert to/from a struct and a CBOR array
		_    struct{} `cbor:",toarray"`
		Type uint
		Size uint
	}
	RawHeader cbor.Tag
}
