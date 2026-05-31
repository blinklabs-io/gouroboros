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
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

const LeiosBlsSignatureSize = 48

type LeiosVoteId struct {
	cbor.StructAsArray
	SlotNo  uint64
	VoterId uint64
}

type LeiosVote struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	SlotNo            uint64
	EndorserBlockHash Blake2b256
	VoterId           uint64
	VoteSignature     []byte
}

func (v LeiosVote) MarshalCBOR() ([]byte, error) {
	if raw := v.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode([]any{
		v.SlotNo,
		v.EndorserBlockHash,
		v.VoterId,
		v.VoteSignature,
	})
}

func (v *LeiosVote) UnmarshalCBOR(cborData []byte) error {
	type tLeiosVote LeiosVote
	var tmp tLeiosVote
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	if err := LeiosVote(tmp).Validate(); err != nil {
		return err
	}
	*v = LeiosVote(tmp)
	v.SetCbor(cborData)
	return nil
}

func (v LeiosVote) Validate() error {
	return ValidateLeiosSignature("LeiosVote: VoteSignature", v.VoteSignature)
}

// LeiosEbCertificate is the CIP-0164 EB certificate payload shape used by
// Leios vote aggregation helpers. It is intentionally separate from Dijkstra
// block certificate slots: the generated Dijkstra CDDL currently defines
// leios_cert = [] and peras_cert = [], implemented by ledger/dijkstra.
type LeiosEbCertificate struct {
	cbor.StructAsArray
	cbor.DecodeStoreCbor
	SlotNo              uint64
	EndorserBlockHash   Blake2b256
	Signers             []byte
	AggregatedSignature []byte
}

func (c LeiosEbCertificate) MarshalCBOR() ([]byte, error) {
	if raw := c.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode([]any{
		c.SlotNo,
		c.EndorserBlockHash,
		c.Signers,
		c.AggregatedSignature,
	})
}

func (c *LeiosEbCertificate) UnmarshalCBOR(
	cborData []byte,
) error {
	type tLeiosEbCertificate LeiosEbCertificate
	var tmp tLeiosEbCertificate
	if _, err := cbor.Decode(cborData, &tmp); err != nil {
		return err
	}
	if len(tmp.AggregatedSignature) != LeiosBlsSignatureSize {
		return fmt.Errorf(
			"LeiosEbCertificate: AggregatedSignature must be %d bytes (BLS signature), got %d",
			LeiosBlsSignatureSize,
			len(tmp.AggregatedSignature),
		)
	}
	*c = LeiosEbCertificate(tmp)
	c.SetCbor(cborData)
	return nil
}

// Validate checks committee-dependent fields that cannot be verified from CBOR
// alone.
func (c *LeiosEbCertificate) Validate(committeeSize uint64) error {
	if err := ValidateLeiosSignature(
		"LeiosEbCertificate: AggregatedSignature",
		c.AggregatedSignature,
	); err != nil {
		return err
	}
	return ValidateLeiosSignerBitfield(c.Signers, committeeSize)
}

func (c *LeiosEbCertificate) Signer(voterId uint64) bool {
	return LeiosSignerBit(c.Signers, voterId)
}

// GetAggregatedSignature returns the aggregated vote signature as a byte slice.
func (c *LeiosEbCertificate) GetAggregatedSignature() []byte {
	return c.AggregatedSignature
}

// SetAggregatedSignature sets the aggregated vote signature from a byte slice
// and validates it meets the 48-byte BLS signature requirement per CIP-0164.
func (c *LeiosEbCertificate) SetAggregatedSignature(sig []byte) error {
	if err := ValidateLeiosSignature(
		"LeiosEbCertificate: AggregatedSignature",
		sig,
	); err != nil {
		return err
	}
	c.AggregatedSignature = sig
	c.SetCbor(nil)
	return nil
}

func ValidateLeiosSignature(name string, sig []byte) error {
	if len(sig) != LeiosBlsSignatureSize {
		return fmt.Errorf(
			"%s must be %d bytes (BLS signature), got %d",
			name,
			LeiosBlsSignatureSize,
			len(sig),
		)
	}
	return nil
}

func LeiosSignerBitfieldSize(committeeSize uint64) uint64 {
	return (committeeSize + 7) / 8
}

func ValidateLeiosSignerBitfield(signers []byte, committeeSize uint64) error {
	expectedLen := LeiosSignerBitfieldSize(committeeSize)
	if uint64(len(signers)) != expectedLen {
		return fmt.Errorf(
			"leios signer bitfield must be %d bytes for committee size %d, got %d",
			expectedLen,
			committeeSize,
			len(signers),
		)
	}
	if committeeSize == 0 || committeeSize%8 == 0 {
		return nil
	}
	unusedBits := uint(8 - committeeSize%8)
	unusedMask := byte((1 << unusedBits) - 1)
	if signers[len(signers)-1]&unusedMask != 0 {
		return fmt.Errorf(
			"leios signer bitfield has non-zero unused bits for committee size %d",
			committeeSize,
		)
	}
	return nil
}

func LeiosSignerBit(signers []byte, voterId uint64) bool {
	byteIdx := voterId / 8
	if byteIdx >= uint64(len(signers)) {
		return false
	}
	bitIdx := 7 - voterId%8
	return signers[byteIdx]&(1<<bitIdx) != 0
}
