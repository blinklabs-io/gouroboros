// Copyright 2024 Blink Labs Software
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
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"math/big"
	"slices"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/pkg/data"
	"github.com/btcsuite/btcd/btcutil/bech32"
	"golang.org/x/crypto/blake2b"
)

const (
	Blake2b256Size = 32
	Blake2b224Size = 28
	Blake2b160Size = 20
)

type Blake2b256 [Blake2b256Size]byte

func NewBlake2b256(data []byte) Blake2b256 {
	b := Blake2b256{}
	copy(b[:], data)
	return b
}

func (b Blake2b256) String() string {
	return hex.EncodeToString(b[:])
}

func (b Blake2b256) Bytes() []byte {
	return b[:]
}

func (b Blake2b256) ToPlutusData() data.PlutusData {
	return data.NewByteString(b[:])
}

// Blake2b256Hash generates a Blake2b-256 hash from the provided data
func Blake2b256Hash(data []byte) Blake2b256 {
	tmpHash, err := blake2b.New(Blake2b256Size, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error generating empty blake2b hash: %s",
				err,
			),
		)
	}
	tmpHash.Write(data)
	return Blake2b256(tmpHash.Sum(nil))
}

type Blake2b224 [Blake2b224Size]byte

func NewBlake2b224(data []byte) Blake2b224 {
	b := Blake2b224{}
	copy(b[:], data)
	return b
}

func (b Blake2b224) String() string {
	return hex.EncodeToString(b[:])
}

func (b Blake2b224) Bytes() []byte {
	return b[:]
}

func (b Blake2b224) ToPlutusData() data.PlutusData {
	return data.NewByteString(b[:])
}

func (b Blake2b224) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

// Blake2b224Hash generates a Blake2b-224 hash from the provided data
func Blake2b224Hash(data []byte) Blake2b224 {
	tmpHash, err := blake2b.New(Blake2b224Size, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error generating empty blake2b hash: %s",
				err,
			),
		)
	}
	tmpHash.Write(data)
	return Blake2b224(tmpHash.Sum(nil))
}

type Blake2b160 [Blake2b160Size]byte

func NewBlake2b160(data []byte) Blake2b160 {
	b := Blake2b160{}
	copy(b[:], data)
	return b
}

func (b Blake2b160) String() string {
	return hex.EncodeToString(b[:])
}

func (b Blake2b160) Bytes() []byte {
	return b[:]
}

func (b Blake2b160) ToPlutusData() data.PlutusData {
	return data.NewByteString(b[:])
}

// Blake2b160Hash generates a Blake2b-160 hash from the provided data
func Blake2b160Hash(data []byte) Blake2b160 {
	tmpHash, err := blake2b.New(Blake2b160Size, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error generating empty blake2b hash: %s",
				err,
			),
		)
	}
	tmpHash.Write(data)
	return Blake2b160(tmpHash.Sum(nil))
}

type (
	MultiAssetTypeOutput = uint64
	MultiAssetTypeMint   = int64
)

// MultiAsset represents a collection of policies, assets, and quantities. It's used for
// TX outputs (uint64) and TX asset minting (int64 to allow for negative values for burning)
type MultiAsset[T MultiAssetTypeOutput | MultiAssetTypeMint] struct {
	data map[Blake2b224]map[cbor.ByteString]T
}

// NewMultiAsset creates a MultiAsset with the specified data
func NewMultiAsset[T MultiAssetTypeOutput | MultiAssetTypeMint](
	data map[Blake2b224]map[cbor.ByteString]T,
) MultiAsset[T] {
	if data == nil {
		data = make(map[Blake2b224]map[cbor.ByteString]T)
	}
	return MultiAsset[T]{data: data}
}

// multiAssetJson is a convenience type for marshaling/unmarshaling MultiAsset to/from JSON
type multiAssetJson[T MultiAssetTypeOutput | MultiAssetTypeMint] struct {
	Name        string `json:"name"`
	NameHex     string `json:"nameHex"`
	PolicyId    string `json:"policyId"`
	Fingerprint string `json:"fingerprint"`
	Amount      T      `json:"amount"`
}

func (m *MultiAsset[T]) UnmarshalCBOR(data []byte) error {
	_, err := cbor.Decode(data, &(m.data))
	return err
}

func (m *MultiAsset[T]) MarshalCBOR() ([]byte, error) {
	return cbor.Encode(&(m.data))
}

func (m MultiAsset[T]) MarshalJSON() ([]byte, error) {
	tmpAssets := []multiAssetJson[T]{}
	for policyId, policyData := range m.data {
		for assetName, amount := range policyData {
			tmpObj := multiAssetJson[T]{
				Name:     string(assetName.Bytes()),
				NameHex:  hex.EncodeToString(assetName.Bytes()),
				Amount:   amount,
				PolicyId: policyId.String(),
				Fingerprint: NewAssetFingerprint(
					policyId.Bytes(),
					assetName.Bytes(),
				).String(),
			}
			tmpAssets = append(tmpAssets, tmpObj)
		}
	}
	return json.Marshal(&tmpAssets)
}

func (m *MultiAsset[T]) ToPlutusData() data.PlutusData {
	tmpData := make([][2]data.PlutusData, 0, len(m.data))
	// Sort policy IDs
	policyKeys := slices.Collect(maps.Keys(m.data))
	slices.SortFunc(policyKeys, func(a, b Blake2b224) int { return bytes.Compare(a.Bytes(), b.Bytes()) })
	for _, policyId := range policyKeys {
		policyData := m.data[policyId]
		tmpPolicyData := make([][2]data.PlutusData, 0, len(policyData))
		// Sort asset names
		assetKeys := slices.Collect(maps.Keys(policyData))
		slices.SortFunc(assetKeys, func(a, b cbor.ByteString) int { return bytes.Compare(a.Bytes(), b.Bytes()) })
		for _, assetName := range assetKeys {
			amount := policyData[assetName]
			tmpPolicyData = append(
				tmpPolicyData,
				[2]data.PlutusData{
					data.NewByteString(assetName.Bytes()),
					data.NewInteger(big.NewInt(int64(amount))),
				},
			)
		}
		tmpData = append(
			tmpData,
			[2]data.PlutusData{
				data.NewByteString(policyId.Bytes()),
				data.NewMap(tmpPolicyData),
			},
		)
	}
	return data.NewMap(tmpData)
}

