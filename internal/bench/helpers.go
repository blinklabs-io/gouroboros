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
	"strings"

	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
)

// BenchLedgerState returns a mock ledger state suitable for benchmarks.
// It provides a minimal ledger state with mainnet network ID and a configurable
// UTXO lookup function.
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

// BenchLedgerStateWithUtxoFunc returns a mock ledger state with a custom UTXO
// lookup function.
func BenchLedgerStateWithUtxoFunc(
	fn func(common.TransactionInput) (common.Utxo, error),
) common.LedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithNetworkId(common.AddressNetworkMainnet).
		WithUtxoById(fn).
		Build()
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
// The era should be one of: "byron", "shelley", "allegra", "mary", "alonzo",
// "babbage", "conway". The name parameter is currently unused but reserved for
// future fixture variants.
func LoadBlockFixture(era, name string) (*BlockFixture, error) {
	blockType, err := BlockTypeFromEra(era)
	if err != nil {
		return nil, err
	}

	cbor, err := blockCborFromEra(era)
	if err != nil {
		return nil, err
	}

	block, err := ledger.NewBlockFromCbor(blockType, cbor)
	if err != nil {
		return nil, fmt.Errorf("decode %s block: %w", era, err)
	}

	return &BlockFixture{
		Name:      name,
		Era:       era,
		BlockType: blockType,
		Cbor:      cbor,
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

// TxFixture contains a pre-loaded transaction for benchmarking.
type TxFixture struct {
	Name string
	Era  string
	Cbor []byte
	Tx   common.Transaction
}

// LoadTxFixture loads a test transaction for the given era.
// Currently, transactions are extracted from block fixtures.
// The name parameter is reserved for future fixture variants (e.g., "simple",
// "multisig", "script").
func LoadTxFixture(era, name string) (*TxFixture, error) {
	blockFixture, err := LoadBlockFixture(era, "default")
	if err != nil {
		return nil, err
	}

	txs := blockFixture.Block.Transactions()
	if len(txs) == 0 {
		return nil, fmt.Errorf("no transactions in %s block fixture", era)
	}

	// Use first transaction from block
	tx := txs[0]
	return &TxFixture{
		Name: name,
		Era:  era,
		Cbor: tx.Cbor(),
		Tx:   tx,
	}, nil
}

// MustLoadTxFixture loads a test transaction and panics on error.
func MustLoadTxFixture(era, name string) *TxFixture {
	fixture, err := LoadTxFixture(era, name)
	if err != nil {
		panic(fmt.Sprintf("failed to load %s tx fixture: %v", era, err))
	}
	return fixture
}

// BlockTypeFromEra returns the ledger block type constant for the given era
// name.
func BlockTypeFromEra(era string) (uint, error) {
	switch strings.ToLower(era) {
	case "byron":
		return ledger.BlockTypeByronMain, nil
	case "shelley":
		return ledger.BlockTypeShelley, nil
	case "allegra":
		return ledger.BlockTypeAllegra, nil
	case "mary":
		return ledger.BlockTypeMary, nil
	case "alonzo":
		return ledger.BlockTypeAlonzo, nil
	case "babbage":
		return ledger.BlockTypeBabbage, nil
	case "conway":
		return ledger.BlockTypeConway, nil
	default:
		return 0, fmt.Errorf("unknown era: %s", era)
	}
}

// TxTypeFromEra returns the ledger transaction type constant for the given era
// name.
func TxTypeFromEra(era string) (uint, error) {
	switch strings.ToLower(era) {
	case "byron":
		return ledger.TxTypeByron, nil
	case "shelley":
		return ledger.TxTypeShelley, nil
	case "allegra":
		return ledger.TxTypeAllegra, nil
	case "mary":
		return ledger.TxTypeMary, nil
	case "alonzo":
		return ledger.TxTypeAlonzo, nil
	case "babbage":
		return ledger.TxTypeBabbage, nil
	case "conway":
		return ledger.TxTypeConway, nil
	default:
		return 0, fmt.Errorf("unknown era: %s", era)
	}
}

// EraNames returns the list of supported era names for benchmarking.
func EraNames() []string {
	return []string{
		"byron",
		"shelley",
		"allegra",
		"mary",
		"alonzo",
		"babbage",
		"conway",
	}
}

// PostByronEraNames returns era names excluding Byron (for Praos-era
// benchmarks).
func PostByronEraNames() []string {
	return []string{"shelley", "allegra", "mary", "alonzo", "babbage", "conway"}
}

// blockCborFromEra returns the raw CBOR bytes for the given era's test block.
func blockCborFromEra(era string) ([]byte, error) {
	switch strings.ToLower(era) {
	case "byron":
		return testdata.MustDecodeHex(testdata.ByronBlockHex), nil
	case "shelley":
		return testdata.MustDecodeHex(testdata.ShelleyBlockHex), nil
	case "allegra":
		return testdata.MustDecodeHex(testdata.AllegraBlockHex), nil
	case "mary":
		return testdata.MustDecodeHex(testdata.MaryBlockHex), nil
	case "alonzo":
		return testdata.MustDecodeHex(testdata.AlonzoBlockHex), nil
	case "babbage":
		return testdata.MustDecodeHex(testdata.BabbageBlockHex), nil
	case "conway":
		return testdata.MustDecodeHex(testdata.ConwayBlockHex), nil
	default:
		return nil, fmt.Errorf("unknown era: %s", era)
	}
}

// GetTestBlocks returns all available test blocks from internal/testdata.
// This is a convenience wrapper around testdata.GetTestBlocks().
func GetTestBlocks() []testdata.TestBlock {
	return testdata.GetTestBlocks()
}
