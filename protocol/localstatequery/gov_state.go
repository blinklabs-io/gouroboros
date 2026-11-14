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

package localstatequery

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
)

// The encoders in this file reproduce cardano-ledger's byte layout:
//
//   - StrictMaybe and Maybe are a list of zero or one items (encodeStrictMaybe).
//   - Map and list encoders emit a definite-length header up to
//     ledgerMapLenThreshold items and an indefinite-length one above it
//     (variableMapLenEncoding, variableListLenEncoding).
//   - A Map is written in the key type's Ord order. For Credential that puts
//     ScriptHashObj (wire tag 1) before KeyHashObj (wire tag 0).

// encodedPair is one already-encoded map entry.
type encodedPair struct {
	key   []byte
	value []byte
}

// encodeLedgerMap joins entries, in the order given, into a map using the
// ledger's variable-length header rule.
func encodeLedgerMap(pairs []encodedPair) []byte {
	buf := bytes.NewBuffer(nil)
	if len(pairs) > ledgerMapLenThreshold {
		buf.WriteByte(cborIndefiniteMapHeader)
	} else {
		buf.WriteByte(0xa0 | byte(len(pairs)&0x1f))
	}
	for _, p := range pairs {
		buf.Write(p.key)
		buf.Write(p.value)
	}
	if len(pairs) > ledgerMapLenThreshold {
		buf.WriteByte(cborBreak)
	}
	return buf.Bytes()
}

// encodeLedgerList joins already-encoded items into a list using the
// ledger's variable-length header rule.
func encodeLedgerList(items [][]byte) []byte {
	if len(items) > ledgerMapLenThreshold {
		return encodeIndefiniteList(items)
	}
	buf := bytes.NewBuffer([]byte{0x80 | byte(len(items)&0x1f)})
	for _, item := range items {
		buf.Write(item)
	}
	return buf.Bytes()
}

// credentialRank orders ScriptHashObj before KeyHashObj, as the derived Ord
// of Credential does.
func credentialRank(tag uint64) int {
	if tag == 1 {
		return 0
	}
	return 1
}

func compareCredentials(a, b StakeCredential) int {
	if c := cmp.Compare(credentialRank(a.Tag), credentialRank(b.Tag)); c != 0 {
		return c
	}
	return bytes.Compare(a.Bytes[:], b.Bytes[:])
}

func encodeCredentialKeyedMap[V any](
	m map[StakeCredential]V,
	encodeValue func(V) ([]byte, error),
) ([]byte, error) {
	keys := make([]StakeCredential, 0, len(m))
	for k := range m {
		if k.Tag > 1 {
			return nil, fmt.Errorf("invalid credential type: %d", k.Tag)
		}
		keys = append(keys, k)
	}
	slices.SortFunc(keys, compareCredentials)
	pairs := make([]encodedPair, 0, len(keys))
	for _, k := range keys {
		key, err := cbor.Encode([]any{k.Tag, k.Bytes[:]})
		if err != nil {
			return nil, err
		}
		value, err := encodeValue(m[k])
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, encodedPair{key: key, value: value})
	}
	return encodeLedgerMap(pairs), nil
}

func encodeVote(v lcommon.Vote) ([]byte, error) {
	return cbor.Encode(uint8(v))
}

func encodeUint64(v uint64) ([]byte, error) {
	return cbor.Encode(v)
}

