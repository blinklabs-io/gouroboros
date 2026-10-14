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

// LeiosMaxTxsPerEndorserBlock is the most transaction references an
// endorser block may carry. The reference node derives it from the 600000-byte
// endorser block ceiling over the smallest encoded reference, and rejects a
// larger count before decoding any reference.
const LeiosMaxTxsPerEndorserBlock = 17142

const cborTypeUnsignedInt uint8 = 0x00

// LeiosEndorserBlock is a Leios endorser block: a bare, definite-length CBOR
// map from transaction hash to encoded transaction size, in endorser-block
// order:
//
//	endorser_block = { 1*17142 hash32 => uint .size 4 }
//
// Leios is an overlay protocol, not a ledger era. This is not a normal ledger
// block and intentionally does not implement Block.
type LeiosEndorserBlock struct {
	cbor.DecodeStoreCbor
	TransactionReferences []LeiosTransactionReference
}

type LeiosTransactionReference struct {
	TransactionHash Blake2b256
	TransactionSize uint32
}

func (b *LeiosEndorserBlock) UnmarshalCBOR(cborData []byte) error {
	if len(cborData) == 0 {
		return errors.New("empty leios endorser block")
	}
	refs, err := decodeLeiosTransactionReferences(cborData)
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
	return encodeLeiosTransactionReferences(b.TransactionReferences)
}

// Hash returns the endorser block hash that ranking-block headers announce:
// Blake2b-256 of the canonical encoding of the transaction references, with
// every CBOR head in its shortest form. The decoder accepts wider heads, as
// the reference decoder does, and the reference node checks a received block
// by hashing its re-encoding rather than the bytes it arrived as, so a decoded
// block hashes the same as the canonical form of its references whatever
// encoding it was decoded from. A block that cannot be encoded hashes to the
// zero hash.
func (b LeiosEndorserBlock) Hash() Blake2b256 {
	if err := validateLeiosTransactionReferences(b.TransactionReferences); err != nil {
		return Blake2b256{}
	}
	raw, err := encodeLeiosTransactionReferences(b.TransactionReferences)
	if err != nil {
		return Blake2b256{}
	}
	return Blake2b256Hash(raw)
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
	if len(refs) > LeiosMaxTxsPerEndorserBlock {
		return leiosReferenceCountError(len(refs))
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
	if count < 1 || count > LeiosMaxTxsPerEndorserBlock {
		if count == 0 {
			return nil, errors.New(
				"leios endorser block must contain at least one transaction reference",
			)
		}
		return nil, leiosReferenceCountError(count)
	}
	// A hash32 needs a two-byte header plus its payload, and size needs
	// at least one byte. A claimed count cannot allocate absent entries.
	const minimumReferenceBytes = Blake2b256Size + 3
	if count > (len(raw)-dec.Position())/minimumReferenceBytes {
		return nil, errors.New("leios reference count exceeds encoded entries")
	}
	refs := make([]LeiosTransactionReference, 0)
	seen := make(map[Blake2b256]struct{})
	for idx := range count {
		// The reference decodes each entry with decodeBytes and decodeWord32,
		// which accept a definite head of any width but no indefinite-length
		// byte string and no tagged value, so a bignum size is refused too.
		if err := requireLeiosDefiniteHead(
			raw,
			dec.Position(),
			cbor.CborTypeByteString,
		); err != nil {
			return nil, fmt.Errorf(
				"decode leios transaction reference %d hash: %w",
				idx,
				err,
			)
		}
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
		if err := requireLeiosDefiniteHead(
			raw,
			dec.Position(),
			cborTypeUnsignedInt,
		); err != nil {
			return nil, fmt.Errorf(
				"decode leios transaction reference %d size: %w",
				idx,
				err,
			)
		}
		var size uint64
		if _, _, err := dec.Decode(&size); err != nil {
			return nil, fmt.Errorf(
				"decode leios transaction reference %d size: %w",
				idx,
				err,
			)
		}
		if size > math.MaxUint32 {
			return nil, fmt.Errorf(
				"leios transaction reference %d size exceeds uint32: %d",
				idx,
				size,
			)
		}
		refs = append(refs, LeiosTransactionReference{
			TransactionHash: txHash,
			TransactionSize: uint32(size),
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

// requireLeiosDefiniteHead checks that the data item at pos has the given
// major type and a definite argument of any width.
func requireLeiosDefiniteHead(raw []byte, pos int, major uint8) error {
	if pos < 0 || pos >= len(raw) {
		return errors.New("unexpected end of data")
	}
	head := raw[pos]
	if head&cbor.CborTypeMask != major {
		return fmt.Errorf("unexpected CBOR major type %d", head>>5)
	}
	// Additional information 24-27 carries a 1-8 byte argument; 28-30 are
	// reserved and 31 marks an indefinite length.
	if head&0x1f > 27 {
		return errors.New("indefinite or reserved CBOR head")
	}
	return nil
}

func leiosReferenceCountError(count int) error {
	return fmt.Errorf(
		"leios endorser block reference count %d not in [1, %d]",
		count,
		LeiosMaxTxsPerEndorserBlock,
	)
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
