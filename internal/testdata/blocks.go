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

// Package testdata provides shared test block data for benchmarks and tests.
package testdata

import (
	_ "embed"
	"encoding/hex"
	"strings"

	"github.com/blinklabs-io/gouroboros/ledger"
)

// Byron block from mainnet
// https://cexplorer.io/block/1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
// Slot: 4471207
// Hash: 1451a0dbf16cfeddf4991a838961df1b08a68f43a19c0eb3b36cc4029c77a2d8
//
//go:embed byron_block.hex
var ByronBlockHex string

// Shelley block from mainnet
// https://cexplorer.io/block/2308cdd4c0bf8b8bf92523bdd1dd31640c0f42ff079d985fcc07c36cbf915c2b
// Slot: 16156972
// Hash: 2308cdd4c0bf8b8bf92523bdd1dd31640c0f42ff079d985fcc07c36cbf915c2b
//
//go:embed shelley_block.hex
var ShelleyBlockHex string

// Allegra block from mainnet
// https://cexplorer.io/block/8115134ab013f6a5fd88fd2a10825177a2eedcde31cb2f1f35e492df469cf9a8
// Slot: 23068573
// Hash: 8115134ab013f6a5fd88fd2a10825177a2eedcde31cb2f1f35e492df469cf9a8
//
//go:embed allegra_block.hex
var AllegraBlockHex string

// Mary block from mainnet
// https://cexplorer.io/block/d36ab36f451e9fcbd4247daef45ce5be9a4b918fce5ee97a63b8aeac606fca03
// Slot: 39916670
// Hash: d36ab36f451e9fcbd4247daef45ce5be9a4b918fce5ee97a63b8aeac606fca03
//
//go:embed mary_block.hex
var MaryBlockHex string

// Alonzo block from mainnet
// https://cexplorer.io/block/1d7974cb01cc9e3fbe9dd7594795a36b21cb1deb2f1b70a0625332c91bd7e5a7
// Slot: 72316767
// Hash: 1d7974cb01cc9e3fbe9dd7594795a36b21cb1deb2f1b70a0625332c91bd7e5a7
//
//go:embed alonzo_block.hex
var AlonzoBlockHex string

// Babbage block from mainnet
// https://cexplorer.io/block/db19fcfaba30607e363113b0a13616e6a9da5aa48b86ec2c033786f0a2e13f7d
// Slot: 76204984
// Hash: db19fcfaba30607e363113b0a13616e6a9da5aa48b86ec2c033786f0a2e13f7d
//
//go:embed babbage_block.hex
var BabbageBlockHex string

// Conway block from mainnet
// https://cexplorer.io/block/27807a70215e3e018eec9be8c619c692e06a78ebcb63daf90d7abe823f3bbf47
// Slot: 159835207
// Hash: 27807a70215e3e018eec9be8c619c692e06a78ebcb63daf90d7abe823f3bbf47
//
//go:embed conway_block.hex
var ConwayBlockHex string

// TestBlock contains block data for testing.
type TestBlock struct {
	Name      string
	BlockType uint
	Cbor      []byte
}

// GetTestBlocks returns a slice of test blocks for various eras.
// The blocks are real mainnet blocks extracted from ledger test files.
func GetTestBlocks() []TestBlock {
	return []TestBlock{
		{Name: "Byron", BlockType: ledger.BlockTypeByronMain, Cbor: MustDecodeHex(ByronBlockHex)},
		{Name: "Shelley", BlockType: ledger.BlockTypeShelley, Cbor: MustDecodeHex(ShelleyBlockHex)},
		{Name: "Allegra", BlockType: ledger.BlockTypeAllegra, Cbor: MustDecodeHex(AllegraBlockHex)},
		{Name: "Mary", BlockType: ledger.BlockTypeMary, Cbor: MustDecodeHex(MaryBlockHex)},
		{Name: "Alonzo", BlockType: ledger.BlockTypeAlonzo, Cbor: MustDecodeHex(AlonzoBlockHex)},
		{Name: "Babbage", BlockType: ledger.BlockTypeBabbage, Cbor: MustDecodeHex(BabbageBlockHex)},
		{Name: "Conway", BlockType: ledger.BlockTypeConway, Cbor: MustDecodeHex(ConwayBlockHex)},
	}
}

// MustDecodeHex decodes a hex string to bytes, panicking on error.
func MustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil {
		panic(err)
	}
	return b
}