func (s GovActionState) MarshalCBOR() ([]byte, error) {
	if len(s.ProposalProcedure) == 0 {
		return nil, errors.New("governance action state: missing proposal procedure")
	}
	id, err := cbor.Encode(s.Id)
	if err != nil {
		return nil, err
	}
	committee, err := encodeCredentialKeyedMap(s.CommitteeVotes, encodeVote)
	if err != nil {
		return nil, err
	}
	drep, err := encodeCredentialKeyedMap(s.DRepVotes, encodeVote)
	if err != nil {
		return nil, err
	}
	spoKeys := make([]ledger.Blake2b224, 0, len(s.SPOVotes))
	for k := range s.SPOVotes {
		spoKeys = append(spoKeys, k)
	}
	slices.SortFunc(spoKeys, func(a, b ledger.Blake2b224) int {
		return bytes.Compare(a[:], b[:])
	})
	spoPairs := make([]encodedPair, 0, len(spoKeys))
	for _, k := range spoKeys {
		key, err := cbor.Encode(k[:])
		if err != nil {
			return nil, err
		}
		value, err := encodeVote(s.SPOVotes[k])
		if err != nil {
			return nil, err
		}
		spoPairs = append(spoPairs, encodedPair{key: key, value: value})
	}
	proposedIn, err := cbor.Encode(s.ProposedIn)
	if err != nil {
		return nil, err
	}
	expiresAfter, err := cbor.Encode(s.ExpiresAfter)
	if err != nil {
		return nil, err
	}
	return bytes.Join([][]byte{
		{0x87},
		id,
		committee,
		drep,
		encodeLedgerMap(spoPairs),
		s.ProposalProcedure,
		proposedIn,
		expiresAfter,
	}, nil), nil
}

// MarshalCBOR encodes the committee as cardano-ledger's Committee: the
// member map in credential Ord order and the threshold.
func (c Committee) MarshalCBOR() ([]byte, error) {
	members, err := encodeCredentialKeyedMap(c.Members, encodeUint64)
	if err != nil {
		return nil, err
	}
	threshold, err := cbor.Encode(c.Threshold)
	if err != nil {
		return nil, err
	}
	return bytes.Join([][]byte{{0x82}, members, threshold}, nil), nil
}

// EncodeCommittee encodes the StrictMaybe Committee carried by
// [GovStateResult.Committee]: an empty list for a nil committee and a
// one-element list otherwise.
func EncodeCommittee(c *Committee) (cbor.RawMessage, error) {
	if c == nil {
		return cbor.RawMessage{0x80}, nil
	}
	enc, err := c.MarshalCBOR()
	if err != nil {
		return nil, err
	}
	return append(cbor.RawMessage{0x81}, enc...), nil
}

// PrevGovActionIds holds the root of each governance action purpose: the
// most recently enacted action of that purpose, or nil when none was enacted.
// It is cardano-ledger's GovRelation StrictMaybe, a four-element array in the
// order parameter change, hard fork, committee, constitution.
type PrevGovActionIds struct {
	ParameterChange *lcommon.GovActionId
	HardFork        *lcommon.GovActionId
	Committee       *lcommon.GovActionId
	Constitution    *lcommon.GovActionId
}

func (p *PrevGovActionIds) UnmarshalCBOR(data []byte) error {
	items, err := decodeBoundedArray(data, 4, "governance action roots")
	if err != nil {
		return err
	}
	if len(items) != 4 {
		return fmt.Errorf(
			"governance action roots: expected 4 elements, got %d",
			len(items),
		)
	}
	var out PrevGovActionIds
	for i, dst := range []**lcommon.GovActionId{
		&out.ParameterChange,
		&out.HardFork,
		&out.Committee,
		&out.Constitution,
	} {
		maybe, err := decodeBoundedArray(items[i], 1, "governance action root")
		if err != nil {
			return err
		}
		if len(maybe) == 0 {
			continue
		}
		var id lcommon.GovActionId
		if _, err := cbor.Decode(maybe[0], &id); err != nil {
			return err
		}
		*dst = &id
	}
	*p = out
	return nil
}

func (p PrevGovActionIds) MarshalCBOR() ([]byte, error) {
	buf := bytes.NewBuffer([]byte{0x84})
	for _, id := range []*lcommon.GovActionId{
		p.ParameterChange,
		p.HardFork,
		p.Committee,
		p.Constitution,
	} {
		if id == nil {
			buf.WriteByte(0x80)
			continue
		}
		enc, err := cbor.Encode(*id)
		if err != nil {
			return nil, err
		}
		buf.WriteByte(0x81)
		buf.Write(enc)
	}
	return buf.Bytes(), nil
}

// FuturePParamsKind says how certain the node is about the protocol
// parameters of the next epoch.
type FuturePParamsKind uint8

