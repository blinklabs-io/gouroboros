// Copyright 2025 Blink Labs Software
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
	"math"
	"math/big"
	"slices"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/btcsuite/btcd/btcutil/bech32"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
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

func (b Blake2b256) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

func (b Blake2b256) MarshalCBOR() ([]byte, error) {
	// Ensure we always encode a full-sized bytestring, even if the hash is zero-valued
	hashBytes := make([]byte, Blake2b256Size)
	copy(hashBytes, b[:])
	return cbor.Encode(hashBytes)
}

func (b Blake2b256) Bech32(prefix string) string {
	// Convert data to base32 and encode as bech32
	convData, err := bech32.ConvertBits(b[:], 8, 5, true)
	if err != nil {
		panic(
			fmt.Sprintf("unexpected error converting data to base32: %s", err),
		)
	}
	encoded, err := bech32.Encode(prefix, convData)
	if err != nil {
		panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
	}
	return encoded
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

func (b Blake2b224) MarshalCBOR() ([]byte, error) {
	// Ensure we always encode a full-sized bytestring, even if the hash is zero-valued
	hashBytes := make([]byte, Blake2b224Size)
	copy(hashBytes, b[:])
	return cbor.Encode(hashBytes)
}

func (b Blake2b224) Bech32(prefix string) string {
	// Convert data to base32 and encode as bech32
	convData, err := bech32.ConvertBits(b[:], 8, 5, true)
	if err != nil {
		panic(
			fmt.Sprintf("unexpected error converting data to base32: %s", err),
		)
	}
	encoded, err := bech32.Encode(prefix, convData)
	if err != nil {
		panic(fmt.Sprintf("unexpected error encoding data as bech32: %s", err))
	}
	return encoded
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

// GenesisHash is a type alias for the Blake2b-224 hash used for genesis keys
type GenesisHash = Blake2b224

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

func (b Blake2b160) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.String())
}

