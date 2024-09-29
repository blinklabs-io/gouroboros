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

package ledger

import "github.com/blinklabs-io/gouroboros/ledger/babbage"

// The below are compatibility types, constants, and functions for the Babbage era
// to keep existing code working after a refactor of the ledger package

// Babbage types
type BabbageBlock = babbage.BabbageBlock
type BabbageBlockHeader = babbage.BabbageBlockHeader
type BabbageTransaction = babbage.BabbageTransaction
type BabbageTransactionBody = babbage.BabbageTransactionBody
type BabbageTransactionOutput = babbage.BabbageTransactionOutput
type BabbageTransactionWitnessSet = babbage.BabbageTransactionWitnessSet
type BabbageProtocolParameters = babbage.BabbageProtocolParameters
type BabbageProtocolParameterUpdate = babbage.BabbageProtocolParameterUpdate

// Babbage constants
const (
	EraIdBabbage           = babbage.EraIdBabbage
	BlockTypeBabbage       = babbage.BlockTypeBabbage
	BlockHeaderTypeBabbage = babbage.BlockHeaderTypeBabbage
	TxTypeBabbage          = babbage.TxTypeBabbage
)

// Babbage functions
var (
	NewBabbageBlockFromCbor             = babbage.NewBabbageBlockFromCbor
	NewBabbageBlockHeaderFromCbor       = babbage.NewBabbageBlockHeaderFromCbor
	NewBabbageTransactionFromCbor       = babbage.NewBabbageTransactionFromCbor
	NewBabbageTransactionBodyFromCbor   = babbage.NewBabbageTransactionBodyFromCbor
	NewBabbageTransactionOutputFromCbor = babbage.NewBabbageTransactionOutputFromCbor
)
