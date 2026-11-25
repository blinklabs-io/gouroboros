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
	"errors"
	"fmt"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborwalk"
)

// ShelleyStakePoolDefaultVoteQuery is QueryStakePoolDefaultVote (Shelley
// query 35): the vote a pool's reward account delegation implies on
// governance actions it does not vote on. Wire form: [35, pool_keyhash].
type ShelleyStakePoolDefaultVoteQuery struct {
	cbor.StructAsArray
	Type   int
	PoolId ledger.PoolId
}

// DefaultVote is the ledger's DefaultVote.
type DefaultVote uint8

const (
	DefaultVoteNo           DefaultVote = 0
	DefaultVoteAbstain      DefaultVote = 1
	DefaultVoteNoConfidence DefaultVote = 2
)

func (v *DefaultVote) UnmarshalCBOR(data []byte) error {
	var tmp uint8
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	if tmp > uint8(DefaultVoteNoConfidence) {
		return fmt.Errorf("invalid DefaultVote tag %d", tmp)
	}
	*v = DefaultVote(tmp)
	return nil
}

// StakePoolDefaultVoteResult is the QueryStakePoolDefaultVote reply inside
// the single-element era result wrapper.
type StakePoolDefaultVoteResult struct {
	cbor.StructAsArray
	Vote DefaultVote
}

// ShelleyStakeDistribution2Query is GetStakeDistribution2 (Shelley query 37),
// which replaces GetStakeDistribution from node-to-client version 21. Wire
// form: [37].
type ShelleyStakeDistribution2Query struct {
	simpleQueryBase
}

// StakeDistribution2Result is the GetStakeDistribution2 reply, the ledger's
// PoolDistr inside the single-element era result wrapper. A GetPoolDistr2
// reply has the same form.
type StakeDistribution2Result struct {
	cbor.StructAsArray
	Distribution PoolDistribution
}

// PoolDistribution is the ledger's PoolDistr: each pool's stake and the total
// active stake.
type PoolDistribution struct {
	cbor.StructAsArray
	Pools            map[ledger.PoolId]IndividualPoolStake
	TotalActiveStake uint64
}

func (d *PoolDistribution) UnmarshalCBOR(data []byte) error {
	var tmp struct {
		cbor.StructAsArray
		Pools            growingMap[ledger.PoolId, IndividualPoolStake]
		TotalActiveStake uint64
	}
	if _, err := cbor.Decode(data, &tmp); err != nil {
		return err
	}
	*d = PoolDistribution{
		Pools:            tmp.Pools,
		TotalActiveStake: tmp.TotalActiveStake,
	}
	return nil
}

// IndividualPoolStake is the ledger's IndividualPoolStake. BlsKey holds the
// CBOR of the pool's registered BLS key when the entry carries one, and is
// nil otherwise.
type IndividualPoolStake struct {
	StakeFraction  *cbor.Rat
	TotalPoolStake uint64
	VrfHash        ledger.Blake2b256
	BlsKey         cbor.RawMessage
}

func (s *IndividualPoolStake) UnmarshalCBOR(data []byte) error {
	fields, err := decodeBoundedArray(data, 4, "IndividualPoolStake")
	if err != nil {
		return err
	}
	if len(fields) != 3 && len(fields) != 4 {
		return fmt.Errorf(
			"IndividualPoolStake: expected 3 or 4 fields, got %d",
			len(fields),
		)
	}
	stake, err := decodeRational(fields[0])
	if err != nil {
		return fmt.Errorf("IndividualPoolStake: %w", err)
	}
	var tmp IndividualPoolStake
	tmp.StakeFraction = &cbor.Rat{Rat: stake}
	if _, err := cbor.Decode(fields[1], &tmp.TotalPoolStake); err != nil {
		return fmt.Errorf("IndividualPoolStake: total stake: %w", err)
	}
	if _, err := cbor.Decode(fields[2], &tmp.VrfHash); err != nil {
		return fmt.Errorf("IndividualPoolStake: VRF hash: %w", err)
	}
	// The fourth field is a StrictMaybe BlsKey, a list of at most one key.
	if len(fields) == 4 {
		maybe, err := decodeBoundedArray(fields[3], 1, "BLS key")
		if err != nil {
			return fmt.Errorf("IndividualPoolStake: BLS key: %w", err)
		}
		switch len(maybe) {
		case 0:
		case 1:
			tmp.BlsKey = maybe[0]
		default:
			return fmt.Errorf(
				"IndividualPoolStake: BLS key: expected at most 1 value, got %d",
				len(maybe),
			)
		}
	}
	*s = tmp
	return nil
}

