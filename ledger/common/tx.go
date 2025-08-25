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
	"fmt"
	"iter"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/plutigo/data"
	utxorpc "github.com/utxorpc/go-codegen/utxorpc/v1alpha/cardano"
)

type Transaction interface {
	TransactionBody
	Type() int
	Cbor() []byte
	Metadata() TransactionMetadataSet
	IsValid() bool
	Consumed() []TransactionInput
	Produced() []Utxo
	Witnesses() TransactionWitnessSet
}

type TransactionMetadataSet map[uint64]TransactionMetadatum

type TransactionMetadatum interface {
	TypeName() string
}

type MetaInt struct {
	Value int64
}

type MetaBytes struct {
	Value []byte
}

type MetaText struct {
	Value string
}

type MetaList struct {
	Items []TransactionMetadatum
}

type MetaPair struct {
	Key   TransactionMetadatum
	Value TransactionMetadatum
}

type MetaMap struct {
	Pairs []MetaPair
}

type TransactionBody interface {
	Cbor() []byte
	Fee() uint64
	Hash() Blake2b256
	Inputs() []TransactionInput
	Outputs() []TransactionOutput
	TTL() uint64
	ProtocolParameterUpdates() (uint64, map[Blake2b224]ProtocolParameterUpdate)
	ValidityIntervalStart() uint64
	ReferenceInputs() []TransactionInput
	Collateral() []TransactionInput
	CollateralReturn() TransactionOutput
	TotalCollateral() uint64
	Certificates() []Certificate
	Withdrawals() map[*Address]uint64
	AuxDataHash() *Blake2b256
	RequiredSigners() []Blake2b224
	AssetMint() *MultiAsset[MultiAssetTypeMint]
	ScriptDataHash() *Blake2b256
	VotingProcedures() VotingProcedures
	ProposalProcedures() []ProposalProcedure
	CurrentTreasuryValue() int64
	Donation() uint64
	Utxorpc() (*utxorpc.Tx, error)
}

type TransactionInput interface {
	Id() Blake2b256
	Index() uint32
	String() string
	Utxorpc() (*utxorpc.TxInput, error)
	ToPlutusData() data.PlutusData
}

type TransactionOutput interface {
	Address() Address
	Amount() uint64
	Assets() *MultiAsset[MultiAssetTypeOutput]
	Datum() *Datum
	DatumHash() *Blake2b256
	Cbor() []byte
	Utxorpc() (*utxorpc.TxOutput, error)
	ScriptRef() Script
	ToPlutusData() data.PlutusData
}

type TransactionWitnessSet interface {
	Vkey() []VkeyWitness
	NativeScripts() []NativeScript
	Bootstrap() []BootstrapWitness
	PlutusData() []Datum
	PlutusV1Scripts() [][]byte
	PlutusV2Scripts() [][]byte
	PlutusV3Scripts() [][]byte
	Redeemers() TransactionWitnessRedeemers
}

type TransactionWitnessRedeemers interface {
	Indexes(RedeemerTag) []uint
	Value(uint, RedeemerTag) RedeemerValue
	Iter() iter.Seq2[RedeemerKey, RedeemerValue]
}

type Utxo struct {
	cbor.StructAsArray
	Id     TransactionInput
	Output TransactionOutput
}

// TransactionBodyBase provides a set of functions that return empty values to satisfy the
// TransactionBody interface. It also provides functionality for generating a transaction hash
// and storing/retrieving the original CBOR
type TransactionBodyBase struct {
	cbor.DecodeStoreCbor
	hash *Blake2b256
}

func (b *TransactionBodyBase) Hash() Blake2b256 {
	if b.hash == nil {
		tmpHash := Blake2b256Hash(b.Cbor())
		b.hash = &tmpHash
	}
	return *b.hash
}

func (b *TransactionBodyBase) Inputs() []TransactionInput {
	return nil
}

func (b *TransactionBodyBase) Outputs() []TransactionOutput {
	return nil
}

func (b *TransactionBodyBase) Fee() uint64 {
	return 0
}

func (b *TransactionBodyBase) TTL() uint64 {
	return 0
}

func (b *TransactionBodyBase) ValidityIntervalStart() uint64 {
	return 0
}

func (b *TransactionBodyBase) ReferenceInputs() []TransactionInput {
	return []TransactionInput{}
}

func (b *TransactionBodyBase) Collateral() []TransactionInput {
	return nil
}

func (b *TransactionBodyBase) CollateralReturn() TransactionOutput {
	return nil
}

