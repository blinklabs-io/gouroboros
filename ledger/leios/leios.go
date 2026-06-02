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

package leios

import "github.com/blinklabs-io/gouroboros/ledger/dijkstra"

// Deprecated: Leios is not a ledger era. Leios features are delivered in the
// Dijkstra era, so this package is a source-compatibility shim only.

type (
	LeiosBlockHeader             = dijkstra.DijkstraBlockHeader
	LeiosCertificate             = dijkstra.DijkstraLeiosCertificate
	LeiosEndorserBlock           = dijkstra.DijkstraBlock
	LeiosRankingBlock            = dijkstra.DijkstraBlock
	LeiosTransaction             = dijkstra.DijkstraTransaction
	LeiosTransactionBody         = dijkstra.DijkstraTransactionBody
	LeiosTransactionWitnessSet   = dijkstra.DijkstraTransactionWitnessSet
	LeiosGenesis                 = dijkstra.DijkstraGenesis
	LeiosProtocolParameters      = dijkstra.DijkstraProtocolParameters
	LeiosProtocolParameterUpdate = dijkstra.DijkstraProtocolParameterUpdate
)

const (
	EraIdLeios           = dijkstra.EraIdDijkstra
	EraNameLeios         = dijkstra.EraNameDijkstra
	BlockHeaderTypeLeios = dijkstra.BlockHeaderTypeDijkstra
	TxTypeLeios          = dijkstra.TxTypeDijkstra

	// Compatibility constants retain the old Leios block-type values. New
	// on-wire Leios/Dijkstra blocks use dijkstra.BlockTypeDijkstra.
	BlockTypeLeiosRanking  = 8
	BlockTypeLeiosEndorser = 9
)

var (
	EraLeios = dijkstra.EraDijkstra

	NewLeiosRankingBlockFromCbor    = dijkstra.NewDijkstraBlockFromCbor
	NewLeiosEndorserBlockFromCbor   = dijkstra.NewDijkstraBlockFromCbor
	NewLeiosBlockHeaderFromCbor     = dijkstra.NewDijkstraBlockHeaderFromCbor
	NewLeiosTransactionFromCbor     = dijkstra.NewDijkstraTransactionFromCbor
	NewLeiosTransactionBodyFromCbor = dijkstra.NewDijkstraTransactionBodyFromCbor
	NewLeiosGenesisFromReader       = dijkstra.NewDijkstraGenesisFromReader
	NewLeiosGenesisFromFile         = dijkstra.NewDijkstraGenesisFromFile
)
