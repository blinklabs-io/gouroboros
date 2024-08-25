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

import "github.com/blinklabs-io/gouroboros/ledger/allegra"

// The below are compatability types, constants, and functions for the Allegra era
// to keep existing code working after a refactor of the ledger package

// Allegra types
type AllegraBlock = allegra.AllegraBlock
type AllegraTransaction = allegra.AllegraTransaction
type AllegraTransactionBody = allegra.AllegraTransactionBody
type AllegraProtocolParameters = allegra.AllegraProtocolParameters
type AllegraProtocolParameterUpdate = allegra.AllegraProtocolParameterUpdate

// Allegra constants
const (
	EraIdAllegra           = allegra.EraIdAllegra
	BlockTypeAllegra       = allegra.BlockTypeAllegra
	BlockHeaderTypeAllegra = allegra.BlockHeaderTypeAllegra
	TxTypeAllegra          = allegra.TxTypeAllegra
)

// Allegra functions
var (
	NewAllegraBlockFromCbor           = allegra.NewAllegraBlockFromCbor
	NewAllegraTransactionFromCbor     = allegra.NewAllegraTransactionFromCbor
	NewAllegraTransactionBodyFromCbor = allegra.NewAllegraTransactionBodyFromCbor
)
