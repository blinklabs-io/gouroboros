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
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"slices"
	"strconv"
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
	MultiAssetTypeOutput = *big.Int
	MultiAssetTypeMint   = *big.Int
)

// MultiAsset represents a collection of policies, assets, and quantities. It's used for
// TX outputs (uint64) and TX asset minting (int64 to allow for negative values for burning)
type MultiAsset[T int64 | uint64 | *big.Int] struct {
	data map[Blake2b224]map[cbor.ByteString]T
}

// NewMultiAsset creates a MultiAsset with the specified data
func NewMultiAsset[T int64 | uint64 | *big.Int](
	data map[Blake2b224]map[cbor.ByteString]T,
) MultiAsset[T] {
	if data == nil {
		data = make(map[Blake2b224]map[cbor.ByteString]T)
	}
	return MultiAsset[T]{data: data}
}

// multiAssetJson is a convenience type for marshaling/unmarshaling MultiAsset to/from JSON
type multiAssetJson[T int64 | uint64 | *big.Int] struct {
	Name        string `json:"name"`
	NameHex     string `json:"nameHex"`
	PolicyId    string `json:"policyId"`
	Fingerprint string `json:"fingerprint"`
	Amount      string `json:"amount"`
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
	tmpAssets := make([]multiAssetJson[T], 0, len(m.data))
	for policyId, policyData := range m.data {
		for assetName, amount := range policyData {
			tmpObj := multiAssetJson[T]{
				Name:     string(assetName.Bytes()),
				NameHex:  hex.EncodeToString(assetName.Bytes()),
				Amount:   amountToString(amount),
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

func (m *MultiAsset[T]) UnmarshalJSON(data []byte) error {
	tmpAssets := []multiAssetJson[T]{}
	if err := json.Unmarshal(data, &tmpAssets); err != nil {
		return err
	}
	if m.data == nil {
		m.data = make(map[Blake2b224]map[cbor.ByteString]T)
	}
	for _, tmp := range tmpAssets {
		policyBytes, err := hex.DecodeString(tmp.PolicyId)
		if err != nil {
			return err
		}
		var policy Blake2b224
		copy(policy[:], policyBytes)
		nameBytes, err := hex.DecodeString(tmp.NameHex)
		if err != nil {
			return err
		}
		amount, err := parseAmount[T](tmp.Amount)
		if err != nil {
			return err
		}
		if _, ok := m.data[policy]; !ok {
			m.data[policy] = make(map[cbor.ByteString]T)
		}
		m.data[policy][cbor.NewByteString(nameBytes)] = amount
	}
	return nil
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
					data.NewInteger(amountToBigInt(amount)),
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
	ret := make([]Blake2b224, 0, len(m.data))
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
	ret := make([][]byte, 0, len(assets))
	for assetName := range assets {
		ret = append(ret, assetName.Bytes())
	}
	return ret
}

func (m *MultiAsset[T]) Asset(policyId Blake2b224, assetName []byte) T {
	policy, ok := m.data[policyId]
	if !ok {
		var zero T
		return zero
	}
	return policy[cbor.NewByteString(assetName)]
}

func (m *MultiAsset[T]) Add(assets *MultiAsset[T]) {
	if assets == nil {
		return
	}
	for policy, assets := range assets.data {
		for asset, amount := range assets {
			existing := m.Asset(policy, asset.Bytes())
			newAmount := addAmounts(existing, amount)
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
			if !amountsEqual(amount, m.Asset(policy, asset.Bytes())) {
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
			if !amountIsZero(amount) {
				if _, ok := ret[policy]; !ok {
					ret[policy] = make(map[cbor.ByteString]T)
				}
				// copy amount for big.Int to avoid aliasing
				switch v := any(amount).(type) {
				case *big.Int:
					ret[policy][asset] = any(new(big.Int).Set(v)).(T)
				default:
					ret[policy][asset] = amount
				}
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
			b.WriteString(amountToString(assets[name]))
		}
	}
	b.WriteByte(']')
	return b.String()
}

// Helper functions for generic amount handling

func addAmounts[T int64 | uint64 | *big.Int](a, b T) T {
	switch av := any(a).(type) {
	case *big.Int:
		var aInt, bInt *big.Int
		if av != nil {
			aInt = av
		} else {
			aInt = new(big.Int)
		}
		bv := any(b).(*big.Int)
		if bv != nil {
			bInt = bv
		} else {
			bInt = new(big.Int)
		}
		return any(new(big.Int).Add(aInt, bInt)).(T)
	case int64:
		return any(av + any(b).(int64)).(T)
	case uint64:
		return any(av + any(b).(uint64)).(T)
	default:
		var zero T
		return zero
	}
}

func amountsEqual[T int64 | uint64 | *big.Int](a, b T) bool {
	switch av := any(a).(type) {
	case *big.Int:
		bv := any(b).(*big.Int)
		if av == nil && bv == nil {
			return true
		}
		if av == nil {
			return bv.Sign() == 0
		}
		if bv == nil {
			return av.Sign() == 0
		}
		return av.Cmp(bv) == 0
	case int64:
		return av == any(b).(int64)
	case uint64:
		return av == any(b).(uint64)
	default:
		return false
	}
}

func amountIsZero[T int64 | uint64 | *big.Int](a T) bool {
	switch av := any(a).(type) {
	case *big.Int:
		if av == nil {
			return true
		}
		return av.Sign() == 0
	case int64:
		return av == 0
	case uint64:
		return av == 0
	default:
		return false
	}
}

func amountToString[T int64 | uint64 | *big.Int](a T) string {
	switch av := any(a).(type) {
	case *big.Int:
		if av == nil {
			return "0"
		}
		return av.String()
	case int64:
		return strconv.FormatInt(av, 10)
	case uint64:
		return strconv.FormatUint(av, 10)
	default:
		return "0"
	}
}

func amountToBigInt[T int64 | uint64 | *big.Int](a T) *big.Int {
	switch av := any(a).(type) {
	case *big.Int:
		if av == nil {
			return new(big.Int)
		}
		return new(big.Int).Set(av)
	case int64:
		return big.NewInt(av)
	case uint64:
		return new(big.Int).SetUint64(av)
	default:
		return new(big.Int)
	}
}

func parseAmount[T int64 | uint64 | *big.Int](s string) (T, error) {
	var zero T
	switch any(zero).(type) {
	case *big.Int:
		v, ok := new(big.Int).SetString(s, 10)
		if !ok {
			return zero, fmt.Errorf("invalid big.Int: %s", s)
		}
		return any(v).(T), nil
	case int64:
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return zero, err
		}
		return any(i).(T), nil
	case uint64:
		u, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return zero, err
		}
		return any(u).(T), nil
	default:
		return zero, errors.New("unsupported amount type")
	}
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

// BigIntToUtxorpcBigInt converts a *big.Int into a *utxorpc.BigInt pointer
func BigIntToUtxorpcBigInt(v *big.Int) *utxorpc.BigInt {
	if v == nil {
		return &utxorpc.BigInt{
			BigInt: &utxorpc.BigInt_Int{Int: 0},
		}
	}
	// If it fits in int64, use the compact representation
	if v.IsInt64() {
		return &utxorpc.BigInt{
			BigInt: &utxorpc.BigInt_Int{Int: v.Int64()},
		}
	}
	// Otherwise use the big int bytes representation
	return &utxorpc.BigInt{
		BigInt: &utxorpc.BigInt_BigUInt{
			BigUInt: v.Bytes(),
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

// ByteRange represents a byte offset and length within block CBOR.
type ByteRange struct {
	Offset uint32 // Byte offset within block CBOR
	Length uint32 // Length in bytes
}

// TransactionLocation represents the byte location of a transaction within block CBOR.
type TransactionLocation struct {
	Body     ByteRange // Location of transaction body within block CBOR
	Witness  ByteRange // Location of witness set within block CBOR
	Metadata ByteRange // Location of metadata within block CBOR (zero if no metadata)

	// Outputs contains byte locations for each transaction output (UTxO)
	// Index matches the output index within the transaction
	Outputs []ByteRange

	// Witness set component offsets (Alonzo+ era)
	// These map content hashes to their locations within the block

	// Datums maps datum hash to its byte location
	Datums map[Blake2b256]ByteRange

	// Redeemers maps redeemer key to its byte location
	Redeemers map[RedeemerKey]ByteRange

	// Scripts maps script hash to its byte location
	// Includes native scripts, Plutus V1/V2/V3 scripts
	Scripts map[ScriptHash]ByteRange
}

// BlockTransactionOffsets contains byte offset information for all transactions in a block.
type BlockTransactionOffsets struct {
	// Transactions maps transaction index to its location information
	Transactions []TransactionLocation
}

// ExtractTransactionOffsets extracts byte offsets for all transactions in a block.
// This enables efficient CBOR extraction from block data without full deserialization.
//
// The function parses the block CBOR structure to find where each transaction body,
// witness set, and metadata starts and ends within the raw block bytes.
func ExtractTransactionOffsets(cborData []byte) (*BlockTransactionOffsets, error) {
	// First pass: decode block as array of RawMessages to get component boundaries
	var blockArray []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &blockArray); err != nil {
		return nil, fmt.Errorf("failed to decode block array: %w", err)
	}
	if len(blockArray) < 3 {
		// Block doesn't have separated components (e.g., Byron EBB)
		// Return empty slice instead of nil to prevent nil pointer dereference
		return &BlockTransactionOffsets{Transactions: []TransactionLocation{}}, nil
	}

	// Calculate header size by finding where blockArray[0] starts
	// CBOR array header is 1 byte for arrays < 24 elements, more for larger
	arrayHeaderSize := cborArrayHeaderSize(len(blockArray))

	// blockArray[0] is the header, blockArray[1] is tx bodies, blockArray[2] is witnesses
	// blockArray[3] is metadata (if present)
	headerOffset := arrayHeaderSize
	txBodiesOffset := headerOffset + uint32(len(blockArray[0]))    // #nosec G115 -- Cardano block segments are <<4GiB
	witnessesOffset := txBodiesOffset + uint32(len(blockArray[1])) // #nosec G115 -- Cardano block segments are <<4GiB
	var metadataOffset uint32
	if len(blockArray) > 3 {
		metadataOffset = witnessesOffset + uint32(len(blockArray[2])) // #nosec G115 -- Cardano block segments are <<4GiB
	}

	// Parse transaction bodies array
	var txBodiesRaw []cbor.RawMessage
	if _, err := cbor.Decode([]byte(blockArray[1]), &txBodiesRaw); err != nil {
		return nil, fmt.Errorf("failed to decode transaction bodies: %w", err)
	}

	// Parse witness sets array
	var witnessesRaw []cbor.RawMessage
	if _, err := cbor.Decode([]byte(blockArray[2]), &witnessesRaw); err != nil {
		return nil, fmt.Errorf("failed to decode witness sets: %w", err)
	}

	// Validate that transaction bodies and witness sets have matching lengths
	if len(txBodiesRaw) != len(witnessesRaw) {
		return nil, fmt.Errorf(
			"mismatched transaction bodies (%d) and witness sets (%d)",
			len(txBodiesRaw),
			len(witnessesRaw),
		)
	}

	// Parse metadata map (keyed by transaction index)
	// Metadata is stored as a CBOR map: {tx_index: metadata, ...}
	metadataByTxIdx := make(map[uint32]struct {
		offset uint32
		length uint32
	})
	if len(blockArray) > 3 && len(blockArray[3]) > 1 {
		// Extract metadata offsets by parsing the map structure
		// Ignore errors - metadata extraction is best-effort
		// Some blocks may have unusual metadata structures
		_ = extractMetadataOffsets(
			[]byte(blockArray[3]),
			metadataOffset,
			metadataByTxIdx,
		)
	}

	// Build transaction locations
	result := &BlockTransactionOffsets{
		Transactions: make([]TransactionLocation, len(txBodiesRaw)),
	}

	// Calculate body offsets within the tx bodies array
	bodiesArrayHeader := cborArrayHeaderSize(len(txBodiesRaw))
	bodyPos := txBodiesOffset + bodiesArrayHeader
	for i, rawBody := range txBodiesRaw {
		bodyLen := uint32(len(rawBody)) // #nosec G115 -- Cardano block segments are <<4GiB
		result.Transactions[i].Body = ByteRange{
			Offset: bodyPos,
			Length: bodyLen,
		}

		// Extract output offsets from transaction body
		extractOutputOffsets([]byte(rawBody), bodyPos, &result.Transactions[i])

		bodyPos += bodyLen
	}

	// Calculate witness offsets within the witnesses array
	witnessArrayHeader := cborArrayHeaderSize(len(witnessesRaw))
	witnessPos := witnessesOffset + witnessArrayHeader
	for i, rawWitness := range witnessesRaw {
		if i < len(result.Transactions) {
			result.Transactions[i].Witness = ByteRange{
				Offset: witnessPos,
				Length: uint32(len(rawWitness)), // #nosec G115 -- Cardano block segments are <<4GiB
			}

			// Extract datum, redeemer, and script offsets from witness set
			extractWitnessComponentOffsets(
				[]byte(rawWitness),
				witnessPos,
				&result.Transactions[i],
			)
		}
		witnessPos += uint32(len(rawWitness)) // #nosec G115 -- Cardano block segments are <<4GiB
	}

	// Assign metadata offsets
	for i := range result.Transactions {
		// #nosec G115 -- transaction index bounded by block size
		if meta, ok := metadataByTxIdx[uint32(i)]; ok {
			result.Transactions[i].Metadata = ByteRange{
				Offset: meta.offset,
				Length: meta.length,
			}
		}
	}

	return result, nil
}

// Witness set map keys (Alonzo+ era)
const (
	witnessKeyVkeyWitnesses    = 0
	witnessKeyNativeScripts    = 1
	witnessKeyBootstrapWitness = 2
	witnessKeyPlutusV1Scripts  = 3
	witnessKeyPlutusData       = 4 // Datums
	witnessKeyRedeemers        = 5
	witnessKeyPlutusV2Scripts  = 6
	witnessKeyPlutusV3Scripts  = 7
)

// extractOutputOffsets parses a transaction body to find output CBOR positions.
// This uses streaming decode to track positions of each output within the body.
// The bodyData is the raw CBOR of the transaction body (a map).
// The bodyOffset is the absolute offset of bodyData within the block.
func extractOutputOffsets(
	bodyData []byte,
	bodyOffset uint32,
	loc *TransactionLocation,
) {
	if len(bodyData) < 2 {
		return
	}

	// Transaction body is a CBOR map with numeric keys
	// Key 1 contains the outputs array
	count, headerSize, indefinite := cborMapInfo(bodyData)
	if count < 0 && !indefinite {
		return
	}

	// Create a stream decoder for the body content (after map header)
	bodyStream, err := cbor.NewStreamDecoder(bodyData[headerSize:])
	if err != nil {
		return
	}

	// Scan through map key-value pairs looking for key 1 (outputs)
	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite maps
		if indefinite {
			pos := bodyStream.Position()
			checkPos := int(headerSize) + pos
			if checkPos >= len(bodyData) || bodyData[checkPos] == 0xff {
				break
			}
		}

		// Decode map key
		var key uint64
		if _, _, err := bodyStream.Decode(&key); err != nil {
			return
		}

		if key == 1 {
			// Found the outputs array - decode it with position tracking
			valueStart := bodyStream.Position()

			// Decode outputs as array of RawMessage
			var outputsRaw []cbor.RawMessage
			if _, _, err := bodyStream.Decode(&outputsRaw); err != nil {
				return
			}

			// Calculate the absolute offset of the outputs array
			// #nosec G115 -- valueStart is position within a Cardano tx body, well under 4GiB
			outputsArrayOffset := bodyOffset + headerSize + uint32(valueStart)
			outputsArrayHeader := uint32(cborArrayHeaderSize(len(outputsRaw)))

			// Track position within outputs array
			outputPos := outputsArrayOffset + outputsArrayHeader

			// Record each output's position
			loc.Outputs = make([]ByteRange, len(outputsRaw))
			for j, rawOutput := range outputsRaw {
				outputLen := uint32(len(rawOutput)) // #nosec G115 -- Cardano outputs are <<4GiB
				loc.Outputs[j] = ByteRange{
					Offset: outputPos,
					Length: outputLen,
				}
				outputPos += outputLen
			}

			return // Found outputs, done with this body
		}

		// Skip the value for this key
		if _, _, err := bodyStream.Skip(); err != nil {
			return
		}
	}
}

// extractWitnessComponentOffsets parses a witness set and extracts byte offsets
// for datums, redeemers, and scripts using streaming decoding.
func extractWitnessComponentOffsets(
	witnessData []byte,
	baseOffset uint32,
	loc *TransactionLocation,
) {
	if len(witnessData) < 2 {
		return
	}

	// Initialize maps with small capacity hints
	loc.Datums = make(map[Blake2b256]ByteRange, 4)
	loc.Redeemers = make(map[RedeemerKey]ByteRange, 4)
	loc.Scripts = make(map[ScriptHash]ByteRange, 4)

	// Get map info from header
	count, headerSize, indefinite := cborMapInfo(witnessData)
	if count < 0 && !indefinite {
		return
	}

	// Use streaming decoder starting after the map header
	dec, err := cbor.NewStreamDecoder(witnessData[headerSize:])
	if err != nil {
		return
	}

	// Process each key-value pair in CBOR order
	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite maps
		if indefinite {
			pos := dec.Position()
			if int(headerSize)+pos >= len(witnessData) || witnessData[int(headerSize)+pos] == 0xff {
				break
			}
		}

		// Decode the key
		var key uint64
		if _, _, err := dec.Decode(&key); err != nil {
			return
		}

		// Get value position and bytes
		valueOffset, valueBytes, err := dec.DecodeRaw(new(cbor.RawMessage))
		if err != nil {
			return
		}

		absOffset := baseOffset + headerSize + uint32(valueOffset) // #nosec G115

		switch key {
		case witnessKeyPlutusData:
			extractDatumOffsets(valueBytes, absOffset, loc.Datums)

		case witnessKeyRedeemers:
			extractRedeemerOffsets(valueBytes, absOffset, loc.Redeemers)

		case witnessKeyNativeScripts:
			extractScriptArrayOffsets(valueBytes, absOffset, ScriptRefTypeNativeScript, loc.Scripts)

		case witnessKeyPlutusV1Scripts:
			extractScriptArrayOffsets(valueBytes, absOffset, ScriptRefTypePlutusV1, loc.Scripts)

		case witnessKeyPlutusV2Scripts:
			extractScriptArrayOffsets(valueBytes, absOffset, ScriptRefTypePlutusV2, loc.Scripts)

		case witnessKeyPlutusV3Scripts:
			extractScriptArrayOffsets(valueBytes, absOffset, ScriptRefTypePlutusV3, loc.Scripts)
		}
	}
}

// extractDatumOffsets extracts datum hash -> offset mappings from a datum array using streaming decoding.
// The datum structure is an array of datums: [ datum_cbor, datum_cbor, ... ]
// Each datum's hash is computed via blake2b-256 of its CBOR encoding.
func extractDatumOffsets(datumArrayData []byte, baseOffset uint32, result map[Blake2b256]ByteRange) {
	if len(datumArrayData) < 1 {
		return
	}

	// Get array info from header
	count, headerSize, indefinite := cborArrayInfo(datumArrayData)
	if count < 0 && !indefinite {
		return // Invalid array header
	}

	// Use streaming decoder starting after the array header
	dec, err := cbor.NewStreamDecoder(datumArrayData[headerSize:])
	if err != nil {
		return
	}

	// Process each datum in the array
	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite arrays
		if indefinite {
			pos := dec.Position()
			if int(headerSize)+pos >= len(datumArrayData) {
				break
			}
			if datumArrayData[int(headerSize)+pos] == 0xff {
				break
			}
		}

		// Get datum position and raw bytes
		datumOffset, datumBytes, err := dec.DecodeRaw(new(cbor.RawMessage))
		if err != nil {
			return
		}

		// Compute datum hash: blake2b-256 of the datum CBOR
		computed := blake2b.Sum256(datumBytes)
		var hash Blake2b256
		copy(hash[:], computed[:])

		result[hash] = ByteRange{
			Offset: baseOffset + headerSize + uint32(datumOffset), // #nosec G115 -- Cardano block segments are <<4GiB
			Length: uint32(len(datumBytes)),                       // #nosec G115 -- Cardano block segments are <<4GiB
		}
	}
}

// extractRedeemerOffsets extracts redeemer key -> offset mappings.
// Redeemers can be either an array (Alonzo-Babbage) or map (Conway+).
func extractRedeemerOffsets(redeemerData []byte, baseOffset uint32, result map[RedeemerKey]ByteRange) {
	if len(redeemerData) < 1 {
		return
	}

	// Check if it's a map or array by looking at the first byte (CBOR major type)
	firstByte := redeemerData[0]
	majorType := firstByte & 0xe0

	switch majorType {
	case 0x80:
		// It's an array (Alonzo-Babbage format): [[purpose, index, data, exunits], ...]
		// Major type 4 = 0x80-0x9f (including 0x9f for indefinite length)
		extractRedeemerArrayOffsets(redeemerData, baseOffset, result)
	case 0xa0:
		// It's a map (Conway+ format): {[purpose, index]: [data, exunits], ...}
		// Major type 5 = 0xa0-0xbf (including 0xbf for indefinite length)
		extractRedeemerMapOffsets(redeemerData, baseOffset, result)
	}
}

// extractRedeemerMapOffsets handles the map format for redeemers using streaming decoding.
// Map format (Conway+): {[purpose, index]: [data, exunits], ...}
func extractRedeemerMapOffsets(redeemerData []byte, baseOffset uint32, result map[RedeemerKey]ByteRange) {
	count, headerSize, indefinite := cborMapInfo(redeemerData)
	if count < 0 && !indefinite {
		return
	}

	dec, err := cbor.NewStreamDecoder(redeemerData[headerSize:])
	if err != nil {
		return
	}

	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite maps
		if indefinite {
			pos := dec.Position()
			if int(headerSize)+pos >= len(redeemerData) || redeemerData[int(headerSize)+pos] == 0xff {
				break
			}
		}

		// Decode key as [purpose, index]
		var keyPair []uint64
		if _, _, err := dec.Decode(&keyPair); err != nil || len(keyPair) < 2 {
			return
		}

		key := RedeemerKey{
			Tag:   RedeemerTag(keyPair[0]), // #nosec G115 -- redeemer tag is a small enum value
			Index: uint32(keyPair[1]),      // #nosec G115 -- redeemer index bounded by transaction size
		}

		// Get value position and bytes: value is [data, exunits]
		valueOffset, valueBytes, err := dec.DecodeRaw(new(cbor.RawMessage))
		if err != nil {
			return
		}

		// Parse the value array to find the data element's position within it
		// Get array header size first, then create decoder starting after the header
		_, valueHeaderSize, _ := cborArrayInfo(valueBytes)
		if int(valueHeaderSize) >= len(valueBytes) {
			continue
		}
		valueDec, err := cbor.NewStreamDecoder(valueBytes[valueHeaderSize:])
		if err != nil {
			continue
		}

		// Get first element (data) position - decoder now starts at the first element
		dataOffset, dataLen, err := valueDec.Skip()
		if err != nil {
			continue
		}

		result[key] = ByteRange{
			Offset: baseOffset + headerSize + uint32(valueOffset) + valueHeaderSize + uint32(dataOffset), // #nosec G115
			Length: uint32(dataLen),                                                                      // #nosec G115
		}
	}
}

// extractRedeemerArrayOffsets handles the array format for redeemers (Alonzo-Babbage) using streaming decoding.
// Array format: [[purpose, index, data, exunits], ...]
func extractRedeemerArrayOffsets(redeemerData []byte, baseOffset uint32, result map[RedeemerKey]ByteRange) {
	count, headerSize, indefinite := cborArrayInfo(redeemerData)
	if count < 0 && !indefinite {
		return
	}

	dec, err := cbor.NewStreamDecoder(redeemerData[headerSize:])
	if err != nil {
		return
	}

	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite arrays
		if indefinite {
			pos := dec.Position()
			if int(headerSize)+pos >= len(redeemerData) || redeemerData[int(headerSize)+pos] == 0xff {
				break
			}
		}

		// Get the redeemer element's position and bytes
		elemOffset, elemBytes, err := dec.DecodeRaw(new(cbor.RawMessage))
		if err != nil {
			return
		}

		// Parse the redeemer array [purpose, index, data, exunits] using streaming
		// Get inner array header size first, then create decoder starting after the header
		_, innerHeaderSize, _ := cborArrayInfo(elemBytes)
		if int(innerHeaderSize) >= len(elemBytes) {
			continue
		}
		elemDec, err := cbor.NewStreamDecoder(elemBytes[innerHeaderSize:])
		if err != nil {
			continue
		}

		// Decode purpose and index - decoder now starts at the first element
		var purpose, index uint64
		if _, _, err := elemDec.Decode(&purpose); err != nil {
			continue
		}
		if _, _, err := elemDec.Decode(&index); err != nil {
			continue
		}

		key := RedeemerKey{
			Tag:   RedeemerTag(purpose), // #nosec G115 -- redeemer tag is a small enum value
			Index: uint32(index),        // #nosec G115 -- redeemer index bounded by transaction size
		}

		// Get the data element's position (parts[2])
		dataOffset, dataLen, err := elemDec.Skip()
		if err != nil {
			continue
		}

		result[key] = ByteRange{
			Offset: baseOffset + headerSize + uint32(elemOffset) + innerHeaderSize + uint32(dataOffset), // #nosec G115
			Length: uint32(dataLen),                                                                     // #nosec G115
		}
	}
}

// cborArrayInfo extracts array item count and header size from CBOR array data.
// Returns (count, headerSize, isIndefinite). Count is -1 for invalid headers.
func cborArrayInfo(data []byte) (int, uint32, bool) {
	if len(data) == 0 {
		return -1, 0, false
	}
	firstByte := data[0]

	// Check it's an array (major type 4)
	if firstByte&0xe0 != 0x80 {
		return -1, 0, false
	}

	additional := firstByte & 0x1f
	switch {
	case additional <= 23:
		return int(additional), 1, false
	case additional == 24 && len(data) >= 2:
		return int(data[1]), 2, false
	case additional == 25 && len(data) >= 3:
		return int(uint16(data[1])<<8 | uint16(data[2])), 3, false
	case additional == 26 && len(data) >= 5:
		// 4-byte length - check for overflow before converting to int
		len32 := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
		if len32 > uint32(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len32), 5, false
	case additional == 27 && len(data) >= 9:
		// 8-byte length (unlikely for Cardano blocks but handle for completeness)
		len64 := uint64(data[1])<<56 | uint64(data[2])<<48 |
			uint64(data[3])<<40 | uint64(data[4])<<32 |
			uint64(data[5])<<24 | uint64(data[6])<<16 |
			uint64(data[7])<<8 | uint64(data[8])
		if len64 > uint64(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len64), 9, false
	case additional == 31:
		return 0, 1, true // Indefinite length
	default:
		return -1, 0, false
	}
}

// extractScriptArrayOffsets extracts script hash -> offset mappings from a script array.
// The scriptType parameter specifies the Cardano script type prefix used in hashing:
// 0x00 = native script, 0x01 = PlutusV1, 0x02 = PlutusV2, 0x03 = PlutusV3
func extractScriptArrayOffsets(scriptArrayData []byte, baseOffset uint32, scriptType byte, result map[ScriptHash]ByteRange) {
	if len(scriptArrayData) < 1 {
		return
	}

	var scripts []cbor.RawMessage
	if _, err := cbor.Decode(scriptArrayData, &scripts); err != nil {
		return
	}

	// Determine header size based on actual encoding
	// 0x9f indicates indefinite-length array (header = 1 byte)
	var arrayHeaderSize uint32
	if scriptArrayData[0] == 0x9f {
		arrayHeaderSize = 1
	} else {
		_, arrayHeaderSize, _ = cborArrayInfo(scriptArrayData)
	}
	pos := arrayHeaderSize

	for _, scriptRaw := range scripts {
		scriptBytes := []byte(scriptRaw)

		// Compute script hash: blake2b-224(prefix || script_cbor)
		// This matches the Hash() method on Script types
		hashKey := Blake2b224Hash(slices.Concat([]byte{scriptType}, scriptBytes))

		result[hashKey] = ByteRange{
			Offset: baseOffset + pos,
			Length: uint32(len(scriptBytes)), // #nosec G115 -- Cardano block segments are <<4GiB
		}

		pos += uint32(len(scriptBytes)) // #nosec G115 -- Cardano block segments are <<4GiB
	}
}

// extractMetadataOffsets parses a CBOR map to extract value offsets using streaming decoding.
// The metadata map has transaction indices as keys and metadata as values.
func extractMetadataOffsets(
	mapData []byte,
	baseOffset uint32,
	result map[uint32]struct {
		offset uint32
		length uint32
	},
) error {
	if len(mapData) == 0 {
		return nil
	}

	// Get map item count from header
	count, headerSize, indefinite := cborMapInfo(mapData)
	if count < 0 {
		return nil // Invalid map header
	}

	// Use streaming decoder starting after the map header
	dec, err := cbor.NewStreamDecoder(mapData[headerSize:])
	if err != nil {
		return err
	}

	// For indefinite maps, we don't know the count upfront
	// Process until we hit the break byte (0xff) or run out of data
	for i := 0; indefinite || i < count; i++ {
		// Check for break byte in indefinite maps
		if indefinite {
			pos := dec.Position()
			if int(headerSize)+pos >= len(mapData) {
				break
			}
			if mapData[int(headerSize)+pos] == 0xff {
				break // End of indefinite map
			}
		}

		// Decode the key (transaction index)
		var txIdx uint64
		if _, _, err := dec.Decode(&txIdx); err != nil {
			return err
		}

		// Get position and skip the value to get its byte range
		valueOffset, valueLen, err := dec.Skip()
		if err != nil {
			return err
		}

		// #nosec G115 -- transaction index bounded by block size, Cardano block segments are <<4GiB
		result[uint32(txIdx)] = struct {
			offset uint32
			length uint32
		}{
			offset: baseOffset + headerSize + uint32(valueOffset),
			length: uint32(valueLen),
		}
	}

	return nil
}

// cborMapInfo extracts map item count and header size from CBOR map data.
// Returns (count, headerSize, isIndefinite). Count is -1 for invalid headers.
func cborMapInfo(data []byte) (int, uint32, bool) {
	if len(data) == 0 {
		return -1, 0, false
	}
	firstByte := data[0]

	// Check it's a map (major type 5)
	if firstByte&0xe0 != 0xa0 {
		return -1, 0, false
	}

	additional := firstByte & 0x1f
	switch {
	case additional <= 23:
		return int(additional), 1, false
	case additional == 24 && len(data) >= 2:
		return int(data[1]), 2, false
	case additional == 25 && len(data) >= 3:
		return int(uint16(data[1])<<8 | uint16(data[2])), 3, false
	case additional == 26 && len(data) >= 5:
		// 4-byte length - check for overflow before converting to int
		len32 := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
		if len32 > uint32(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len32), 5, false
	case additional == 27 && len(data) >= 9:
		// 8-byte length (unlikely for Cardano blocks but handle for completeness)
		len64 := uint64(data[1])<<56 | uint64(data[2])<<48 |
			uint64(data[3])<<40 | uint64(data[4])<<32 |
			uint64(data[5])<<24 | uint64(data[6])<<16 |
			uint64(data[7])<<8 | uint64(data[8])
		if len64 > uint64(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len64), 9, false
	case additional == 31:
		return 0, 1, true // Indefinite length
	default:
		return -1, 0, false
	}
}

// cborArrayHeaderSize returns the CBOR header size in bytes for an array of given length.
func cborArrayHeaderSize(length int) uint32 {
	if length < 24 {
		return 1 // 0x80 + length
	} else if length < 256 {
		return 2 // 0x98 + 1-byte length
	} else if length < 65536 {
		return 3 // 0x99 + 2-byte length
	}
	return 5 // 0x9a + 4-byte length (covers up to 2^32 elements)
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
