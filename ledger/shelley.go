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

import "github.com/blinklabs-io/gouroboros/ledger/shelley"

// The below are compatibility types, constants, and functions for the Shelley era
// to keep existing code working after a refactor of the ledger package

// Shelley types
type ShelleyBlock = shelley.ShelleyBlock
type ShelleyBlockHeader = shelley.ShelleyBlockHeader
type ShelleyTransaction = shelley.ShelleyTransaction
type ShelleyTransactionBody = shelley.ShelleyTransactionBody
type ShelleyTransactionInput = shelley.ShelleyTransactionInput
type ShelleyTransactionOutput = shelley.ShelleyTransactionOutput
type ShelleyTransactionWitnessSet = shelley.ShelleyTransactionWitnessSet
type ShelleyProtocolParameters = shelley.ShelleyProtocolParameters
type ShelleyProtocolParameterUpdate = shelley.ShelleyProtocolParameterUpdate

// Shelley constants
const (
	EraIdShelley           = shelley.EraIdShelley
	BlockTypeShelley       = shelley.BlockTypeShelley
	BlockHeaderTypeShelley = shelley.BlockHeaderTypeShelley
	TxTypeShelley          = shelley.TxTypeShelley
)

// Shelley functions
var (
	NewShelleyBlockFromCbor             = shelley.NewShelleyBlockFromCbor
	NewShelleyBlockHeaderFromCbor       = shelley.NewShelleyBlockHeaderFromCbor
	NewShelleyTransactionInput          = shelley.NewShelleyTransactionInput
	NewShelleyTransactionFromCbor       = shelley.NewShelleyTransactionFromCbor
	NewShelleyTransactionBodyFromCbor   = shelley.NewShelleyTransactionBodyFromCbor
	NewShelleyTransactionOutputFromCbor = shelley.NewShelleyTransactionOutputFromCbor
)
