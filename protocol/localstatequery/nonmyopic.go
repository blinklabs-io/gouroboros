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
	"encoding/binary"
	"errors"
	"fmt"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/internal/cborpreflight"
)

const (
	// stakeCredentialKeyHashTag and stakeCredentialScriptHashTag are the
	// wire tags of a staking credential: KeyHashObj is 0, ScriptHashObj is 1.
	stakeCredentialKeyHashTag    = 0
	stakeCredentialScriptHashTag = 1

	nonMyopicLeftTag  = 0
	nonMyopicRightTag = 1

	maxNonMyopicMemberRewardsItems = 131_072
)

// NonMyopicMemberRewardsKey is a key of the non-myopic member rewards query,
// an Either of a coin amount or a staking credential. A coin key is encoded
// as [0, coin] and a credential key as [1, [tag, hash]].
type NonMyopicMemberRewardsKey struct {
	// IsCredential selects Credential; otherwise Coin is the key.
	IsCredential bool
	Coin         uint64
	Credential   StakeCredential
}

// NewNonMyopicCoinKey returns the key for a stake amount.
func NewNonMyopicCoinKey(coin uint64) NonMyopicMemberRewardsKey {
	return NonMyopicMemberRewardsKey{Coin: coin}
}

// NewNonMyopicCredentialKey returns the key for a staking credential.
func NewNonMyopicCredentialKey(
	credential StakeCredential,
) NonMyopicMemberRewardsKey {
	return NonMyopicMemberRewardsKey{
		IsCredential: true,
		Credential:   credential,
	}
}

// stakeCredentialRank orders credentials as the ledger's derived Ord does,
// which declares ScriptHashObj before KeyHashObj, the reverse of the wire
// tags.
func stakeCredentialRank(tag uint64) int {
	if tag == stakeCredentialScriptHashTag {
		return 0
	}
	return 1
}

// Compare orders keys as the ledger orders Either Coin (Credential Staking):
// coins before credentials, coins numerically, script credentials before key
// credentials, then hashes bytewise. The node requires this order of the
// query set and writes the result map in it.
func (k NonMyopicMemberRewardsKey) Compare(
	other NonMyopicMemberRewardsKey,
) int {
	if k.IsCredential != other.IsCredential {
		if k.IsCredential {
			return 1
		}
		return -1
	}
	if !k.IsCredential {
		return cmp.Compare(k.Coin, other.Coin)
	}
	if c := cmp.Compare(
		stakeCredentialRank(k.Credential.Tag),
		stakeCredentialRank(other.Credential.Tag),
	); c != 0 {
		return c
	}
	return bytes.Compare(
		k.Credential.Bytes.Bytes(),
		other.Credential.Bytes.Bytes(),
	)
}

func (k NonMyopicMemberRewardsKey) MarshalCBOR() ([]byte, error) {
	if !k.IsCredential {
		return cbor.Encode([]any{nonMyopicLeftTag, k.Coin})
	}
	if k.Credential.Tag != stakeCredentialKeyHashTag &&
		k.Credential.Tag != stakeCredentialScriptHashTag {
		return nil, fmt.Errorf(
			"non-myopic member rewards key: unknown credential tag %d",
			k.Credential.Tag,
		)
	}
	return cbor.Encode([]any{
		nonMyopicRightTag,
		[]any{k.Credential.Tag, k.Credential.Bytes},
	})
}

func (k *NonMyopicMemberRewardsKey) UnmarshalCBOR(data []byte) error {
	items, err := cborpreflight.ArrayItems(
		data,
		2,
		"non-myopic member rewards key",
	)
	if err != nil {
		return err
	}
	if len(items) != 2 {
		return fmt.Errorf(
			"non-myopic member rewards key: expected 2 items, got %d",
			len(items),
		)
	}
	var tag uint64
	if _, err := cbor.Decode(items[0], &tag); err != nil {
		return fmt.Errorf("non-myopic member rewards key tag: %w", err)
	}
	switch tag {
	case nonMyopicLeftTag:
		var coin uint64
		if _, err := cbor.Decode(items[1], &coin); err != nil {
			return fmt.Errorf("non-myopic member rewards coin: %w", err)
		}
		*k = NewNonMyopicCoinKey(coin)
	case nonMyopicRightTag:
		credential, err := decodeStakeCredential(items[1])
		if err != nil {
			return err
		}
		*k = NewNonMyopicCredentialKey(credential)
	default:
		return fmt.Errorf(
			"non-myopic member rewards key: unknown Either tag %d",
			tag,
		)
	}
	return nil
}

func decodeStakeCredential(data []byte) (StakeCredential, error) {
	items, err := cborpreflight.ArrayItems(data, 2, "staking credential")
	if err != nil {
		return StakeCredential{}, err
	}
	if len(items) != 2 {
		return StakeCredential{}, fmt.Errorf(
			"staking credential: expected 2 items, got %d",
			len(items),
		)
	}
	var credential StakeCredential
	if _, err := cbor.Decode(items[0], &credential.Tag); err != nil {
		return StakeCredential{}, fmt.Errorf("staking credential tag: %w", err)
	}
	if credential.Tag != stakeCredentialKeyHashTag &&
		credential.Tag != stakeCredentialScriptHashTag {
		return StakeCredential{}, fmt.Errorf(
			"staking credential: unknown tag %d",
			credential.Tag,
		)
	}
	if _, err := cbor.Decode(items[1], &credential.Bytes); err != nil {
		return StakeCredential{}, err
	}
	return credential, nil
}

