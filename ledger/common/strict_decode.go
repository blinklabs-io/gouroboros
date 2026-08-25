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

	"github.com/blinklabs-io/gouroboros/cbor"
)

// rewardAccountCBOR isolates reward addresses from the Blake2b224 decoder.
// The ledger wire value is a one-byte address header followed by a 28-byte
// credential, while the exported certificate field retains only the
// credential. Accept the repository's legacy 28-byte encoding as well so
// existing constructed values round-trip.
type rewardAccountCBOR struct {
	credential AddrKeyHash
}

func unmarshalFixedLengthByteString(
	cborData []byte,
	destination []byte,
	name string,
) error {
	decoded, decodedLength, err := decodeByteString(cborData)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if decodedLength != uint64(len(destination)) {
		return fmt.Errorf(
			"invalid %s length: expected %d bytes, got %d",
			name,
			len(destination),
			decodedLength,
		)
	}
	copy(destination, decoded[:decodedLength])
	return nil
}

func decodeByteString(cborData []byte) ([32]byte, uint64, error) {
	var decoded [32]byte
	length, headerLength, indefinite, err := byteStringHeader(cborData)
	if err != nil {
		return decoded, 0, err
	}
	if !indefinite {
		if length > uint64(len(decoded)) {
			return decoded, length, nil
		}
		payloadLength := int(length) // #nosec G115 -- bounded by decoded above
		if payloadLength != len(cborData)-headerLength {
			return decoded, 0, errors.New(
				"byte string length does not match data",
			)
		}
		copy(decoded[:], cborData[headerLength:])
		return decoded, length, nil
	}

	var total uint64
	for offset := headerLength; ; {
		if offset >= len(cborData) {
			return decoded, 0, errors.New("unterminated byte string")
		}
		if cborData[offset] == 0xff {
			if offset != len(cborData)-1 {
				return decoded, 0, errors.New("trailing data after byte string")
			}
			return decoded, total, nil
		}
		chunkLength, chunkHeaderLength, chunkIndefinite, err := byteStringHeader(
			cborData[offset:],
		)
		if err != nil {
			return decoded, 0, err
		}
		if chunkIndefinite {
			return decoded, 0, errors.New("indefinite byte string chunk")
		}
		if total > ^uint64(0)-chunkLength {
			return decoded, 0, errors.New("byte string length overflow")
		}
		if total+chunkLength > uint64(len(decoded)) {
			return decoded, total + chunkLength, nil
		}
		chunkLengthInt := int(chunkLength) // #nosec G115 -- bounded above
		chunkStart := offset + chunkHeaderLength
		if chunkLengthInt > len(cborData)-chunkStart {
			return decoded, 0, errors.New("byte string chunk exceeds data")
		}
		chunkEnd := chunkStart + chunkLengthInt
		copy(decoded[total:], cborData[chunkStart:chunkEnd])
		total += chunkLength
		offset = chunkEnd
	}
}

func byteStringHeader(data []byte) (uint64, int, bool, error) {
	if len(data) == 0 {
		return 0, 0, false, errors.New("empty CBOR data")
	}
	if data[0]&cbor.CborTypeMask != cbor.CborTypeByteString {
		return 0, 0, false, errors.New("expected CBOR byte string")
	}
	additionalInfo := data[0] & 0x1f
	switch additionalInfo {
	case 0x18:
		if len(data) < 2 {
			return 0, 0, false, errors.New("truncated byte string length")
		}
		return uint64(data[1]), 2, false, nil
	case 0x19:
		if len(data) < 3 {
			return 0, 0, false, errors.New("truncated byte string length")
		}
		return uint64(binary.BigEndian.Uint16(data[1:3])), 3, false, nil
	case 0x1a:
		if len(data) < 5 {
			return 0, 0, false, errors.New("truncated byte string length")
		}
		return uint64(binary.BigEndian.Uint32(data[1:5])), 5, false, nil
	case 0x1b:
		if len(data) < 9 {
			return 0, 0, false, errors.New("truncated byte string length")
		}
		return binary.BigEndian.Uint64(data[1:9]), 9, false, nil
	case 0x1f:
		return 0, 1, true, nil
	default:
		if additionalInfo < 0x18 {
			return uint64(additionalInfo), 1, false, nil
		}
		return 0, 0, false, errors.New("invalid byte string length")
	}
}

func (b *Blake2b256) UnmarshalCBOR(cborData []byte) error {
	if b == nil {
		return errors.New("nil Blake2b256 receiver")
	}
	return unmarshalFixedLengthByteString(
		cborData,
		b[:],
		"blake2b-256 hash",
	)
}

func (b *Blake2b224) UnmarshalCBOR(cborData []byte) error {
	if b == nil {
		return errors.New("nil Blake2b224 receiver")
	}
	return unmarshalFixedLengthByteString(
		cborData,
		b[:],
		"blake2b-224 hash",
	)
}

func (b *Blake2b160) UnmarshalCBOR(cborData []byte) error {
	if b == nil {
		return errors.New("nil Blake2b160 receiver")
	}
	return unmarshalFixedLengthByteString(
		cborData,
		b[:],
		"blake2b-160 hash",
	)
}

func (p *PoolId) UnmarshalCBOR(cborData []byte) error {
	if p == nil {
		return errors.New("nil PoolId receiver")
	}
	return unmarshalFixedLengthByteString(cborData, p[:], "pool ID")
}

func (i *IssuerVkey) UnmarshalCBOR(cborData []byte) error {
	if i == nil {
		return errors.New("nil IssuerVkey receiver")
	}
	return unmarshalFixedLengthByteString(
		cborData,
		i[:],
		"issuer verification key",
	)
}

func (r *rewardAccountCBOR) UnmarshalCBOR(cborData []byte) error {
	if r == nil {
		return errors.New("nil reward account receiver")
	}
	decoded, decodedLength, err := decodeByteString(cborData)
	if err != nil {
		return fmt.Errorf("decode reward account: %w", err)
	}
	if decodedLength != Blake2b224Size && decodedLength != Blake2b224Size+1 {
		return fmt.Errorf(
			"invalid reward account length: expected 28 or 29 bytes, got %d",
			decodedLength,
		)
	}
	credentialOffset := 0
	if decodedLength == Blake2b224Size+1 {
		credentialOffset = 1
	}
	copy(
		r.credential[:],
		decoded[credentialOffset:credentialOffset+Blake2b224Size],
	)
	return nil
}

func (a *GovAnchor) UnmarshalCBOR(cborData []byte) error {
	if a == nil {
		return errors.New("nil GovAnchor receiver")
	}
	var decoded struct {
		cbor.StructAsArray
		Url      string
		DataHash Blake2b256
	}
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		return fmt.Errorf("decode governance anchor: %w", err)
	}
	a.Url = decoded.Url
	copy(a.DataHash[:], decoded.DataHash[:])
	return nil
}

func (id *GovActionId) UnmarshalCBOR(cborData []byte) error {
	if id == nil {
		return errors.New("nil GovActionId receiver")
	}
	var decoded struct {
		cbor.StructAsArray
		TransactionId Blake2b256
		GovActionIdx  uint32
	}
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		return fmt.Errorf("decode governance action ID: %w", err)
	}
	copy(id.TransactionId[:], decoded.TransactionId[:])
	id.GovActionIdx = decoded.GovActionIdx
	return nil
}
