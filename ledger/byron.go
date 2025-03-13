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

import "github.com/blinklabs-io/gouroboros/ledger/byron"

// The below are compatibility types, constants, and functions for the Byron era
// to keep existing code working after a refactor of the ledger package

// Byron types
type (
	ByronEpochBoundaryBlock      = byron.ByronEpochBoundaryBlock
	ByronMainBlock               = byron.ByronMainBlock
	ByronEpochBounaryBlockHeader = byron.ByronEpochBoundaryBlockHeader
	ByronMainBlockHeader         = byron.ByronMainBlockHeader
	ByronTransaction             = byron.ByronTransaction
	ByronTransactionInput        = byron.ByronTransactionInput
	ByronTransactionOutput       = byron.ByronTransactionOutput
)

// Byron constants
const (
	EraIdByron           = byron.EraIdByron
	BlockTypeByronEbb    = byron.BlockTypeByronEbb
	BlockTypeByronMain   = byron.BlockTypeByronMain
	BlockHeaderTypeByron = byron.BlockHeaderTypeByron
	TxTypeByron          = byron.TxTypeByron
)

// Byron functions
var (
	NewByronEpochBoundaryBlockFromCbor       = byron.NewByronEpochBoundaryBlockFromCbor
	NewByronMainBlockFromCbor                = byron.NewByronMainBlockFromCbor
	NewByronEpochBoundaryBlockHeaderFromCbor = byron.NewByronEpochBoundaryBlockHeaderFromCbor
	NewByronMainBlockHeaderFromCbor          = byron.NewByronMainBlockHeaderFromCbor
	NewByronTransactionInput                 = byron.NewByronTransactionInput
	NewByronTransactionFromCbor              = byron.NewByronTransactionFromCbor
)
