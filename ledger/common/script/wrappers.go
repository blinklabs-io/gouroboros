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

package script

import (
	"math/big"
	"reflect"
	"slices"

	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/data"
)

// ToPlutusData is an interface that represents types that support serialization to PlutusData when building a ScriptContext
type ToPlutusData interface {
	ToPlutusData() data.PlutusData
}

type Option[T any] struct {
	Value any
}

func (o Option[T]) ToPlutusData() data.PlutusData {
	if o.Value == nil {
		return data.NewConstr(1)
	}
	return data.NewConstr(
		0,
		toPlutusData(o.Value),
	)
}

type Pairs[T1 any, T2 any] []Pair[T1, T2]

type Pair[T1 any, T2 any] struct {
	T1 T1
	T2 T2
}

type KeyValuePairs[K any, V any] []KeyValuePair[K, V]

func (k KeyValuePairs[K, V]) ToPairs() Pairs[K, V] {
	ret := make(Pairs[K, V], len(k))
	for i := range k {
		ret[i] = Pair[K, V]{
			T1: k[i].Key,
			T2: k[i].Value,
		}
	}
	return ret
}

func (k KeyValuePairs[K, V]) ToPlutusData() data.PlutusData {
	pairs := make([][2]data.PlutusData, len(k))
	for i, tmpPair := range k {
		pairs[i] = [2]data.PlutusData{
			toPlutusData(tmpPair.Key),
			toPlutusData(tmpPair.Value),
		}
	}
	return data.NewMap(pairs)
}

type KeyValuePair[K any, V any] struct {
	Key   K
	Value V
}

func toPlutusData(val any) data.PlutusData {
	if pd, ok := val.(ToPlutusData); ok {
		return pd.ToPlutusData()
	}
	switch v := val.(type) {
	case bool:
		if v {
			return data.NewConstr(1)
		}
		return data.NewConstr(0)
	case int64:
		return data.NewInteger(new(big.Int).SetInt64(v))
	case uint64:
		return data.NewInteger(new(big.Int).SetUint64(v))
	case []ToPlutusData:
		tmpItems := make([]data.PlutusData, len(v))
		for i, item := range v {
			tmpItems[i] = item.ToPlutusData()
		}
		return data.NewList(tmpItems...)
	case []byte:
		return data.NewByteString(v)
	case data.PlutusData:
		return v
	default:
		rv := reflect.ValueOf(v)
		// nolint:exhaustive
		switch rv.Kind() {
		case reflect.Slice:
			tmpItems := make([]data.PlutusData, rv.Len())
			for i := range rv.Len() {
				item := rv.Index(i)
				tmpItems[i] = toPlutusData(item.Interface())
			}
			return data.NewList(tmpItems...)
		case reflect.Map:
			tmpPairs := make([][2]data.PlutusData, rv.Len())
			for i, k := range rv.MapKeys() {
				v := rv.MapIndex(k)
				tmpPairs[i] = [2]data.PlutusData{
					toPlutusData(k.Interface()),
					toPlutusData(v.Interface()),
				}
			}
			return data.NewMap(tmpPairs)
		}
	}
	return nil
}

type Coin int64

func (c Coin) ToPlutusData() data.PlutusData {
	return data.NewInteger(new(big.Int).SetInt64(int64(c)))
}

type PositiveCoin uint64

func (c PositiveCoin) ToPlutusData() data.PlutusData {
	return data.NewInteger(new(big.Int).SetUint64(uint64(c)))
}

type Value struct {
	Coin         uint64
	AssetsMint   *lcommon.MultiAsset[lcommon.MultiAssetTypeMint]
	AssetsOutput *lcommon.MultiAsset[lcommon.MultiAssetTypeOutput]
}

type ResolvedInput lcommon.Utxo

func (r ResolvedInput) ToPlutusData() data.PlutusData {
	return data.NewConstr(
		0,
		r.Id.ToPlutusData(),
		r.Output.ToPlutusData(),
	)
}

type Redeemer struct {
	Tag     lcommon.RedeemerTag
	Index   uint32
	Data    data.PlutusData
	ExUnits lcommon.ExUnits
}

func (r Redeemer) ToPlutusData() data.PlutusData {
	return r.Data
}

type WithWrappedTransactionId struct {
	Value any
}

