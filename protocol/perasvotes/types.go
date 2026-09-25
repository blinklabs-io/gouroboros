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

package perasvotes

import (
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// VRFCert is the CIP-0140 draft's [output, proof] tuple. The draft fixes the
// proof at 80 bytes but leaves the output length unconstrained.
type VRFCert struct {
	Output []byte
	Proof  []byte
}

func (c VRFCert) MarshalCBOR() ([]byte, error) {
	if len(c.Proof) != 80 {
		return nil, fmt.Errorf(
			"peras VRF proof must be 80 bytes, got %d",
			len(c.Proof),
		)
	}
	return cbor.Encode([]any{c.Output, c.Proof})
}

func (c *VRFCert) UnmarshalCBOR(cborData []byte) error {
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &fields); err != nil {
		return fmt.Errorf("decode Peras VRF certificate: %w", err)
	}
	if len(fields) != 2 {
		return fmt.Errorf(
			"peras VRF certificate must have 2 fields, got %d",
			len(fields),
		)
	}
	var tmp VRFCert
	if _, err := cbor.Decode(fields[0], &tmp.Output); err != nil {
		return fmt.Errorf("decode Peras VRF output: %w", err)
	}
	if _, err := cbor.Decode(fields[1], &tmp.Proof); err != nil {
		return fmt.Errorf("decode Peras VRF proof: %w", err)
	}
	if len(tmp.Proof) != 80 {
		return fmt.Errorf("peras VRF proof must be 80 bytes, got %d", len(tmp.Proof))
	}
	*c = tmp
	return nil
}

// Vote is the eight-field vote record in CIP-0140's draft CDDL at revision
// eb6796a (2026-09-25). It is draft-tracking; the CIP says the vote and
// certificate scheme may change before activation.
type Vote struct {
	cbor.DecodeStoreCbor
	VoterID      []byte
	VotingRound  uint64
	BlockHash    []byte
	VotingProof  VRFCert
	VotingWeight uint64
	KESPeriod    uint64
	KESVKey      []byte
	KESSignature []byte
}

func (v Vote) MarshalCBOR() ([]byte, error) {
	if raw := v.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if err := v.Validate(); err != nil {
		return nil, err
	}
	proof, err := v.VotingProof.MarshalCBOR()
	if err != nil {
		return nil, err
	}
	return cbor.Encode([]any{
		v.VoterID,
		v.VotingRound,
		v.BlockHash,
		cbor.RawMessage(proof),
		v.VotingWeight,
		v.KESPeriod,
		v.KESVKey,
		v.KESSignature,
	})
}

func (v *Vote) UnmarshalCBOR(cborData []byte) error {
	var fields []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &fields); err != nil {
		return fmt.Errorf("decode Peras vote: %w", err)
	}
	if len(fields) != 8 {
		return fmt.Errorf("peras vote must have 8 fields, got %d", len(fields))
	}
	var tmp Vote
	decode := func(index int, target any, field string) error {
		if _, err := cbor.Decode(fields[index], target); err != nil {
			return fmt.Errorf("decode Peras vote %s: %w", field, err)
		}
		return nil
	}
	for _, field := range []struct {
		index int
		value any
		name  string
	}{
		{0, &tmp.VoterID, "voter id"},
		{1, &tmp.VotingRound, "round"},
		{2, &tmp.BlockHash, "block hash"},
		{3, &tmp.VotingProof, "VRF proof"},
		{4, &tmp.VotingWeight, "voting weight"},
		{5, &tmp.KESPeriod, "KES period"},
		{6, &tmp.KESVKey, "KES verification key"},
		{7, &tmp.KESSignature, "KES signature"},
	} {
		if err := decode(field.index, field.value, field.name); err != nil {
			return err
		}
	}
	if err := tmp.Validate(); err != nil {
		return err
	}
	tmp.SetCbor(cborData)
	*v = tmp
	return nil
}

// Validate enforces the fixed-size byte strings in CIP-0140's draft CDDL.
func (v Vote) Validate() error {
	for _, field := range []struct {
		name string
		got  int
		want int
	}{
		{"voter id", len(v.VoterID), 32},
		{"block hash", len(v.BlockHash), 32},
		{"KES verification key", len(v.KESVKey), 32},
		{"KES signature", len(v.KESSignature), 448},
	} {
		if field.got != field.want {
			return fmt.Errorf(
				"peras vote %s must be %d bytes, got %d",
				field.name,
				field.want,
				field.got,
			)
		}
	}
	if len(v.VotingProof.Proof) != 80 {
		return fmt.Errorf(
			"peras vote VRF proof must be 80 bytes, got %d",
			len(v.VotingProof.Proof),
		)
	}
	return nil
}

// VoterCert is the draft's per-voter CBOR record, whose current CDDL is the
// same eight-field tuple as Vote. The alias is a draft name only; no separate
// voter-certificate format is defined at CIP-0140 revision eb6796a
// (2026-09-25).
type VoterCert = Vote

// VoteCert preserves one complete CBOR value for the aggregate vote
// certificate. CIP-0140 revision eb6796a (2026-09-25) defers the aggregate
// certificate encoding to another proposal, so this wrapper deliberately
// does not interpret or re-encode its fields.
type VoteCert cbor.RawMessage

func (c VoteCert) MarshalCBOR() ([]byte, error) {
	if len(c) == 0 {
		return nil, errors.New("peras vote certificate has no encoded value")
	}
	var raw cbor.RawMessage
	if _, err := cbor.Decode(c, &raw); err != nil {
		return nil, fmt.Errorf("invalid encoded Peras vote certificate: %w", err)
	}
	return []byte(c), nil
}

func (c *VoteCert) UnmarshalCBOR(cborData []byte) error {
	if len(cborData) == 0 {
		return errors.New("peras vote certificate has no encoded value")
	}
	var raw cbor.RawMessage
	if _, err := cbor.Decode(cborData, &raw); err != nil {
		return fmt.Errorf("decode Peras vote certificate: %w", err)
	}
	*c = VoteCert(raw)
	return nil
}