// MarshalCBOR writes three fields, or four when BlsKey is set, as the ledger
// does for IndividualPoolStake.
func (s IndividualPoolStake) MarshalCBOR() ([]byte, error) {
	if s.StakeFraction == nil || s.StakeFraction.Rat == nil {
		return nil, errors.New("IndividualPoolStake: nil stake fraction")
	}
	fields := []any{s.StakeFraction, s.TotalPoolStake, s.VrfHash}
	if s.BlsKey != nil {
		fields = append(fields, []cbor.RawMessage{s.BlsKey})
	}
	return cbor.Encode(fields)
}

// ShelleyMaxMajorProtocolVersionQuery is GetMaxMajorProtocolVersion (Shelley
// query 38): the highest major protocol version the node can follow. Wire
// form: [38].
type ShelleyMaxMajorProtocolVersionQuery struct {
	simpleQueryBase
}

// MaxMajorProtocolVersionResult is the GetMaxMajorProtocolVersion reply
// inside the single-element era result wrapper.
type MaxMajorProtocolVersionResult struct {
	cbor.StructAsArray
	Version uint64
}

// ShelleyDRepDelegationsQuery is GetDRepDelegations (Shelley query 39),
// added in node-to-client version 23: the stake credentials delegated to each
// DRep in the filter. Wire form: [39, set<drep>]; an empty set asks about
// every DRep.
type ShelleyDRepDelegationsQuery struct {
	cbor.StructAsArray
	Type  int
	DReps cbor.SetType[lcommon.Drep]
}

// DRepDelegationsEntry is one DRep and the stake credentials delegated to it.
type DRepDelegationsEntry struct {
	Drep       lcommon.Drep
	Delegators []lcommon.Credential
}

// DRepDelegationsResult is the GetDRepDelegations reply, the ledger's
// Map DRep (Set (Credential Staking)) inside the single-element era result
// wrapper. It is a slice for the reason given on [DRepStakeDistrResult], and
// keeps the wire order.
type DRepDelegationsResult []DRepDelegationsEntry

func (r *DRepDelegationsResult) UnmarshalCBOR(data []byte) error {
	entries := DRepDelegationsResult{}
	err := decodeDRepKeyedResult(
		data,
		"DRep delegations",
		func(dec *cbor.StreamDecoder, drep lcommon.Drep) error {
			delegators, err := nextCredentialSet(dec)
			if err != nil {
				return fmt.Errorf(
					"DRep delegations: decoding delegators: %w",
					err,
				)
			}
			entries = append(entries, DRepDelegationsEntry{
				Drep:       drep,
				Delegators: delegators,
			})
			return nil
		},
	)
	if err != nil {
		return err
	}
	*r = entries
	return nil
}

// nextCredentialSet decodes the credential set at dec's position and advances
// past it. The set is measured and read in place: skipping it through the
// stream decoder would first copy the whole set into its read buffer.
func nextCredentialSet(dec *cbor.StreamDecoder) ([]lcommon.Credential, error) {
	start := dec.Position()
	length, err := cborwalk.ItemLength(dec.Data()[start:])
	if err != nil {
		return nil, err
	}
	if err := dec.Advance(length); err != nil {
		return nil, err
	}
	return decodeCredentialSet(dec.Data()[start : start+length])
}

// cborTagSelfDescribed is the self-described CBOR tag (RFC 8949 3.4.6).
const cborTagSelfDescribed = 55799

// decodeCredentialSet decodes a set of stake credentials, tagged 258 or
// untagged, reading data in place. It grows the result only with credentials
// that decode, where a cbor.SetType would size it from the peer-declared count
// first.
func decodeCredentialSet(data []byte) ([]lcommon.Credential, error) {
	content := data
	for {
		head, ok, err := cborwalk.ReadHead(content, 0)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errors.New("truncated credential set")
		}
		if head.Major != cbor.CborTypeTag>>5 {
			break
		}
		// The generic decoder ignores the self-described tag, so a set
		// carrying it was accepted before and still is.
		if head.Argument != cbor.CborTagSet &&
			head.Argument != cborTagSelfDescribed {
			return nil, fmt.Errorf("unexpected tag %d", head.Argument)
		}
		content = content[head.EncodedSize:]
	}
	if len(content) == 1 && (content[0] == 0xf6 || content[0] == 0xf7) {
		return nil, errors.New("credential set cannot be null or undefined")
	}
	var items growingArray[lcommon.Credential]
	if _, err := cbor.Decode(content, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func (r DRepDelegationsResult) MarshalCBOR() ([]byte, error) {
	entries := make([]drepKeyedValue, 0, len(r))
	for _, entry := range r {
		delegators := entry.Delegators
		if delegators == nil {
			delegators = []lcommon.Credential{}
		}
		set := cbor.NewSetType(delegators, true)
		value, err := cbor.Encode(&set)
		if err != nil {
			return nil, fmt.Errorf(
				"DRep delegations: encoding delegators: %w",
				err,
			)
		}
		entries = append(entries, drepKeyedValue{
			drep:  entry.Drep,
			value: value,
		})
	}
	return encodeDRepKeyedResult(entries, "DRep delegations")
}