func (b Blake2b160) MarshalCBOR() ([]byte, error) {
	// Ensure we always encode a full-sized bytestring, even if the hash is zero-valued
	hashBytes := make([]byte, Blake2b160Size)
	copy(hashBytes, b[:])
	return cbor.Encode(hashBytes)
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
	// The CBOR library is configured with SortCoreDeterministic, so direct encoding
	// of the map produces deterministic output without manual sorting
	return cbor.Encode(m.data)
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
	slices.SortFunc(
		policyKeys,
		func(a, b Blake2b224) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
	)
	for _, policyId := range policyKeys {
		policyData := m.data[policyId]
		tmpPolicyData := make([][2]data.PlutusData, 0, len(policyData))
		// Sort asset names
		assetKeys := slices.Collect(maps.Keys(policyData))
		slices.SortFunc(
			assetKeys,
			func(a, b cbor.ByteString) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
		)
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

// String returns a stable, human-friendly representation of the MultiAsset.
// Output format: [<policyId>.<assetNameHex>=<amount>, ...] sorted by policyId, then asset name
func (m *MultiAsset[T]) String() string {
	if m == nil {
		return "[]"
	}
	norm := m.normalize()
	if len(norm) == 0 {
		return "[]"
	}

	policies := slices.Collect(maps.Keys(norm))
	slices.SortFunc(
		policies,
		func(a, b Blake2b224) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
	)

	var b strings.Builder
	b.WriteByte('[')
	first := true
	for _, pid := range policies {
		assets := norm[pid]
		names := slices.Collect(maps.Keys(assets))
		slices.SortFunc(
			names,
			func(a, b cbor.ByteString) int { return bytes.Compare(a.Bytes(), b.Bytes()) },
		)

		for _, name := range names {
			if !first {
				b.WriteString(", ")
			}
			first = false
			b.WriteString(pid.String())
			b.WriteByte('.')
			b.WriteString(hex.EncodeToString(name.Bytes()))
			b.WriteByte('=')
			fmt.Fprintf(&b, "%d", assets[name])
		}
	}
	b.WriteByte(']')
	return b.String()
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
	return Blake2b224(p).Bech32("pool")
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
	Memory int64
	Steps  int64
}

// GenesisRat is a convenience type for cbor.Rat
type GenesisRat = cbor.Rat

// ToUtxorpcBigInt converts a uint64 into a *utxorpc.BigInt pointer
func ToUtxorpcBigInt(v uint64) *utxorpc.BigInt {
	if v <= math.MaxInt64 {
		return &utxorpc.BigInt{
			BigInt: &utxorpc.BigInt_Int{Int: int64(v)},
		}
	}
	return &utxorpc.BigInt{
		BigInt: &utxorpc.BigInt_BigUInt{
			BigUInt: new(big.Int).SetUint64(v).Bytes(),
		},
	}
}

// ExtractAndSetTransactionCbor extracts raw CBOR bytes for transaction bodies and witness sets
// from a block's CBOR data and calls the provided setters to store them. This preserves original
// encoding for correct transaction size calculations when re-serialized.
//
// The setter functions receive the index and the raw CBOR bytes to store.
// expectedBodies and expectedWitnesses provide bounds checking to prevent panics.
func ExtractAndSetTransactionCbor(
	cborData []byte,
	setBodyCbor func(index int, data []byte),
	setWitnessCbor func(index int, data []byte),
	expectedBodies int,
	expectedWitnesses int,
) error {
	var blockArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &blockArray); err != nil {
		return err
	}
	if len(blockArray) < 3 {
		return nil // Block doesn't have separated components
	}

	// Extract and store body CBOR
	var txBodiesRaw []cbor.RawMessage
	if _, err := cbor.Decode([]byte(blockArray[1]), &txBodiesRaw); err != nil {
		return fmt.Errorf(
			"failed to extract transaction bodies from block: %w",
			err,
		)
	}
	if len(txBodiesRaw) != expectedBodies {
		return fmt.Errorf(
			"transaction body count mismatch: expected %d, got %d",
			expectedBodies, len(txBodiesRaw),
		)
	}
	for i, rawBody := range txBodiesRaw {
		setBodyCbor(i, []byte(rawBody))
	}

	// Extract and store witness set CBOR
	var txWitnessSetsRaw []cbor.RawMessage
	if _, err := cbor.Decode([]byte(blockArray[2]), &txWitnessSetsRaw); err != nil {
		return fmt.Errorf(
			"failed to extract transaction witnesses from block: %w",
			err,
		)
	}
	if len(txWitnessSetsRaw) != expectedWitnesses {
		return fmt.Errorf(
			"transaction witness set count mismatch: expected %d, got %d",
			expectedWitnesses, len(txWitnessSetsRaw),
		)
	}
	for i, rawWitness := range txWitnessSetsRaw {
		setWitnessCbor(i, []byte(rawWitness))
	}

	return nil
}

type MissingTransactionMetadataError struct {
	Hash Blake2b256
}

func (e MissingTransactionMetadataError) Error() string {
	return fmt.Sprintf(
		"missing transaction metadata: body declares hash %x but no metadata provided",
		e.Hash,
	)
}

type MissingTransactionAuxiliaryDataHashError struct {
	Hash Blake2b256
}

func (e MissingTransactionAuxiliaryDataHashError) Error() string {
	return fmt.Sprintf(
		"missing transaction auxiliary data hash: metadata provided with hash %x but body has no hash",
		e.Hash,
	)
}

type ConflictingMetadataHashError struct {
	Supplied Blake2b256
	Expected Blake2b256
}

func (e ConflictingMetadataHashError) Error() string {
	return fmt.Sprintf(
		"conflicting metadata hash: body declares %x, actual metadata hash is %x",
		e.Supplied,
		e.Expected,
	)
}

// NewGenesisRat creates a GenesisRat from numerator and denominator.
// This is a helper for tests that need to construct genesis ratio values.
func NewGenesisRat(num, denom int64) GenesisRat {
	if denom == 0 {
		panic("denominator cannot be zero")
	}
	var r GenesisRat
	jsonData := fmt.Sprintf(`{"numerator": %d, "denominator": %d}`, num, denom)

	if err := r.UnmarshalJSON([]byte(jsonData)); err != nil {
		panic(fmt.Sprintf("failed to unmarshal GenesisRat JSON: %v", err))
	}
	return r
}
