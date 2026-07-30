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

package ledger

import "github.com/blinklabs-io/gouroboros/ledger/dijkstra"

// The below are compatibility types, constants, and functions for the Dijkstra
// era to keep existing code working after a refactor of the ledger package.

// Dijkstra types
type (
	DijkstraBlock                   = dijkstra.DijkstraBlock
	DijkstraBlockBody               = dijkstra.DijkstraBlockBody
	DijkstraBlockHeader             = dijkstra.DijkstraBlockHeader
	DijkstraLeiosCertificate        = dijkstra.DijkstraLeiosCertificate
	DijkstraTransaction             = dijkstra.DijkstraTransaction
	DijkstraTransactionBody         = dijkstra.DijkstraTransactionBody
	DijkstraTransactionOutput       = dijkstra.DijkstraTransactionOutput
	DijkstraTransactionWitnessSet   = dijkstra.DijkstraTransactionWitnessSet
	DijkstraGenesis                 = dijkstra.DijkstraGenesis
	DijkstraProtocolParameters      = dijkstra.DijkstraProtocolParameters
	DijkstraProtocolParameterUpdate = dijkstra.DijkstraProtocolParameterUpdate
)

// Dijkstra constants
const (
	EraIdDijkstra           = dijkstra.EraIdDijkstra
	BlockTypeDijkstra       = dijkstra.BlockTypeDijkstra
	BlockHeaderTypeDijkstra = dijkstra.BlockHeaderTypeDijkstra
	TxTypeDijkstra          = dijkstra.TxTypeDijkstra
)

// Dijkstra functions
var (
	NewDijkstraBlockFromCbor           = dijkstra.NewDijkstraBlockFromCbor
	NewDijkstraBlockHeaderFromCbor     = dijkstra.NewDijkstraBlockHeaderFromCbor
	NewDijkstraTransactionFromCbor     = dijkstra.NewDijkstraTransactionFromCbor
	NewDijkstraTransactionBodyFromCbor = dijkstra.NewDijkstraTransactionBodyFromCbor
	NewDijkstraGenesisFromReader       = dijkstra.NewDijkstraGenesisFromReader
	NewDijkstraGenesisFromFile         = dijkstra.NewDijkstraGenesisFromFile
)