func (w WithWrappedTransactionId) ToPlutusData() data.PlutusData {
	switch v := w.Value.(type) {
	case []lcommon.Utxo:
		tmpItems := make([]data.PlutusData, len(v))
		for i := range v {
			tmpItems[i] = WithWrappedTransactionId{
				v[i],
			}.ToPlutusData()
		}
		return data.NewList(tmpItems...)
	case lcommon.Utxo:
		return data.NewConstr(
			0,
			data.NewConstr(
				0,
				data.NewByteString(v.Id.Id().Bytes()),
			),
			v.Output.ToPlutusData(),
		)
	case []ResolvedInput:
		tmpItems := make([]data.PlutusData, len(v))
		for i := range v {
			tmpItems[i] = WithWrappedTransactionId{
				v[i],
			}.ToPlutusData()
		}
		return data.NewList(tmpItems...)
	case ResolvedInput:
		return data.NewConstr(
			0,
			data.NewConstr(
				0,
				data.NewByteString(v.Id.Id().Bytes()),
			),
			v.Output.ToPlutusData(),
		)
	case lcommon.TransactionInput:
		return data.NewConstr(
			0,
			data.NewConstr(
				0,
				data.NewByteString(
					v.Id().Bytes(),
				),
			),
			data.NewInteger(
				new(big.Int).SetUint64(uint64(v.Index())),
			),
		)
	case KeyValuePairs[ScriptPurpose, Redeemer]:
		tmpPairs := make([][2]data.PlutusData, len(v))
		for i := range v {
			tmpPairs[i] = [2]data.PlutusData{
				WithWrappedTransactionId{
					v[i].Key,
				}.ToPlutusData(),
				v[i].Value.ToPlutusData(),
			}
		}
		return data.NewMap(tmpPairs)
	case ScriptPurpose:
		// NOTE: This is a _small_ abuse of the 'WithWrappedTransactionId'. We know the wrapped
		// is needed for V1 and V2, and it also appears that for V1 and V2, the certifying
		// purpose mustn't include the certificate index. So, we also short-circuit it here.
		switch p := v.(type) {
		case ScriptPurposeMinting:
			return p.ToPlutusData()
		case ScriptPurposeSpending:
			return data.NewConstr(
				1,
				WithWrappedTransactionId{
					p.Input.Id,
				}.ToPlutusData(),
			)
		case ScriptPurposeRewarding:
			return data.NewConstr(
				2,
				WithWrappedStakeCredential{
					p.StakeCredential,
				}.ToPlutusData(),
			)
		case ScriptPurposeCertifying:
			return data.NewConstr(
				3,
				WithPartialCertificates{
					p.Certificate,
				}.ToPlutusData(),
			)
		}
	}
	return nil
}

type WithWrappedStakeCredential struct {
	Value any
}

func (w WithWrappedStakeCredential) ToPlutusData() data.PlutusData {
	switch v := w.Value.(type) {
	case KeyValuePairs[*lcommon.Address, uint64]:
		tmpPairs := make([][2]data.PlutusData, len(v))
		for i := range v {
			tmpPairs[i] = [2]data.PlutusData{
				data.NewConstr(
					0,
					v[i].Key.ToPlutusData(),
				),
				toPlutusData(v[i].Value),
			}
		}
		return data.NewMap(tmpPairs)
	case Pairs[*lcommon.Address, uint64]:
		tmpItems := make([]data.PlutusData, len(v))
		for i := range v {
			tmpItems[i] = data.NewList(
				data.NewConstr(
					0,
					v[i].T1.ToPlutusData(),
				),
				toPlutusData(v[i].T2),
			)
		}
		return data.NewList(tmpItems...)
	case lcommon.Credential:
		return data.NewConstr(
			0,
			v.ToPlutusData(),
		)
	}
	return nil
}

type WithOptionDatum struct {
	Value any
}

func (w WithOptionDatum) ToPlutusData() data.PlutusData {
	switch v := w.Value.(type) {
	case WithZeroAdaAsset:
		switch v2 := v.Value.(type) {
		case []lcommon.TransactionOutput:
			tmpItems := make([]data.PlutusData, len(v2))
			for i := range v2 {
				tmpItems[i] = WithOptionDatum{WithZeroAdaAsset{v2[i]}}.ToPlutusData()
			}
			return data.NewList(tmpItems...)
		case lcommon.TransactionOutput:
			// We need to assign to a var to call ToPlutusData() with pointer receiver below
			addr := v2.Address()
			var datumHash Option[*lcommon.Blake2b256]
			if tmp := v2.DatumHash(); tmp != nil {
				datumHash.Value = tmp
			}
			return data.NewConstr(
				0,
				addr.ToPlutusData(),
				WithZeroAdaAsset{
					Value{
						Coin:         v2.Amount(),
						AssetsOutput: v2.Assets(),
					},
				}.ToPlutusData(),
				datumHash.ToPlutusData(),
			)
		case WithWrappedTransactionId:
			switch v3 := v2.Value.(type) {
			case []ResolvedInput:
				tmpItems := make([]data.PlutusData, len(v3))
				for i := range v3 {
					tmpItems[i] = WithOptionDatum{
						WithZeroAdaAsset{
							WithWrappedTransactionId{
								v3[i],
							},
						},
					}.ToPlutusData()
				}
				return data.NewList(tmpItems...)
			case ResolvedInput:
				return data.NewConstr(
					0,
					WithWrappedTransactionId{v3.Id}.ToPlutusData(),
					WithOptionDatum{WithZeroAdaAsset{v3.Output}}.ToPlutusData(),
				)
			}
		}
	}
	return nil
}

type WithZeroAdaAsset struct {
	Value any
}

