// Copyright 2023 Blink Labs, LLC.
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
	"encoding/hex"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cmd/common"
	"github.com/blinklabs-io/gouroboros/ledger"
	ocommon "github.com/blinklabs-io/gouroboros/protocol/common"
)

type blockFetchFlags struct {
	*common.GlobalFlags
	slot uint64
	hash string
	all  bool
}

func main() {
	// Parse commandline
	f := blockFetchFlags{
		GlobalFlags: common.NewGlobalFlags(),
	}
	f.Flagset.Uint64Var(&f.slot, "slot", 0, "slot for single block to fetch")
	f.Flagset.StringVar(&f.hash, "hash", "", "hash for single block to fetch")
	f.Flagset.BoolVar(&f.all, "all", false, "show all available detail for block")
	f.Parse()
	// Create connection
	conn := common.CreateClientConnection(f.GlobalFlags)
	errorChan := make(chan error)
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR(async): %s\n", err)
			os.Exit(1)
		}
	}()
	o, err := ouroboros.New(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetworkMagic(uint32(f.NetworkMagic)),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(f.NtnProto),
		ouroboros.WithKeepAlive(true),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	blockHash, err := hex.DecodeString(f.hash)
	if err != nil {
		fmt.Printf("ERROR: failed to decode block hash: %s\n", err)
		os.Exit(1)
	}
	block, err := o.BlockFetch().Client.GetBlock(ocommon.NewPoint(f.slot, blockHash))
	if err != nil {
		fmt.Printf("ERROR: failed to fetch block: %s\n", err)
		os.Exit(1)
	}

	// Display block info
	switch v := block.(type) {
	case *ledger.ByronEpochBoundaryBlock:
		fmt.Printf("era = Byron (EBB), epoch = %d, id = %s\n", v.Header.ConsensusData.Epoch, v.Hash())
	case *ledger.ByronMainBlock:
		fmt.Printf("era = Byron, epoch = %d, slot = %d, id = %s\n", v.Header.ConsensusData.SlotId.Epoch, v.SlotNumber(), v.Hash())
	case ledger.Block:
		fmt.Printf("era = %s, slot = %d, block_no = %d, id = %s\n", v.Era().Name, v.SlotNumber(), v.BlockNumber(), v.Hash())
	}
	if f.all {
		// Display transaction info
		fmt.Printf("\nTransactions:\n")
		for _, tx := range block.Transactions() {
			fmt.Printf("  Hash: %s\n", tx.Hash())
			fmt.Printf("  Inputs:\n")
			for _, input := range tx.Inputs() {
				fmt.Printf("    Id: %s\n", input.Id())
				fmt.Printf("    Index: %d\n", input.Index())
				fmt.Println("")
			}
			fmt.Printf("  Outputs:\n")
			for _, output := range tx.Outputs() {
				fmt.Printf("    Address: %x\n", output.Address())
				fmt.Printf("    Amount: %d\n", output.Amount())
				assets := output.Assets()
				if assets != nil {
					fmt.Printf("    Assets:\n")
					for _, policyId := range assets.Policies() {
						fmt.Printf("      Policy Id: %s\n", policyId)
						fmt.Printf("      Policy assets:\n")
						for _, assetName := range assets.Assets(policyId) {
							fmt.Printf("        Asset name: %s\n", assetName)
							fmt.Printf("        Amount: %d\n", assets.Asset(policyId, assetName))
							fmt.Println("")
						}
					}
				}
				fmt.Println("")
			}
		}
	}
}
