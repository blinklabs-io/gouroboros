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
	"unicode/utf8"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// ValidateCBORArrayLength checks that data is one CBOR array containing the
// required number of elements. Definite and indefinite arrays are both
// accepted; callers can then decode the validated shape into a struct.
func ValidateCBORArrayLength(data []byte, expected int, name string) error {
	if expected < 0 {
		return fmt.Errorf("%s has an invalid expected array length %d", name, expected)
	}
	arrayLength, _, indefinite := cbor.ArrayInfo(data)
	if arrayLength < 0 && !indefinite {
		return fmt.Errorf("%s must be a CBOR array", name)
	}
	if !indefinite {
		if arrayLength != expected {
			return fmt.Errorf("%s must contain %d array elements, got %d", name, expected, arrayLength)
		}
		if err := cbor.ValidateExact(data); err != nil {
			return fmt.Errorf("decode %s: %w", name, err)
		}
		return nil
	}
	var items []cbor.RawMessage
	n, err := cbor.Decode(data, &items)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if n != len(data) {
		return fmt.Errorf("decode %s: %d trailing bytes after array", name, len(data)-n)
	}
	if len(items) != expected {
		return fmt.Errorf("%s must contain %d array elements, got %d", name, expected, len(items))
	}
	return nil
}

// ValidateDefiniteCBORArrayLength checks that data is a definite-length CBOR
// array with exactly the required number of elements.
func ValidateDefiniteCBORArrayLength(
	data []byte,
	expected int,
	name string,
) error {
	if expected < 0 {
		return fmt.Errorf("%s has an invalid expected array length %d", name, expected)
	}
	length, _, indefinite := cbor.ArrayInfo(data)
	if length < 0 {
		return fmt.Errorf("%s must be a CBOR array", name)
	}
	if indefinite {
		return fmt.Errorf("%s must be a definite-length CBOR array", name)
	}
	if length != expected {
		return fmt.Errorf("%s must contain %d array elements, got %d", name, expected, length)
	}
	if err := cbor.ValidateExact(data); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}

type byteStringRange struct {
	start int
	end   int
}

func decodeByteStringArray(
	data []byte,
	expectedLengths []int,
	name string,
) ([4][]byte, error) {
	var fields [4][]byte
	if len(expectedLengths) > len(fields) {
		return fields, fmt.Errorf("%s has too many byte string fields", name)
	}
	arrayLength, headerLength, indefinite := cbor.ArrayInfo(data)
	if arrayLength < 0 && !indefinite {
		return fields, fmt.Errorf("%s must be a CBOR array", name)
	}
	if !indefinite && arrayLength != len(expectedLengths) {
		return fields, fmt.Errorf(
			"%s must contain %d array elements, got %d",
			name,
			len(expectedLengths),
			arrayLength,
		)
	}
	if uint64(headerLength) > uint64(len(data)) {
		return fields, fmt.Errorf("decode %s: array header exceeds data", name)
	}
	offset := int(headerLength)
	var ranges [4]byteStringRange
	var totalLength int
	for i, expectedLength := range expectedLengths {
		if offset >= len(data) || (indefinite && data[offset] == 0xff) {
			return fields, fmt.Errorf("decode %s: truncated byte string array", name)
		}
		length, byteHeaderLength, indefiniteByteString, err := byteStringHeader(data[offset:])
		if err != nil {
			return fields, fmt.Errorf("decode %s field %d: %w", name, i, err)
		}
		if indefiniteByteString {
			return fields, fmt.Errorf("%s field %d must be a definite-length byte string", name, i)
		}
		if expectedLength >= 0 && length != uint64(expectedLength) {
			return fields, fmt.Errorf(
				"invalid %s field %d length: expected %d bytes, got %d",
				name,
				i,
				expectedLength,
				length,
			)
		}
		remaining := len(data) - offset
		if byteHeaderLength > remaining {
			return fields, fmt.Errorf("decode %s field %d: byte string exceeds data", name, i)
		}
		remaining -= byteHeaderLength
		if length > uint64(remaining) { // #nosec G115 -- remaining is non-negative and bounded by data above.
			return fields, fmt.Errorf("decode %s field %d: byte string exceeds data", name, i)
		}
		start := offset + byteHeaderLength
		end := start + int(length) // #nosec G115 -- bounded by the input length above
		ranges[i] = byteStringRange{start: start, end: end}
		totalLength += int(length) // #nosec G115 -- disjoint fields are bounded by data
		offset = end
	}
	if indefinite {
		if offset >= len(data) || data[offset] != 0xff {
			return fields, fmt.Errorf(
				"%s must contain exactly %d array elements",
				name,
				len(expectedLengths),
			)
		}
		offset++
	}
	if offset != len(data) {
		return fields, fmt.Errorf("decode %s: trailing data after byte string array", name)
	}

	ownedData := make([]byte, totalLength)
	ownedOffset := 0
	for i := range expectedLengths {
		fieldLength := ranges[i].end - ranges[i].start
		copy(ownedData[ownedOffset:], data[ranges[i].start:ranges[i].end])
		ownedEnd := ownedOffset + fieldLength
		fields[i] = ownedData[ownedOffset:ownedEnd:ownedEnd]
		ownedOffset = ownedEnd
	}
	return fields, nil
}