func (m *MultiAsset[T]) Policies() []Blake2b224 {
	ret := []Blake2b224{}
	for policyId := range m.data {
		ret = append(ret, policyId)
	}
	return ret
}

func (m *MultiAsset[T]) Assets(policyId Blake2b224) [][]byte {
	assets, ok := m.data[policyId]
	if !ok {
		return nil
	}
	ret := [][]byte{}
	for assetName := range assets {
		ret = append(ret, assetName.Bytes())
	}
	return ret
}

func (m *MultiAsset[T]) Asset(policyId Blake2b224, assetName []byte) T {
	policy, ok := m.data[policyId]
	if !ok {
		return 0
	}
	return policy[cbor.NewByteString(assetName)]
}

func (m *MultiAsset[T]) Add(assets *MultiAsset[T]) {
	if assets == nil {
		return
	}
	for policy, assets := range assets.data {
		for asset, amount := range assets {
			newAmount := m.Asset(policy, asset.Bytes()) + amount
			if _, ok := m.data[policy]; !ok {
				m.data[policy] = make(map[cbor.ByteString]T)
			}
			m.data[policy][asset] = newAmount
		}
	}
}

func (m *MultiAsset[T]) Compare(assets *MultiAsset[T]) bool {
	// Normalize data for easier comparison
	tmpData := m.normalize()
	otherData := assets.normalize()
	// Compare policy counts
	if len(otherData) != len(tmpData) {
		return false
	}
	for policy, assets := range otherData {
		// Compare asset counts for policy
		if len(assets) != len(tmpData[policy]) {
			return false
		}
		for asset, amount := range assets {
			// Compare quantity of specific asset
			if amount != m.Asset(policy, asset.Bytes()) {
				return false
			}
		}
	}
	return true
}

func (m *MultiAsset[T]) normalize() map[Blake2b224]map[cbor.ByteString]T {
	ret := map[Blake2b224]map[cbor.ByteString]T{}
	if m == nil || m.data == nil {
		return ret
	}
	for policy, assets := range m.data {
		for asset, amount := range assets {
			if amount != 0 {
				if _, ok := ret[policy]; !ok {
					ret[policy] = make(map[cbor.ByteString]T)
				}
				ret[policy][asset] = amount
			}
		}
	}
	return ret
}

type AssetFingerprint struct {
	policyId  []byte
	assetName []byte
}

func NewAssetFingerprint(policyId []byte, assetName []byte) AssetFingerprint {
	return AssetFingerprint{
		policyId:  policyId,
		assetName: assetName,
	}
}

func (a AssetFingerprint) Hash() Blake2b160 {
	tmpHash, err := blake2b.New(20, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error creating empty blake2b hash: %s",
				err,
			),
		)
	}
	tmpHash.Write(a.policyId)
	tmpHash.Write(a.assetName)
	return NewBlake2b160(tmpHash.Sum(nil))
}

func (a AssetFingerprint) String() string {
	// Convert data to base32 and encode as bech32
	convData, err := bech32.ConvertBits(a.Hash().Bytes(), 8, 5, true)
	if err != nil {
		panic(
			fmt.Sprintf("unexpected error converting data to base32: %s", err),
		)
	}
	encoded, err := bech32.Encode("asset", convData)
	if err != nil {
		panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
	}
	return encoded
}

type PoolId [28]byte

func NewPoolIdFromBech32(poolId string) (PoolId, error) {
	var p PoolId
	_, data, err := bech32.DecodeNoLimit(poolId)
	if err != nil {
		return p, err
	}
	decoded, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return p, err
	}
	if len(decoded) != len(p) {
		return p, fmt.Errorf("invalid pool ID length: %d", len(decoded))
	}
	p = PoolId(decoded)
	return p, err
}

func (p PoolId) String() string {
	// Convert data to base32 and encode as bech32
	convData, err := bech32.ConvertBits(p[:], 8, 5, true)
	if err != nil {
		panic(
			fmt.Sprintf("unexpected error converting data to base32: %s", err),
		)
	}
	encoded, err := bech32.Encode("pool", convData)
	if err != nil {
		panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
	}
	return encoded
}

// IssuerVkey represents the verification key for the stake pool that minted a block
type IssuerVkey [32]byte

func (i IssuerVkey) Hash() Blake2b224 {
	hash, err := blake2b.New(28, nil)
	if err != nil {
		panic(
			fmt.Sprintf(
				"unexpected error creating empty blake2b hash: %s",
				err,
			),
		)
	}
	hash.Write(i[:])
	return Blake2b224(hash.Sum(nil))
}

func (i IssuerVkey) PoolId() string {
	// Convert data to base32 and encode as bech32
	convData, err := bech32.ConvertBits(i.Hash().Bytes(), 8, 5, true)
	if err != nil {
		panic(
			fmt.Sprintf("unexpected error converting data to base32: %s", err),
		)
	}
	encoded, err := bech32.Encode("pool", convData)
	if err != nil {
		panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
	}
	return encoded
}

// ExUnits represents the steps and memory usage for script execution
type ExUnits struct {
	cbor.StructAsArray
	Memory uint64
	Steps  uint64
}