func (b *TransactionBodyBase) TotalCollateral() uint64 {
	return 0
}

func (b *TransactionBodyBase) Certificates() []Certificate {
	return nil
}

func (b *TransactionBodyBase) Withdrawals() map[*Address]uint64 {
	return nil
}

func (b *TransactionBodyBase) AuxDataHash() *Blake2b256 {
	return nil
}

func (b *TransactionBodyBase) RequiredSigners() []Blake2b224 {
	return nil
}

func (b *TransactionBodyBase) AssetMint() *MultiAsset[MultiAssetTypeMint] {
	return nil
}

func (b *TransactionBodyBase) ScriptDataHash() *Blake2b256 {
	return nil
}

func (b *TransactionBodyBase) VotingProcedures() VotingProcedures {
	return nil
}

func (b *TransactionBodyBase) ProposalProcedures() []ProposalProcedure {
	return nil
}

func (b *TransactionBodyBase) CurrentTreasuryValue() int64 {
	return 0
}

func (b *TransactionBodyBase) Donation() uint64 {
	return 0
}

func (b *TransactionBodyBase) Utxorpc() *utxorpc.Tx {
	return nil
}

// TransactionBodyToUtxorpc is a common helper for converting TransactionBody to utxorpc.Tx
func TransactionBodyToUtxorpc(tx TransactionBody) *utxorpc.Tx {
	txi := []*utxorpc.TxInput{}
	txo := []*utxorpc.TxOutput{}
	for _, i := range tx.Inputs() {
		input, err := i.Utxorpc()
		if err != nil {
			return nil
		}
		txi = append(txi, input)
	}
	for _, o := range tx.Outputs() {
		output, err := o.Utxorpc()
		if err != nil {
			return nil
		}
		txo = append(txo, output)
	}
	ret := &utxorpc.Tx{
		Inputs:  txi,
		Outputs: txo,
		// Certificates:    tx.Certificates(),
		// Withdrawals:     tx.Withdrawals(),
		// Mint:            tx.Mint(),
		// ReferenceInputs: tx.ReferenceInputs(),
		// Witnesses:       tx.Witnesses(),
		// Collateral:      tx.Collateral(),
		Fee: tx.Fee(),
		// Validity:        tx.Validity(),
		// Successful:      tx.Successful(),
		// Auxiliary:       tx.AuxData(),
		Hash: tx.Hash().Bytes(),
		// Proposals:       tx.ProposalProcedures(),
	}
	for _, ri := range tx.ReferenceInputs() {
		input, err := ri.Utxorpc()
		if err != nil {
			return nil
		}
		ret.ReferenceInputs = append(ret.ReferenceInputs, input)
	}
	for _, c := range tx.Certificates() {
		cert, err := c.Utxorpc()
		if err != nil {
			return nil
		}
		ret.Certificates = append(ret.Certificates, cert)
	}

	return ret
}

func (m MetaInt) TypeName() string { return "int" }

func (m MetaBytes) TypeName() string { return "bytes" }

func (m MetaText) TypeName() string { return "text" }

func (m MetaList) TypeName() string { return "list" }

func (m MetaMap) TypeName() string { return "map" }

// Tries Decoding CBOR into all TransactionMetadatum variants (int, text, bytes, list, map).
func DecodeMetadatumRaw(b []byte) (TransactionMetadatum, error) {
	// Trying to decode as int64
	{
		var v int64
		if _, err := cbor.Decode(b, &v); err == nil {
			return MetaInt{Value: v}, nil
		}
	}
	// Trying to decode as string
	{
		var s string
		if _, err := cbor.Decode(b, &s); err == nil {
			return MetaText{Value: s}, nil
		}
	}
	// Trying to decode as []bytes
	{
		var bs []byte
		if _, err := cbor.Decode(b, &bs); err == nil {
			return MetaBytes{Value: bs}, nil
		}
	}
	// Trying to decode as cbor.RawMessage first then recursively decode each value
	{
		var arr []cbor.RawMessage
		if _, err := cbor.Decode(b, &arr); err == nil {
			items := make([]TransactionMetadatum, 0, len(arr))
			for _, it := range arr {
				md, err := DecodeMetadatumRaw(it)
				if err != nil {
					return nil, fmt.Errorf("decode list item: %w", err)
				}
				items = append(items, md)
			}
			return MetaList{Items: items}, nil
		}
	}
	// Trying to decode as map[uint64]cbor.RawMessage first.
	// Next trying to decode key as MetaInt and value as MetaMap
	{
		var m map[uint64]cbor.RawMessage
		if _, err := cbor.Decode(b, &m); err == nil && len(m) > 0 {
			pairs := make([]MetaPair, 0, len(m))
			for k, rv := range m {
				val, err := DecodeMetadatumRaw(rv)
				if err != nil {
					return nil, fmt.Errorf("decode map(uint) value: %w", err)
				}
				pairs = append(pairs, MetaPair{
					Key:   MetaInt{Value: int64(k)},
					Value: val,
				})
			}
			return MetaMap{Pairs: pairs}, nil
		}
	}
	// Trying to decode as map[string]cbor.RawMessage first.
	// Next trying to decode key as MetaText and value as MetaMap
	{
		var m map[string]cbor.RawMessage
		if _, err := cbor.Decode(b, &m); err == nil && len(m) > 0 {
			pairs := make([]MetaPair, 0, len(m))
			for k, rv := range m {
				val, err := DecodeMetadatumRaw(rv)
				if err != nil {
					return nil, fmt.Errorf("decode map(text) value: %w", err)
				}
				pairs = append(pairs, MetaPair{
					Key:   MetaText{Value: k},
					Value: val,
				})
			}
			return MetaMap{Pairs: pairs}, nil
		}
	}

	return nil, fmt.Errorf("unsupported metadatum shape")
}

