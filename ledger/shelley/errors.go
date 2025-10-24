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

package shelley

import (
	"fmt"
	"strings"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

type ExpiredUtxoError struct {
	Ttl  uint64
	Slot uint64
}

func (e ExpiredUtxoError) Error() string {
	return fmt.Sprintf(
		"expired UTxO: TTL %d, slot %d",
		e.Ttl,
		e.Slot,
	)
}

type InputSetEmptyUtxoError struct{}

func (InputSetEmptyUtxoError) Error() string {
	return "input set empty"
}

type FeeTooSmallUtxoError struct {
	Provided uint64
	Min      uint64
}

func (e FeeTooSmallUtxoError) Error() string {
	return fmt.Sprintf(
		"fee too small: provided %d, minimum %d",
		e.Provided,
		e.Min,
	)
}

type BadInputsUtxoError struct {
	Inputs []common.TransactionInput
}

func (e BadInputsUtxoError) Error() string {
	tmpInputs := make([]string, len(e.Inputs))
	for idx, tmpInput := range e.Inputs {
		tmpInputs[idx] = tmpInput.String()
	}
	return "bad input(s): " + strings.Join(tmpInputs, ", ")
}

type WrongNetworkError struct {
	NetId uint
	Addrs []common.Address
}

func (e WrongNetworkError) Error() string {
	tmpAddrs := make([]string, len(e.Addrs))
	for idx, tmpAddr := range e.Addrs {
		tmpAddrs[idx] = tmpAddr.String()
	}
	return "wrong network: " + strings.Join(tmpAddrs, ", ")
}

type WrongNetworkWithdrawalError struct {
	NetId uint
	Addrs []common.Address
}

func (e WrongNetworkWithdrawalError) Error() string {
	tmpAddrs := make([]string, len(e.Addrs))
	for idx, tmpAddr := range e.Addrs {
		tmpAddrs[idx] = tmpAddr.String()
	}
	return "wrong network withdrawals: " + strings.Join(tmpAddrs, ", ")
}

type ValueNotConservedUtxoError struct {
	Consumed uint64
	Produced uint64
}

func (e ValueNotConservedUtxoError) Error() string {
	return fmt.Sprintf(
		"value not conserved: consumed %d, produced %d",
		e.Consumed,
		e.Produced,
	)
}

type OutputTooSmallUtxoError struct {
	Outputs []common.TransactionOutput
}

func (e OutputTooSmallUtxoError) Error() string {
	tmpOutputs := make([]string, len(e.Outputs))
	for idx, tmpOutput := range e.Outputs {
		tmpOutputs[idx] = tmpOutput.String()
	}
	return "output too small: " + strings.Join(tmpOutputs, ", ")
}

type OutputBootAddrAttrsTooBigError struct {
	Outputs []common.TransactionOutput
}

func (e OutputBootAddrAttrsTooBigError) Error() string {
	tmpOutputs := make([]string, len(e.Outputs))
	for idx, tmpOutput := range e.Outputs {
		tmpOutputs[idx] = tmpOutput.String()
	}
	return "output bootstrap address attributes too big: " + strings.Join(
		tmpOutputs,
		", ",
	)
}

type MaxTxSizeUtxoError struct {
	TxSize    uint
	MaxTxSize uint
}

func (e MaxTxSizeUtxoError) Error() string {
	return fmt.Sprintf(
		"transaction size too large: size %d, max %d",
		e.TxSize,
		e.MaxTxSize,
	)
}
