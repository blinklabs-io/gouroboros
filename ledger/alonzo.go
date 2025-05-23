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

import "github.com/blinklabs-io/gouroboros/ledger/alonzo"

// The below are compatibility types, constants, and functions for the Alonzo era
// to keep existing code working after a refactor of the ledger package

// Alonzo types
type (
	AlonzoBlock                   = alonzo.AlonzoBlock
	AlonzoBlockHeader             = alonzo.AlonzoBlockHeader
	AlonzoTransaction             = alonzo.AlonzoTransaction
	AlonzoTransactionBody         = alonzo.AlonzoTransactionBody
	AlonzoTransactionOutput       = alonzo.AlonzoTransactionOutput
	AlonzoTransactionWitnessSet   = alonzo.AlonzoTransactionWitnessSet
	AlonzoProtocolParameters      = alonzo.AlonzoProtocolParameters
	AlonzoProtocolParameterUpdate = alonzo.AlonzoProtocolParameterUpdate
)

// Alonzo constants
const (
	EraIdAlonzo           = alonzo.EraIdAlonzo
	BlockTypeAlonzo       = alonzo.BlockTypeAlonzo
	BlockHeaderTypeAlonzo = alonzo.BlockHeaderTypeAlonzo
	TxTypeAlonzo          = alonzo.TxTypeAlonzo
)

// Alonzo functions
var (
	NewAlonzoBlockFromCbor             = alonzo.NewAlonzoBlockFromCbor
	NewAlonzoBlockHeaderFromCbor       = alonzo.NewAlonzoBlockHeaderFromCbor
	NewAlonzoTransactionFromCbor       = alonzo.NewAlonzoTransactionFromCbor
	NewAlonzoTransactionBodyFromCbor   = alonzo.NewAlonzoTransactionBodyFromCbor
	NewAlonzoTransactionOutputFromCbor = alonzo.NewAlonzoTransactionOutputFromCbor
)
