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

package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/kelseyhightower/envconfig"
)

// We parse environment variables using envconfig into this struct
type Config struct {
	Address      string
	Hash         string
	Network      string
	NetworkMagic uint32 `split_words:"true"`
	ReturnCbor   bool   `split_words:"true"`
	Slot         uint64
}

// This code will be executed when run
func main() {
	// Set config defaults (first mainnet Babbage block)
	cfg := Config{
		Address:      "backbone.cardano.iog.io:3001",
		Network:      "mainnet",
		NetworkMagic: 0,
		ReturnCbor:   false,
		// Byron
		// Hash: "37e8df8fe301e8187b1dbfc50713ab0b7ab74c441cef1461bba710e5f05a1959",
		// Slot: 2656803,
		// Shelley
		// Hash: "89b237ee673ef79f065d699aa539bba9644d34b9aa8a49510ab14ad8735c293e",
		// Slot: 4506600,
		// Allegra
		// Hash: "078d102d0247463f91eef69fc77f3fbbf120f3118e68cd5e6a493c15446dbf8c",
		// Slot: 16588800,
		// Alonzo
		// Hash: "8959c0323b94cc670afe44222ab8b4e72cfcad3b5ab665f334bbe642dc6e9ef4",
		// Slot: 39916975,
		// Babbage
		Hash: "eea1247726ababb0b15ef7068b6917ceb6ebe3021c40fe44608585bba44e24b6",
		Slot: 72316896,
	}
	// Parse environment variables
	if err := envconfig.Process("block_fetch", &cfg); err != nil {
		panic(err)
	}
	// Configure NetworkMagic
	if cfg.NetworkMagic == 0 {
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if !ok {
			fmt.Printf("invalid network specified: %v\n", cfg.Network)
			os.Exit(1)
		}
		cfg.NetworkMagic = network.NetworkMagic
	}
	// Create error channel
	errorChan := make(chan error)
	// start error handler
	go func() {
		for {
			err := <-errorChan
			panic(err)
		}
	}()
	// Configure Ouroboros
	o, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(cfg.NetworkMagic),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
	)
	if err != nil {
		panic(err)
	}
	// Connect to Node address
	if err = o.Dial("tcp", cfg.Address); err != nil {
		panic(err)
	}
	// Decode hash string into bytes
	blockHash, err := hex.DecodeString(cfg.Hash)
	if err != nil {
		panic(err)
	}
	// Get requested block from Node via NtN BlockFetch
	block, err := o.BlockFetch().Client.GetBlock(
		ocommon.NewPoint(cfg.Slot, blockHash),
	)
	if block == nil {
		panic("empty block! this shouldn't happen")
	}
	if err != nil {
		panic(err)
	}
	// Check if we want CBOR or text output
	if cfg.ReturnCbor {
		// Write our binary CBOR block to stdout
		_ = binary.Write(
			os.Stdout,
			binary.LittleEndian,
			block.Cbor(),
		)
		os.Exit(0)
	}

	// Display simple block info

	switch v := block.(type) {
	case *ledger.ByronEpochBoundaryBlock:
		fmt.Printf(
			"Block: era = Byron (EBB), epoch = %d, id = %s\n",
			v.BlockHeader.ConsensusData.Epoch,
			v.Hash(),
		)
	case *ledger.ByronMainBlock:
		fmt.Printf(
			"Block: era = Byron, epoch = %d, slot = %d, id = %s\n",
			v.BlockHeader.ConsensusData.SlotId.Epoch,
			v.SlotNumber(),
			v.Hash(),
		)
	case ledger.Block:
		fmt.Printf(
			"Block: era = %s, slot = %d, block_no = %d, id = %s\n",
			v.Era().Name,
			v.SlotNumber(),
			v.BlockNumber(),
			v.Hash(),
		)
	}

	fmt.Printf(
		"Block CBOR: %x\n",
		block.Cbor(),
	)

	// Extended block info

	// Issuer
	fmt.Printf(
		"Minted by: %s (%s)\n",
		block.IssuerVkey().PoolId(),
		block.IssuerVkey().Hash(),
	)
	// Transactions
	fmt.Println("Transactions:")
	for _, tx := range block.Transactions() {
		fmt.Printf("- Hash: %s\n", tx.Hash())
		fmt.Printf("- CBOR: %x\n", tx.Cbor())
		// Show metadata, if present
		if tx.Metadata() != nil {
			fmt.Printf(
				"  Metadata: %#v (%x)\n",
				tx.Metadata().TypeName(),
				tx.Metadata().Cbor(),
			)
		}
		// Inputs
		if len(tx.Inputs()) > 0 {
			fmt.Println("  Inputs:")
			for _, input := range tx.Inputs() {
				fmt.Printf(
					"  - index = %d, id = %s\n",
					input.Index(),
					input.Id(),
				)
			}
		}
		// Outputs
		if len(tx.Outputs()) > 0 {
			fmt.Println("  Outputs:")
			for _, output := range tx.Outputs() {
				// Output our normal address and amount, in all transactions
				fmt.Printf(
					"  - address = %s, amount = %d, cbor (hex) = %x\n",
					output.Address(),
					output.Amount(),
					output.Cbor(),
				)
				// Check for optional assets
				assets := output.Assets()
				if assets != nil {
					fmt.Println("  - Assets:")
					for _, policyId := range assets.Policies() {
						for _, assetName := range assets.Assets(policyId) {
							fmt.Printf(
								"    - Asset: name = %s, amount = %d, policy = %s\n",
								assetName,
								assets.Asset(policyId, assetName),
								policyId,
							)
						}
					}
				}
				// Check for optional datum
				datum := output.Datum()
				if datum != nil {
					jsonData, err := json.Marshal(datum)
					if err != nil {
						fmt.Printf(
							"  - Datum: (hex) %x\n",
							datum.Cbor(),
						)
					} else {
						fmt.Printf(
							"  - Datum: %s\n",
							jsonData,
						)
					}
				}
			}
		}
		// Collateral
		if len(tx.Collateral()) > 0 {
			fmt.Println("  Collateral inputs:")
			for _, input := range tx.Collateral() {
				fmt.Printf(
					"  - index = %d, id = %s\n",
					input.Index(),
					input.Id(),
				)
			}
		}
		// Certificates
		if len(tx.Certificates()) > 0 {
			fmt.Println("  Certificates:")
			for _, cert := range tx.Certificates() {
				fmt.Printf("  - %T\n", cert)
			}
		}
		// Asset mints
		if tx.AssetMint() != nil {
			fmt.Println("  Asset mints:")
			assets := tx.AssetMint()
			for _, policyId := range assets.Policies() {
				for _, assetName := range assets.Assets(policyId) {
					fmt.Printf(
						"    - Asset: name = %s, amount = %d, policy = %s\n",
						assetName,
						assets.Asset(policyId, assetName),
						policyId,
					)
				}
			}
		}
	}
	fmt.Println()
}