// appendMapHeader writes a definite-length map header of n entries.
func appendMapHeader(dst []byte, n int) []byte {
	switch {
	case n < 24:
		//nolint:gosec // n is below 24
		return append(dst, 0xa0|byte(n))
	case n <= 0xff:
		return append(dst, 0xb8, byte(n))
	case n <= 0xffff:
		return binary.BigEndian.AppendUint16(append(dst, 0xb9), uint16(n))
	default:
		//nolint:gosec // a map this large cannot be held in memory
		return binary.BigEndian.AppendUint32(append(dst, 0xba), uint32(n))
	}
}

// MarshalCBOR writes the map as the plain map encoder does: a
// definite-length map in ascending key order, whatever the size, with each
// pool map in ascending pool hash order.
func (r NonMyopicMemberRewardsResult) MarshalCBOR() ([]byte, error) {
	keys := make([]NonMyopicMemberRewardsKey, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, NonMyopicMemberRewardsKey.Compare)
	out := appendMapHeader(nil, len(keys))
	for _, key := range keys {
		encodedKey, err := key.MarshalCBOR()
		if err != nil {
			return nil, err
		}
		out = append(out, encodedKey...)
		pools := r[key]
		poolIds := make([]ledger.Blake2b224, 0, len(pools))
		for poolId := range pools {
			poolIds = append(poolIds, poolId)
		}
		slices.SortFunc(poolIds, func(a, b ledger.Blake2b224) int {
			return bytes.Compare(a.Bytes(), b.Bytes())
		})
		out = appendMapHeader(out, len(poolIds))
		for _, poolId := range poolIds {
			encodedPool, err := cbor.Encode(poolId)
			if err != nil {
				return nil, err
			}
			encodedReward, err := cbor.Encode(pools[poolId])
			if err != nil {
				return nil, err
			}
			out = append(out, encodedPool...)
			out = append(out, encodedReward...)
		}
	}
	return out, nil
}

// UnmarshalCBOR reads the map in either length form: the plain map encoder
// writes a definite length, while the ledger's era encoder writes an
// indefinite-length map above 23 entries. Both write keys in ascending order,
// so keys must be strictly ascending, which also rejects a repeated key. The
// pool maps are held to the same rules.
func (r *NonMyopicMemberRewardsResult) UnmarshalCBOR(data []byte) error {
	result := make(NonMyopicMemberRewardsResult)
	var previous NonMyopicMemberRewardsKey
	err := cborpreflight.ValidateMap(
		data,
		maxNonMyopicMemberRewardsItems,
		"non-myopic member rewards",
		func(idx int, rawKey, rawValue []byte) error {
			var key NonMyopicMemberRewardsKey
			if err := key.UnmarshalCBOR(rawKey); err != nil {
				return err
			}
			if idx > 0 && key.Compare(previous) <= 0 {
				return errors.New(
					"non-myopic member rewards: keys not strictly ascending",
				)
			}
			previous = key
			pools := make(map[ledger.Blake2b224]uint64)
			var previousPool ledger.Blake2b224
			if err := cborpreflight.ValidateMap(
				rawValue,
				maxNonMyopicMemberRewardsItems,
				"non-myopic member rewards pools",
				func(poolIdx int, rawPool, rawReward []byte) error {
					var poolId ledger.Blake2b224
					if _, err := cbor.Decode(rawPool, &poolId); err != nil {
						return err
					}
					if poolIdx > 0 &&
						bytes.Compare(
							poolId.Bytes(),
							previousPool.Bytes(),
						) <= 0 {
						return errors.New(
							"non-myopic member rewards: pools not strictly ascending",
						)
					}
					previousPool = poolId
					var reward uint64
					if _, err := cbor.Decode(rawReward, &reward); err != nil {
						return err
					}
					pools[poolId] = reward
					return nil
				},
			); err != nil {
				return err
			}
			result[key] = pools
			return nil
		},
	)
	if err != nil {
		return err
	}
	*r = result
	return nil
}

// encodeNonMyopicMemberRewardsSet encodes the query argument, a Set of
// keys: tag 258 around a definite-length array in strictly ascending key
// order, which is what the node's set decoder requires.
func encodeNonMyopicMemberRewardsSet(
	keys []NonMyopicMemberRewardsKey,
) ([]byte, error) {
	if len(keys) > maxLocalStateQuerySetItems {
		return nil, fmt.Errorf(
			"stakes contains %d items; maximum is %d",
			len(keys),
			maxLocalStateQuerySetItems,
		)
	}
	sorted := slices.Clone(keys)
	slices.SortFunc(sorted, NonMyopicMemberRewardsKey.Compare)
	sorted = slices.CompactFunc(
		sorted,
		func(a, b NonMyopicMemberRewardsKey) bool {
			return a.Compare(b) == 0
		},
	)
	members := make([]cbor.RawMessage, 0, len(sorted))
	for _, key := range sorted {
		encoded, err := key.MarshalCBOR()
		if err != nil {
			return nil, err
		}
		members = append(members, encoded)
	}
	array, err := cbor.Encode(members)
	if err != nil {
		return nil, err
	}
	return cbor.Encode(cbor.RawTag{Number: cbor.CborTagSet, Content: array})
}
