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

// Package bench provides benchmark utilities and test fixtures for memory
// profiling.
package bench

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// BenchLedgerState returns a mock ledger state suitable for
// benchmarks. It provides a minimal ledger state with mainnet
// network ID and a configurable UTXO lookup function.
func BenchLedgerState() common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		Build()
}

// BenchLedgerStateWithUtxos returns a mock ledger state with the provided
// UTXOs.
func BenchLedgerStateWithUtxos(utxos []common.Utxo) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		WithUtxos(utxos).
		Build()
}

// BenchLedgerStateWithUtxoFunc returns a mock ledger state with a
// custom UTXO lookup function.
func BenchLedgerStateWithUtxoFunc(
	fn func(common.TransactionInput) (common.Utxo, error),
) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		WithUtxoById(fn).
		Build()
}

// BenchProtocolParams returns a minimal ProtocolParameters suitable for
// benchmarks and profiling tests. It uses a zero-value Shelley protocol
// parameters struct with the required rational fields initialized to avoid
// nil dereferences if Utxorpc() is ever called.
func BenchProtocolParams() common.ProtocolParameters {
	zeroRat := &cbor.Rat{Rat: big.NewRat(0, 1)}
	return &shelley.ShelleyProtocolParameters{
		A0:  zeroRat,
		Rho: zeroRat,
		Tau: zeroRat,
	}
}

// BlockFixture contains a pre-loaded block for benchmarking.
type BlockFixture struct {
	Name      string
	Era       string
	BlockType uint
	Cbor      []byte
	Block     common.Block
}

// LoadBlockFixture loads a test block for the given era.
// The era should be one of: "byron", "shelley", "allegra",
// "mary", "alonzo", "babbage", "conway". The name parameter
// is currently unused but reserved for future fixture variants.
func LoadBlockFixture(era, name string) (*BlockFixture, error) {
	blockType, err := BlockTypeFromEra(era)
	if err != nil {
		return nil, err
	}

	blockCbor, err := blockCborFromEra(era)
	if err != nil {
		return nil, err
	}

	block, err := ledger.NewBlockFromCbor(blockType, blockCbor)
	if err != nil {
		return nil, fmt.Errorf("decode %s block: %w", era, err)
	}

	return &BlockFixture{
		Name:      name,
		Era:       era,
		BlockType: blockType,
		Cbor:      blockCbor,
		Block:     block,
	}, nil
}

// MustLoadBlockFixture loads a test block and panics on error.
// Use this in benchmark init() or setup code.
func MustLoadBlockFixture(era, name string) *BlockFixture {
	fixture, err := LoadBlockFixture(era, name)
	if err != nil {
		panic(fmt.Sprintf("failed to load %s block fixture: %v", era, err))
	}
	return fixture
}

// eraInfo holds the block type, transaction type, and block hex for an era.
type eraInfo struct {
	blockType uint
	txType    uint
	blockHex  string
}

// eraLookup maps lowercase era names to their type constants and test data.
var eraLookup = map[string]eraInfo{
	"byron":   {ledger.BlockTypeByronMain, ledger.TxTypeByron, testdata.ByronBlockHex},
	"shelley": {ledger.BlockTypeShelley, ledger.TxTypeShelley, testdata.ShelleyBlockHex},
	"allegra": {ledger.BlockTypeAllegra, ledger.TxTypeAllegra, testdata.AllegraBlockHex},
	"mary":    {ledger.BlockTypeMary, ledger.TxTypeMary, testdata.MaryBlockHex},
	"alonzo":  {ledger.BlockTypeAlonzo, ledger.TxTypeAlonzo, testdata.AlonzoBlockHex},
	"babbage": {ledger.BlockTypeBabbage, ledger.TxTypeBabbage, testdata.BabbageBlockHex},
	"conway":  {ledger.BlockTypeConway, ledger.TxTypeConway, testdata.ConwayBlockHex},
}

// BlockTypeFromEra returns the ledger block type constant for the given era
// name.
func BlockTypeFromEra(era string) (uint, error) {
	info, ok := eraLookup[strings.ToLower(era)]
	if !ok {
		return 0, fmt.Errorf("unknown era: %s", era)
	}
	return info.blockType, nil
}

// TxTypeFromEra returns the ledger transaction type constant
// for the given era name.
func TxTypeFromEra(era string) (uint, error) {
	info, ok := eraLookup[strings.ToLower(era)]
	if !ok {
		return 0, fmt.Errorf("unknown era: %s", era)
	}
	return info.txType, nil
}

// supportedEraNames is the authoritative ordered list of era names.
// Update this list when adding new eras to eraLookup.
var supportedEraNames = []string{
	"byron",
	"shelley",
	"allegra",
	"mary",
	"alonzo",
	"babbage",
	"conway",
}

// EraNames returns the list of supported era names for benchmarking.
// The explicit ordering matters for consistent benchmark output.
func EraNames() []string {
	return supportedEraNames
}

// PostByronEraNames returns era names excluding Byron (for Praos-era
// benchmarks).
func PostByronEraNames() []string {
	return supportedEraNames[1:]
}

// blockCborFromEra returns the raw CBOR bytes for the given
// era's test block.
func blockCborFromEra(era string) ([]byte, error) {
	info, ok := eraLookup[strings.ToLower(era)]
	if !ok {
		return nil, fmt.Errorf("unknown era: %s", era)
	}
	return testdata.MustDecodeHex(info.blockHex), nil
}
