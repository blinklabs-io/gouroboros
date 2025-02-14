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

import "github.com/blinklabs-io/gouroboros/ledger/mary"

// The below are compatibility types, constants, and functions for the Mary era
// to keep existing code working after a refactor of the ledger package

// Mary types
type MaryBlock = mary.MaryBlock
type MaryBlockHeader = mary.MaryBlockHeader
type MaryTransaction = mary.MaryTransaction
type MaryTransactionBody = mary.MaryTransactionBody
type MaryTransactionOutput = mary.MaryTransactionOutput
type MaryTransactionOutputValue = mary.MaryTransactionOutputValue
type MaryProtocolParameters = mary.MaryProtocolParameters
type MaryProtocolParameterUpdate = mary.MaryProtocolParameterUpdate

// Mary constants
const (
	EraIdMary           = mary.EraIdMary
	BlockTypeMary       = mary.BlockTypeMary
	BlockHeaderTypeMary = mary.BlockHeaderTypeMary
	TxTypeMary          = mary.TxTypeMary
)

// Mary functions
var (
	NewMaryBlockFromCbor             = mary.NewMaryBlockFromCbor
	NewMaryTransactionFromCbor       = mary.NewMaryTransactionFromCbor
	NewMaryTransactionBodyFromCbor   = mary.NewMaryTransactionBodyFromCbor
	NewMaryTransactionOutputFromCbor = mary.NewMaryTransactionOutputFromCbor
)