const (
	// FuturePParamsNone means no update is expected at the next epoch boundary.
	FuturePParamsNone FuturePParamsKind = 0
	// FuturePParamsDefinite means the parameters will be adopted at the next
	// epoch boundary.
	FuturePParamsDefinite FuturePParamsKind = 1
	// FuturePParamsPotential means the parameters may be adopted; PParams is
	// empty when the node determined there is no change.
	FuturePParamsPotential FuturePParamsKind = 2
)

// FuturePParamsResult is cardano-ledger's FuturePParams.
type FuturePParamsResult struct {
	Kind    FuturePParamsKind
	PParams cbor.RawMessage
}

// DecodeFuturePParams decodes the FuturePParams field:
// [0], [1, pparams] or [2, [] | [pparams]].
func (g *GovStateResult) DecodeFuturePParams() (*FuturePParamsResult, error) {
	items, err := decodeBoundedArray(g.FuturePParams, 2, "future protocol parameters")
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, errors.New("future protocol parameters: missing tag")
	}
	var tag uint64
	if _, err := cbor.Decode(items[0], &tag); err != nil {
		return nil, err
	}
	switch tag {
	case 0:
		if len(items) != 1 {
			return nil, errors.New("future protocol parameters: unexpected payload")
		}
		return &FuturePParamsResult{Kind: FuturePParamsNone}, nil
	case 1:
		if len(items) != 2 {
			return nil, errors.New("future protocol parameters: missing payload")
		}
		return &FuturePParamsResult{
			Kind:    FuturePParamsDefinite,
			PParams: items[1],
		}, nil
	case 2:
		if len(items) != 2 {
			return nil, errors.New("future protocol parameters: missing payload")
		}
		maybe, err := decodeBoundedArray(items[1], 1, "potential protocol parameters")
		if err != nil {
			return nil, err
		}
		out := &FuturePParamsResult{Kind: FuturePParamsPotential}
		if len(maybe) == 1 {
			out.PParams = maybe[0]
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unknown future protocol parameters tag: %d", tag)
	}
}

// PulsingSnapshot is cardano-ledger's PulsingSnapshot: the proposals, DRep
// stake distribution, DRep registrations and stake pool distribution as they
// stood at the start of the epoch.
type PulsingSnapshot struct {
	Proposals []GovActionState
	DRepDistr DRepStakeDistrResult
	DRepState map[StakeCredential]DRepStateEntry
	PoolDistr map[ledger.Blake2b224]uint64
}

// DRepPulsingStateResult is cardano-ledger's DRepPulsingState in its
// completed form, which is how the node always encodes it.
type DRepPulsingStateResult struct {
	Snapshot    PulsingSnapshot
	RatifyState RatifyStateResult
}

// DecodeDRepPulsingState decodes the DRepPulsingState field.
func (g *GovStateResult) DecodeDRepPulsingState() (*DRepPulsingStateResult, error) {
	var tmp struct {
		cbor.StructAsArray
		Snapshot    pulsingSnapshotWire
		RatifyState RatifyStateResult
	}
	if _, err := cbor.Decode(g.DRepPulsingState, &tmp); err != nil {
		return nil, err
	}
	return &DRepPulsingStateResult{
		Snapshot:    PulsingSnapshot(tmp.Snapshot),
		RatifyState: tmp.RatifyState,
	}, nil
}

type pulsingSnapshotWire PulsingSnapshot

func (s *pulsingSnapshotWire) UnmarshalCBOR(data []byte) error {
	var tmp struct {
		cbor.StructAsArray
		Proposals growingArray[GovActionState]
		DRepDistr cbor.RawMessage
		DRepState growingMap[StakeCredential, DRepStateEntry]
		PoolDistr growingMap[ledger.Blake2b224, uint64]
	}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	// The DRep distribution decoder expects the single-element wrapper the
	// query reply carries, which a snapshot field does not have.
	var distr DRepStakeDistrResult
	wrapped := append([]byte{0x81}, tmp.DRepDistr...)
	if _, err := cbor.Decode(wrapped, &distr); err != nil {
		return err
	}
	*s = pulsingSnapshotWire{
		Proposals: tmp.Proposals,
		DRepDistr: distr,
		DRepState: tmp.DRepState,
		PoolDistr: tmp.PoolDistr,
	}
	return nil
}
