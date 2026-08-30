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

package conway

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

const (
	// MaxRefScriptSizePerTx is the Conway transaction limit from the ledger
	// specification. Dijkstra moves this value into protocol parameters.
	MaxRefScriptSizePerTx uint64 = 200 * 1024
	// MaxRefScriptSizePerBlock is the Conway block limit from the ledger
	// specification. Dijkstra moves this value into protocol parameters.
	MaxRefScriptSizePerBlock uint64 = 1024 * 1024
	// RefScriptCostStride is the fixed Conway reference-script fee tier size.
	RefScriptCostStride uint64 = 25_600
)

func UtxoValidateRefScriptSizePerTx(
	tx common.Transaction,
	slot uint64,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	if _, ok := pp.(*ConwayProtocolParameters); !ok {
		return errors.New("pparams are not expected type")
	}
	totalSize, err := common.ConsumedReferenceScriptSize(tx, ls)
	if err != nil {
		return err
	}
	if totalSize > MaxRefScriptSizePerTx {
		return common.RefScriptSizePerTxTooLargeError{
			TxSize:  totalSize,
			MaxSize: MaxRefScriptSizePerTx,
		}
	}
	return nil
}

// ValidateRefScriptSizePerBlock checks the Conway block limit. The optional
// state parameter keeps the original two-argument calling shape usable for
// publishing-only blocks while allowing consumed scripts to be resolved.
func ValidateRefScriptSizePerBlock(
	block *ConwayBlock,
	pp common.ProtocolParameters,
	utxoStates ...common.UtxoState,
) error {
	conwayPparams, ok := pp.(*ConwayProtocolParameters)
	if !ok {
		return errors.New("pparams are not expected type")
	}
	if len(utxoStates) > 1 {
		return errors.New("expected at most one ledger state")
	}
	var utxoState common.UtxoState
	if len(utxoStates) == 1 {
		utxoState = utxoStates[0]
	}
	totalSize, err := common.ConsumedReferenceScriptSizePerBlock(
		block,
		utxoState,
		conwayPparams.ProtocolVersion.Major >= common.ProtocolVersionVanRossem,
		nil,
	)
	if err != nil {
		return err
	}
	if totalSize > MaxRefScriptSizePerBlock {
		return common.RefScriptSizePerBlockTooLargeError{
			BlockSize: totalSize,
			MaxSize:   MaxRefScriptSizePerBlock,
		}
	}
	return nil
}

// CalculateRefScriptFee calculates the tiered reference-script fee and rounds
// the exact rational result up once, after all tiers have been accumulated.
func CalculateRefScriptFee(
	scriptSize uint64,
	baseCost *cbor.Rat,
	stride uint64,
	multiplier *cbor.Rat,
) (uint64, error) {
	if scriptSize == 0 {
		return 0, nil
	}
	if baseCost == nil || baseCost.Rat == nil || baseCost.Sign() < 0 {
		return 0, errors.New("invalid reference-script base cost")
	}
	if stride == 0 {
		return 0, errors.New("reference-script cost stride must be greater than zero")
	}
	if multiplier == nil || multiplier.Rat == nil || multiplier.Sign() <= 0 {
		return 0, errors.New("invalid reference-script cost multiplier")
	}
	price := new(big.Rat).Set(baseCost.Rat)
	total := new(big.Rat)
	remaining := scriptSize
	for remaining > 0 {
		tierSize := min(remaining, stride)
		tierCost := new(big.Rat).Mul(
			price,
			new(big.Rat).SetInt(new(big.Int).SetUint64(tierSize)),
		)
		total.Add(total, tierCost)
		remaining -= tierSize
		price.Mul(price, multiplier.Rat)
	}
	fee, remainder := new(big.Int).QuoRem(
		total.Num(),
		total.Denom(),
		new(big.Int),
	)
	if remainder.Sign() != 0 {
		fee.Add(fee, big.NewInt(1))
	}
	if !fee.IsUint64() {
		return 0, fmt.Errorf("reference-script fee overflow: %s", fee)
	}
	return fee.Uint64(), nil
}

// MinFeeTxWithRefScriptSize adds the Conway tiered reference-script fee to
// the size-based transaction fee. Callers that already resolved the UTxO set
// can use this function without repeating the lookup.
func MinFeeTxWithRefScriptSize(
	tx common.Transaction,
	pparams common.ProtocolParameters,
	scriptSize uint64,
) (uint64, error) {
	conwayPparams, ok := pparams.(*ConwayProtocolParameters)
	if !ok {
		return 0, errors.New("pparams are not expected type")
	}
	baseFee, err := MinFeeTx(tx, pparams)
	if err != nil {
		return 0, err
	}
	refScriptFee, err := CalculateRefScriptFee(
		scriptSize,
		conwayPparams.MinFeeRefScriptCostPerByte,
		RefScriptCostStride,
		&cbor.Rat{Rat: big.NewRat(6, 5)},
	)
	if err != nil {
		return 0, err
	}
	if baseFee > math.MaxUint64-refScriptFee {
		return 0, errors.New("minimum transaction fee overflow")
	}
	return baseFee + refScriptFee, nil
}

// MinFeeTxWithUtxo calculates the Conway minimum fee using the same consumed
// reference-script set as the transaction and block size limits.
func MinFeeTxWithUtxo(
	tx common.Transaction,
	pparams common.ProtocolParameters,
	utxoState common.UtxoState,
) (uint64, error) {
	scriptSize, err := common.ConsumedReferenceScriptSize(tx, utxoState)
	if err != nil {
		return 0, err
	}
	return MinFeeTxWithRefScriptSize(tx, pparams, scriptSize)
}
