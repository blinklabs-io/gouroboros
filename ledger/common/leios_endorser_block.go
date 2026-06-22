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

package common

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// LeiosEndorserBlock implements the experimental CIP-0164 endorser_block CDDL:
//
//	endorser_block = [ transaction_references : omap<hash32, uint16> ]
//
// Leios is an overlay protocol, not a ledger era. This is not a normal ledger
// block and intentionally does not implement Block.
type LeiosEndorserBlock struct {
	cbor.DecodeStoreCbor
	TransactionReferences []LeiosTransactionReference
}

type LeiosTransactionReference struct {
	TransactionHash Blake2b256
	TransactionSize uint16
}

func (b *LeiosEndorserBlock) UnmarshalCBOR(cborData []byte) error {
	var items []cbor.RawMessage
	bytesRead, err := cbor.Decode(cborData, &items)
	if err != nil {
		return err
	}
	if bytesRead != len(cborData) {
		return fmt.Errorf(
			"trailing bytes after leios endorser block: %d",
			len(cborData)-bytesRead,
		)
	}
	if len(items) != 1 {
		return fmt.Errorf(
			"invalid leios endorser block: expected 1 component, got %d",
			len(items),
		)
	}
	refs, err := decodeLeiosTransactionReferences(items[0])
	if err != nil {
		return err
	}
	b.TransactionReferences = refs
	b.SetCbor(cborData)
	return nil
}

func (b LeiosEndorserBlock) MarshalCBOR() ([]byte, error) {
	if raw := b.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if err := validateLeiosTransactionReferences(b.TransactionReferences); err != nil {
		return nil, err
	}
	refMap, err := encodeLeiosTransactionReferences(b.TransactionReferences)
	if err != nil {
		return nil, err
	}
	return cbor.Encode([]any{cbor.RawMessage(refMap)})
}

func (b LeiosEndorserBlock) Validate() error {
	return validateLeiosTransactionReferences(b.TransactionReferences)
}

func NewLeiosEndorserBlockFromCbor(
	data []byte,
) (*LeiosEndorserBlock, error) {
	var block LeiosEndorserBlock
	bytesRead, err := cbor.Decode(data, &block)
	if err != nil {
		return nil, err
	}
	if bytesRead != len(data) {
		return nil, fmt.Errorf(
			"trailing bytes after leios endorser block: %d",
			len(data)-bytesRead,
		)
	}
	return &block, nil
}

func validateLeiosTransactionReferences(
	refs []LeiosTransactionReference,
) error {
	if len(refs) == 0 {
		return errors.New(
			"leios endorser block must contain at least one transaction reference",
		)
	}
	seen := make(map[Blake2b256]struct{}, len(refs))
	for idx, ref := range refs {
		if ref.TransactionSize == 0 {
			return fmt.Errorf(
				"leios endorser block transaction reference at index %d has zero size",
				idx,
			)
		}
		if _, ok := seen[ref.TransactionHash]; ok {
			return fmt.Errorf(
				"duplicate leios endorser block transaction reference at index %d",
				idx,
			)
		}
		seen[ref.TransactionHash] = struct{}{}
	}
	return nil
}

func encodeLeiosTransactionReferences(
	refs []LeiosTransactionReference,
) ([]byte, error) {
	ret := encodeCborMapHeader(uint64(len(refs)))
	for _, ref := range refs {
		hashCbor, err := cbor.Encode(ref.TransactionHash)
		if err != nil {
			return nil, err
		}
		sizeCbor, err := cbor.Encode(ref.TransactionSize)
		if err != nil {
			return nil, err
		}
		ret = append(ret, hashCbor...)
		ret = append(ret, sizeCbor...)
	}
	return ret, nil
}

func decodeLeiosTransactionReferences(
	raw cbor.RawMessage,
) ([]LeiosTransactionReference, error) {
	dec, err := cbor.NewStreamDecoder(raw)
	if err != nil {
		return nil, fmt.Errorf(
			"decode leios endorser block transaction references: %w",
			err,
		)
	}
	count, _, _, err := dec.DecodeMapHeader()
	if err != nil {
		return nil, fmt.Errorf(
			"decode leios endorser block transaction references: %w",
			err,
		)
	}
	refs := make([]LeiosTransactionReference, 0, count)
	seen := make(map[Blake2b256]struct{}, count)
	for idx := range count {
		var hashBytes cbor.ByteString
		if _, _, err := dec.Decode(&hashBytes); err != nil {
			return nil, fmt.Errorf(
				"decode leios transaction reference %d hash: %w",
				idx,
				err,
			)
		}
		hash := hashBytes.Bytes()
		if len(hash) != Blake2b256Size {
			return nil, fmt.Errorf(
				"leios transaction reference %d hash must be %d bytes, got %d",
				idx,
				Blake2b256Size,
				len(hash),
			)
		}
		txHash := NewBlake2b256(hash)
		if _, ok := seen[txHash]; ok {
			return nil, fmt.Errorf(
				"duplicate leios endorser block transaction reference at index %d",
				idx,
			)
		}
		seen[txHash] = struct{}{}
		var size uint64
		if _, _, err := dec.Decode(&size); err != nil {
			return nil, fmt.Errorf(
				"decode leios transaction reference %d size: %w",
				idx,
				err,
			)
		}
		if size > math.MaxUint16 {
			return nil, fmt.Errorf(
				"leios transaction reference %d size exceeds uint16: %d",
				idx,
				size,
			)
		}
		refs = append(refs, LeiosTransactionReference{
			TransactionHash: txHash,
			TransactionSize: uint16(size),
		})
	}
	if !dec.EOF() {
		return nil, fmt.Errorf(
			"trailing bytes after leios transaction references map: %d",
			len(raw)-dec.Position(),
		)
	}
	if err := validateLeiosTransactionReferences(refs); err != nil {
		return nil, err
	}
	return refs, nil
}

func encodeCborMapHeader(count uint64) []byte {
	return encodeCborTypeHeader(cbor.CborTypeMap, count)
}

func encodeCborTypeHeader(major uint8, value uint64) []byte {
	switch {
	case value <= 23:
		return []byte{major | byte(value)}
	case value <= math.MaxUint8:
		return []byte{major | 24, byte(value)}
	case value <= math.MaxUint16:
		ret := []byte{major | 25, 0, 0}
		binary.BigEndian.PutUint16(ret[1:], uint16(value))
		return ret
	case value <= math.MaxUint32:
		ret := []byte{major | 26, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(ret[1:], uint32(value))
		return ret
	default:
		ret := []byte{major | 27, 0, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint64(ret[1:], value)
		return ret
	}
}