// ValidateNullOrFixedLengthByteStringCBOR validates a CBOR null or a definite
// byte string of exactly expected bytes. Undefined is not a null value.
func ValidateNullOrFixedLengthByteStringCBOR(
	data []byte,
	expected int,
	name string,
) error {
	if len(data) == 1 && data[0] == 0xf6 {
		return nil
	}
	return validateFixedLengthByteString(data, expected, name)
}

// rewardAccountCBOR isolates reward addresses from the Blake2b224 decoder.
// The ledger wire value is a one-byte address header followed by a 28-byte
// credential, while the exported certificate field retains only the
// credential. Keep the credential kind as well as its hash so decoding does
// not erase the distinction between key and script reward accounts.
type rewardAccountCBOR struct {
	credential Credential
	// networkId holds the low nibble of the address header byte, and
	// networkIdKnown records whether a header byte was present at all. The
	networkId      uint
	networkIdKnown bool
}

func unmarshalFixedLengthByteString(
	cborData []byte,
	destination []byte,
	name string,
) error {
	if err := validateFixedLengthByteString(cborData, len(destination), name); err != nil {
		return err
	}
	decoded, decodedLength, err := decodeByteString(cborData)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	copy(destination, decoded[:decodedLength])
	return nil
}

func validateFixedLengthByteString(cborData []byte, expected int, name string) error {
	if expected < 0 {
		return fmt.Errorf("%s has a negative expected length", name)
	}
	_, _, indefinite, err := byteStringHeader(cborData)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if indefinite {
		return fmt.Errorf("decode %s: expected a definite-length byte string", name)
	}
	length, _, _, err := byteStringHeader(cborData)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if length != uint64(expected) { // #nosec G115 -- expected is non-negative after the guard above.
		return fmt.Errorf(
			"invalid %s length: expected %d bytes, got %d",
			name,
			expected,
			length,
		)
	}
	return nil
}

func validateDefiniteTextString(cborData []byte, name string) error {
	if len(cborData) == 0 || cborData[0]&cbor.CborTypeMask != cbor.CborTypeTextString {
		return fmt.Errorf("%s must be a CBOR text string", name)
	}
	if cborData[0]&0x1f == 0x1f {
		return fmt.Errorf("%s must be a definite-length text string", name)
	}
	length, headerLength, _, err := textStringHeader(cborData)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if headerLength > len(cborData) {
		return fmt.Errorf("decode %s: text string header exceeds data", name)
	}
	payloadLength := len(cborData) - headerLength
	if length != uint64(payloadLength) { // #nosec G115 -- payloadLength is non-negative after the bound check above.
		return fmt.Errorf("decode %s: text string length does not match data", name)
	}
	if !utf8.Valid(cborData[headerLength:]) {
		return fmt.Errorf("%s contains invalid UTF-8", name)
	}
	return nil
}

func validateDefiniteByteString(cborData []byte, name string) error {
	length, headerLength, indefinite, err := byteStringHeader(cborData)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if indefinite {
		return fmt.Errorf("%s must be a definite-length byte string", name)
	}
	if headerLength > len(cborData) {
		return fmt.Errorf("decode %s: byte string header exceeds data", name)
	}
	payloadLength := len(cborData) - headerLength
	if length != uint64(payloadLength) { // #nosec G115 -- payloadLength is non-negative after the bound check above.
		return fmt.Errorf("decode %s: byte string length does not match data", name)
	}
	return nil
}

func cborTagHeaderLength(cborData []byte) (int, error) {
	if len(cborData) == 0 || cborData[0]&cbor.CborTypeMask != cbor.CborTypeTag {
		return 0, errors.New("expected CBOR tag")
	}
	switch cborData[0] & 0x1f {
	case 0x18:
		if len(cborData) < 2 {
			return 0, errors.New("truncated CBOR tag")
		}
		return 2, nil
	case 0x19:
		if len(cborData) < 3 {
			return 0, errors.New("truncated CBOR tag")
		}
		return 3, nil
	case 0x1a:
		if len(cborData) < 5 {
			return 0, errors.New("truncated CBOR tag")
		}
		return 5, nil
	case 0x1b:
		if len(cborData) < 9 {
			return 0, errors.New("truncated CBOR tag")
		}
		return 9, nil
	case 0x1f:
		return 0, errors.New("indefinite CBOR tag")
	default:
		return 1, nil
	}
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
	return stringHeader(data, cbor.CborTypeByteString)
}

