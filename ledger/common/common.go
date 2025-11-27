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
	"errors"
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

	// Era IDs for Cardano eras
	EraIdShelley = 1
	EraIdAllegra = 2
	EraIdMary    = 3
	EraIdAlonzo  = 4
	EraIdBabbage = 5
	EraIdConway  = 6
)

// emptyInvalidTransactionsCbor is the CBOR encoding of an empty indefinite-length list
// used for default invalid transactions in body hash calculation. Computed once at package init.
var emptyInvalidTransactionsCbor []byte

func init() {
	var err error
	emptyInvalidTransactionsCbor, err = cbor.Encode(cbor.IndefLengthList([]any{}))
	if err != nil {
		panic(fmt.Sprintf("failed to encode empty invalid transactions CBOR: %v", err))
	}
}

func convertToAnySlice[T any](slice []T) []any {
	result := make([]any, len(slice))
	for i, v := range slice {
		result[i] = v
	}
	return result
}

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

// BlockParseOption is a functional option for configuring block parsing behavior
type BlockParseOption func(*blockParseOptions)

// blockParseOptions holds configuration options for block parsing
type blockParseOptions struct {
	skipBodyHashValidation bool
}

// SkipBodyHashValidation disables body hash validation during block parsing.
// This should only be used for debugging, testing, or when parsing blocks
// from untrusted sources where validation is handled elsewhere.
func SkipBodyHashValidation() BlockParseOption {
	return func(opts *blockParseOptions) {
		opts.skipBodyHashValidation = true
	}
}

// ValidateBlockBodyHash validates that the block's body hash matches the computed hash
// of its components (transactions, witnesses, auxiliary data, and invalid transactions)
// as per the Cardano specification. This ensures block parsing integrity by validating
// the body hash during block construction rather than only during verification.
func ValidateBlockBodyHash(
	block Block,
	cborData []byte,
	opts ...BlockParseOption,
) error {
	options := &blockParseOptions{}
	for _, opt := range opts {
		opt(options)
	}

	if options.skipBodyHashValidation {
		return nil
	}

	expectedBodyHash := block.BlockBodyHash()
	if len(cborData) == 0 {
		return errors.New("block CBOR is required for body hash validation")
	}

	var raw []cbor.RawMessage
	if _, err := cbor.Decode(cborData, &raw); err != nil {
		return fmt.Errorf(
			"failed to decode block CBOR for body hash validation: %w",
			err,
		)
	}
	if len(raw) < 4 {
		return errors.New(
			"invalid block CBOR structure for body hash validation",
		)
	}

	// Compute body hash as per Cardano spec: blake2b_256(hash_tx || hash_wit || hash_aux || hash_invalid)
	hashInvalidDefault := blake2b.Sum256(emptyInvalidTransactionsCbor)
	var bodyHashes []byte

	// Get era for encoding decisions
	eraId := block.Header().Era().Id

	// Hash transaction bodies - re-encode as definite-length list for Shelley, indefinite for later eras
	var txBodies []cbor.RawMessage
	if _, err := cbor.Decode(raw[1], &txBodies); err != nil {
		return fmt.Errorf("failed to decode transaction bodies: %w", err)
	}
	var txBodiesEncoded []byte
	var err error
	// Block body hash must use definite-length canonical CBOR for all eras per Cardano spec
	txBodiesEncoded, err = cbor.Encode(convertToAnySlice(txBodies))
	if err != nil {
		return fmt.Errorf("failed to re-encode transaction bodies: %w", err)
	}
	hashTx := blake2b.Sum256(txBodiesEncoded)
	bodyHashes = append(bodyHashes, hashTx[:]...)

	// Hash transaction witnesses - re-encode as definite-length list for Shelley, indefinite for later eras
	var txWits []cbor.RawMessage
	if _, err := cbor.Decode(raw[2], &txWits); err != nil {
		return fmt.Errorf("failed to decode transaction witnesses: %w", err)
	}
	var txWitsEncoded []byte
	// Block body hash must use definite-length canonical CBOR for all eras per Cardano spec
	txWitsEncoded, err = cbor.Encode(convertToAnySlice(txWits))
	if err != nil {
		return fmt.Errorf("failed to re-encode transaction witnesses: %w", err)
	}
	hashWit := blake2b.Sum256(txWitsEncoded)
	bodyHashes = append(bodyHashes, hashWit[:]...)

	// Hash auxiliary data - re-encode as definite-length map for canonical form
	var auxMap map[uint]any
	if _, err := cbor.Decode(raw[3], &auxMap); err != nil {
		return fmt.Errorf("failed to decode auxiliary data: %w", err)
	}
	auxDataEncoded, err := cbor.Encode(auxMap)
	if err != nil {
		return fmt.Errorf("failed to re-encode auxiliary data: %w", err)
	}
	hashAux := blake2b.Sum256(auxDataEncoded)
	bodyHashes = append(bodyHashes, hashAux[:]...)

	// Determine which invalid transaction logic to use based on era
	switch eraId {
	case EraIdShelley,
		EraIdAllegra,
		EraIdMary: // Shelley, Allegra, Mary - no invalid transactions in body hash
		// Don't append hashInvalid for Shelley era
	case EraIdAlonzo,
		EraIdBabbage,
		EraIdConway: // Alonzo, Babbage, Conway - use actual invalid transactions
		if len(raw) < 5 {
			// If no invalid transactions part, use empty list (matches CalculateBlockBodyHash behavior)
			bodyHashes = append(bodyHashes, hashInvalidDefault[:]...)
		} else {
			// Re-encode invalid transactions as indefinite-length list per Cardano spec
			var invalidTxs []cbor.RawMessage
			if _, err := cbor.Decode(raw[4], &invalidTxs); err != nil {
				return fmt.Errorf("failed to decode invalid transactions: %w", err)
			}
			invalidTxsEncoded, err := cbor.Encode(cbor.IndefLengthList(convertToAnySlice(invalidTxs)))
			if err != nil {
				return fmt.Errorf("failed to re-encode invalid transactions: %w", err)
			}
			hashInvalid := blake2b.Sum256(invalidTxsEncoded)
			bodyHashes = append(bodyHashes, hashInvalid[:]...)
		}
	default:
		return fmt.Errorf("unsupported era for body hash validation: %d", eraId)
	}

	actualBodyHash := blake2b.Sum256(bodyHashes)
	if !bytes.Equal(actualBodyHash[:], expectedBodyHash.Bytes()) {
		return fmt.Errorf("block body hash mismatch: expected %s, got %s",
			expectedBodyHash.String(), hex.EncodeToString(actualBodyHash[:]))
	}

	return nil
}
