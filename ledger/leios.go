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

package ledger

import "github.com/blinklabs-io/gouroboros/ledger/leios"

// The below are compatibility types, constants, and functions for the Leios era
// to keep existing code working after a refactor of the ledger package

// Leios types
type (
	LeiosBlockHeader             = leios.LeiosBlockHeader
	LeiosEndorderBlock           = leios.LeiosEndorserBlock
	LeiosRankingBlock            = leios.LeiosRankingBlock
	LeiosTransaction             = leios.LeiosTransaction
	LeiosTransactionBody         = leios.LeiosTransactionBody
	LeiosTransactionWitnessSet   = leios.LeiosTransactionWitnessSet
	LeiosProtocolParameters      = leios.LeiosProtocolParameters
	LeiosProtocolParameterUpdate = leios.LeiosProtocolParameterUpdate
)

// Leios constants
const (
	EraIdLeios             = leios.EraIdLeios
	BlockTypeLeiosRanking  = leios.BlockTypeLeiosRanking
	BlockTypeLeiosEndorser = leios.BlockTypeLeiosEndorser
	BlockHeaderTypeLeios   = leios.BlockHeaderTypeLeios
	TxTypeLeios            = leios.TxTypeLeios
)

// Leios functions
var (
	NewLeiosEndorserBlockFromCbor   = leios.NewLeiosEndorserBlockFromCbor
	NewLeiosRankingBlockFromCbor    = leios.NewLeiosRankingBlockFromCbor
	NewLeiosBlockHeaderFromCbor     = leios.NewLeiosBlockHeaderFromCbor
	NewLeiosTransactionFromCbor     = leios.NewLeiosTransactionFromCbor
	NewLeiosTransactionBodyFromCbor = leios.NewLeiosTransactionBodyFromCbor
)