func textStringHeader(data []byte) (uint64, int, bool, error) {
	return stringHeader(data, cbor.CborTypeTextString)
}

func stringHeader(data []byte, majorType uint8) (uint64, int, bool, error) {
	if len(data) == 0 {
		return 0, 0, false, errors.New("empty CBOR data")
	}
	if data[0]&cbor.CborTypeMask != majorType {
		switch majorType {
		case cbor.CborTypeByteString:
			return 0, 0, false, errors.New("expected CBOR byte string")
		case cbor.CborTypeTextString:
			return 0, 0, false, errors.New("expected CBOR text string")
		}
		return 0, 0, false, fmt.Errorf("expected CBOR string type 0x%02x", majorType)
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
		return 0, 0, false, errors.New("invalid CBOR string length")
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
	if decodedLength != Blake2b224Size+1 {
		return fmt.Errorf(
			"invalid reward account length: expected 29 bytes, got %d",
			decodedLength,
		)
	}
	{
		// headerIsAccountAddress in Cardano.Ledger.Address:
		// header .&. 0b11101110 == 0b11100000. Bits 7-5 are the account
		// address prefix, bit 4 is the script flag and is free, bits 3-1
		// must be clear, and bit 0 is the network id. So the only valid
		// header bytes are 0xe0, 0xe1, 0xf0 and 0xf1.
		if decoded[0]&0xEE != 0xE0 {
			return fmt.Errorf(
				"invalid reward account address header: 0x%02x",
				decoded[0],
			)
		}
	}
	r.credential.CredType = CredentialTypeAddrKeyHash
	if decoded[0]&0x10 != 0 {
		r.credential.CredType = CredentialTypeScriptHash
	}
	copy(r.credential.Credential[:], decoded[1:])
	{
		// Bit 0 of a reward address header byte is the network id,
		// which the header check above has already constrained to 0 or
		// 1. See Cardano.Ledger.Address (aaNetworkId), read by
		// poolTransition's WrongNetworkPOOL check.
		r.networkId = uint(decoded[0] & 0x0F)
		r.networkIdKnown = true
	}
	return nil
}

func (a *GovAnchor) UnmarshalCBOR(cborData []byte) error {
	if a == nil {
		return errors.New("nil GovAnchor receiver")
	}
	if err := ValidateCBORArrayLength(
		cborData,
		2,
		"governance anchor",
	); err != nil {
		return err
	}
	var raw struct {
		cbor.StructAsArray
		Url      cbor.RawMessage
		DataHash Blake2b256
	}
	if _, err := cbor.Decode(cborData, &raw); err != nil {
		return fmt.Errorf("decode governance anchor: %w", err)
	}
	if err := validateDefiniteTextString(raw.Url, "governance anchor URL"); err != nil {
		return err
	}
	var decoded struct {
		cbor.StructAsArray
		Url      string
		DataHash Blake2b256
	}
	if _, err := cbor.Decode(raw.Url, &decoded.Url); err != nil {
		return fmt.Errorf("decode governance anchor: %w", err)
	}
	decoded.DataHash = raw.DataHash
	if err := validateGovAnchorURL(decoded.Url); err != nil {
		return err
	}
	a.Url = decoded.Url
	copy(a.DataHash[:], decoded.DataHash[:])
	return nil
}

// UnmarshalCBOR decodes `gov_action_id = [transaction_id : transaction_id,
// gov_action_index : uint .size 2]`.
//
// The index is read as a uint64 so the `.size 2` bound can be reported as a
// domain error rather than as a Go overflow, and so a value the field type
// happens to hold -- GovActionIdx is a uint32 -- is still refused.
func (id *GovActionId) UnmarshalCBOR(cborData []byte) error {
	if id == nil {
		return errors.New("nil GovActionId receiver")
	}
	if err := ValidateCBORArrayLength(
		cborData,
		2,
		"governance action ID",
	); err != nil {
		return err
	}
	var decoded struct {
		cbor.StructAsArray
		TransactionId Blake2b256
		GovActionIdx  uint64
	}
	if _, err := cbor.Decode(cborData, &decoded); err != nil {
		return fmt.Errorf("decode governance action ID: %w", err)
	}
	if decoded.GovActionIdx > MaxGovActionIdx {
		return fmt.Errorf(
			"decode governance action ID: index %d exceeds the maximum of %d",
			decoded.GovActionIdx,
			MaxGovActionIdx,
		)
	}
	copy(id.TransactionId[:], decoded.TransactionId[:])
	id.GovActionIdx = uint32(decoded.GovActionIdx)
	return nil
}