// Decodes the transaction metadata set.
func (s *TransactionMetadataSet) UnmarshalCBOR(cborData []byte) error {
	// Trying to decode as map[uint64]cbor.RawMessage.
	// Calling DecodeMetadatumRaw for each entry call to get the typed value.
	{
		var tmp map[uint64]cbor.RawMessage
		if _, err := cbor.Decode(cborData, &tmp); err == nil {
			out := make(TransactionMetadataSet, len(tmp))
			for k, v := range tmp {
				md, err := DecodeMetadatumRaw(v)
				if err != nil {
					return fmt.Errorf("decode metadata value for index %d: %w", k, err)
				}
				out[k] = md
			}
			*s = out
			return nil
		}
	}
	// Trying to decode as []cbor.RawMessage.
	// Each element in array is decoded by calling DecodeMetadatumRaw
	{
		var arr []cbor.RawMessage
		if _, err := cbor.Decode(cborData, &arr); err == nil {
			out := make(TransactionMetadataSet)
			for i, raw := range arr {
				var probe any
				// Skipping null values as well after decoding to cbor.RawMessage
				if _, err := cbor.Decode(raw, &probe); err == nil {
					continue
				}
				md, err := DecodeMetadatumRaw(raw)
				if err != nil {
					return fmt.Errorf("decode metadata list item %d: %w", i, err)
				}
				out[uint64(i)] = md
			}
			*s = out
			return nil
		}
	}
	return fmt.Errorf("unsupported TransactionMetadataSet encoding")
}

// Encodes the transaction metadata set as a CBOR map
func (s TransactionMetadataSet) MarshalCBOR() ([]byte, error) {
	if s == nil {
		return cbor.Encode(&map[uint64]any{})
	}
	contiguous := true
	var maxKey uint64
	for k := range s {
		if k > maxKey {
			maxKey = k
		}
	}
	expectedCount := int(maxKey + 1)
	if len(s) != expectedCount {
		contiguous = false
	} else {
		for i := 0; i < expectedCount; i++ {
			if _, ok := s[uint64(i)]; !ok {
				contiguous = false
				break
			}
		}
	}
	if contiguous {
		arr := make([]any, expectedCount)
		for i := 0; i < expectedCount; i++ {
			arr[i] = metadatumToInterface(s[uint64(i)])
		}
		return cbor.Encode(&arr)
	}
	// Otherwise Encode as a map.
	tmpMap := make(map[uint64]any, len(s))
	for k, v := range s {
		tmpMap[k] = metadatumToInterface(v)
	}
	return cbor.Encode(&tmpMap)
}

// converting typed metadatum back into regular go values where the CBOR library can encode
func metadatumToInterface(m TransactionMetadatum) any {
	switch t := m.(type) {
	case MetaInt:
		return t.Value
	case MetaBytes:
		return []byte(t.Value)
	case MetaText:
		return t.Value
	case MetaList:
		out := make([]any, 0, len(t.Items))
		for _, it := range t.Items {
			out = append(out, metadatumToInterface(it))
		}
		return out
	case MetaMap:
		mm := make(map[any]any, len(t.Pairs))
		for _, p := range t.Pairs {
			mm[metadatumToInterface(p.Key)] = metadatumToInterface(p.Value)
		}
		return mm
	default:
		return nil
	}
}