func (w WithZeroAdaAsset) ToPlutusData() data.PlutusData {
	switch v := w.Value.(type) {
	case Value:
		tmpPairs := [][2]data.PlutusData{coinToPlutusDataMapPair(v.Coin)}
		if v.AssetsOutput != nil {
			tmpPairs = slices.Concat(
				tmpPairs,
				v.AssetsOutput.ToPlutusData().(*data.Map).Pairs,
			)
		}
		if v.AssetsMint != nil {
			tmpPairs = slices.Concat(
				tmpPairs,
				v.AssetsMint.ToPlutusData().(*data.Map).Pairs,
			)
		}
		return data.NewMap(
			tmpPairs,
		)
	case uint64:
		return data.NewMap(
			[][2]data.PlutusData{coinToPlutusDataMapPair(v)},
		)
	case []lcommon.TransactionOutput:
		tmpItems := make([]data.PlutusData, len(v))
		for i := range v {
			tmpItems[i] = WithZeroAdaAsset{
				v[i],
			}.ToPlutusData()
		}
		return data.NewList(tmpItems...)
	case lcommon.TransactionOutput:
		// We need to assign to a var to call ToPlutusData() with pointer receiver below
		addr := v.Address()
		datumOption := data.NewConstr(0)
		if tmp := v.Datum(); tmp != nil {
			datumOption = data.NewConstr(
				2,
				tmp.Data.Clone(),
			)
		} else if tmp := v.DatumHash(); (tmp != nil && *tmp != lcommon.Blake2b256{}) {
			datumOption = data.NewConstr(
				1,
				tmp.ToPlutusData(),
			)
		}
		scriptRef := data.NewConstr(1)
		if tmp := v.ScriptRef(); tmp != nil {
			scriptRef = data.NewConstr(
				0,
				data.NewByteString(tmp.Hash().Bytes()),
			)
		}
		return data.NewConstr(
			0,
			addr.ToPlutusData(),
			WithZeroAdaAsset{
				Value{
					Coin:         v.Amount(),
					AssetsOutput: v.Assets(),
				},
			}.ToPlutusData(),
			datumOption,
			scriptRef,
		)
	case WithWrappedTransactionId:
		switch v2 := v.Value.(type) {
		case []ResolvedInput:
			tmpItems := make([]data.PlutusData, len(v2))
			for i := range v2 {
				tmpItems[i] = WithZeroAdaAsset{
					WithWrappedTransactionId{
						v2[i],
					},
				}.ToPlutusData()
			}
			return data.NewList(tmpItems...)
		case ResolvedInput:
			return data.NewConstr(
				0,
				WithWrappedTransactionId{
					v2.Id,
				}.ToPlutusData(),
				WithZeroAdaAsset{
					v2.Output,
				}.ToPlutusData(),
			)
		}
	}
	return nil
}

type WithPartialCertificates struct {
	Value any
}

func (w WithPartialCertificates) ToPlutusData() data.PlutusData {
	switch v := w.Value.(type) {
	case []lcommon.Certificate:
		tmpItems := make([]data.PlutusData, len(v))
		for i := range v {
			tmpItems[i] = WithPartialCertificates{
				v[i],
			}.ToPlutusData()
		}
		return data.NewList(tmpItems...)
	case lcommon.Certificate:
		switch c := v.(type) {
		case *lcommon.StakeRegistrationCertificate:
			return data.NewConstr(
				0,
				WithWrappedStakeCredential{c.StakeCredential}.ToPlutusData(),
			)
		case *lcommon.RegistrationCertificate:
			return data.NewConstr(
				0,
				WithWrappedStakeCredential{c.StakeCredential}.ToPlutusData(),
			)
		case *lcommon.StakeDeregistrationCertificate:
			return data.NewConstr(
				1,
				WithWrappedStakeCredential{c.StakeCredential}.ToPlutusData(),
			)
		case *lcommon.DeregistrationCertificate:
			return data.NewConstr(
				1,
				WithWrappedStakeCredential{c.StakeCredential}.ToPlutusData(),
			)
		case *lcommon.StakeDelegationCertificate:
			return data.NewConstr(
				2,
				WithWrappedStakeCredential{c.StakeCredential}.ToPlutusData(),
				c.PoolKeyHash.ToPlutusData(),
			)
		case *lcommon.PoolRegistrationCertificate:
			return data.NewConstr(
				3,
				toPlutusData(c.Operator),
				toPlutusData(c.VrfKeyHash),
			)
		case *lcommon.PoolRetirementCertificate:
			return data.NewConstr(
				4,
				toPlutusData(c.PoolKeyHash),
				data.NewInteger(new(big.Int).SetUint64(c.Epoch)),
			)
		}
	}
	return nil
}

func coinToPlutusDataMapPair(val uint64) [2]data.PlutusData {
	return [2]data.PlutusData{
		data.NewByteString(nil),
		data.NewMap(
			[][2]data.PlutusData{
				{
					data.NewByteString(nil),
					data.NewInteger(
						new(big.Int).SetUint64(val),
					),
				},
			},
		),
	}
}
