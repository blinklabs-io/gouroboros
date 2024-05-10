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
	"encoding/hex"
	"flag"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/common"
)

var oConn *ouroboros.Connection

type chainSyncFlags struct {
	flagset    *flag.FlagSet
	startEra   string
	tip        bool
	bulk       bool
	blockRange bool
}

func newChainSyncFlags() *chainSyncFlags {
	f := &chainSyncFlags{
		flagset: flag.NewFlagSet("chain-sync", flag.ExitOnError),
	}
	f.flagset.StringVar(
		&f.startEra,
		"start-era",
		"genesis",
		"era which to start chain-sync at",
	)
	f.flagset.BoolVar(
		&f.tip,
		"tip",
		false,
		"start chain-sync at current chain tip",
	)
	f.flagset.BoolVar(
		&f.bulk,
		"bulk",
		false,
		"use bulk chain-sync mode with NtN",
	)
	f.flagset.BoolVar(
		&f.blockRange,
		"range",
		false,
		"show start/end block of range",
	)
	return f
}

// Intersect points (last block of previous era) for each era on testnet/mainnet
var eraIntersect = map[string]map[string][]interface{}{
	"unknown": map[string][]interface{}{
		"genesis": []interface{}{},
	},
	"testnet": map[string][]interface{}{
		"genesis": []interface{}{},
		"byron":   []interface{}{},
		// Last block of epoch 73 (Byron era)
		"shelley": []interface{}{
			1598399,
			"7e16781b40ebf8b6da18f7b5e8ade855d6738095ef2f1c58c77e88b6e45997a4",
		},
		// Last block of epoch 101 (Shelley era)
		"allegra": []interface{}{
			13694363,
			"b596f9739b647ab5af901c8fc6f75791e262b0aeba81994a1d622543459734f2",
		},
		// Last block of epoch 111 (Allegra era)
		"mary": []interface{}{
			18014387,
			"9914c8da22a833a777d8fc1f735d2dbba70b99f15d765b6c6ee45fe322d92d93",
		},
		// Last block of epoch 153 (Mary era)
		"alonzo": []interface{}{
			36158304,
			"2b95ce628d36c3f8f37a32c2942b48e4f9295ccfe8190bcbc1f012e1e97c79eb",
		},
		// Last block of epoch 214 (Alonzo era)
		"babbage": []interface{}{
			62510369,
			"d931221f9bc4cae34de422d9f4281a2b0344e86aac6b31eb54e2ee90f44a09b9",
		},
	},
	"mainnet": map[string][]interface{}{
		"genesis": []interface{}{},
		// Chain genesis, but explicit
		"byron": []interface{}{},
		// Last block of epoch 207 (Byron era)
		"shelley": []interface{}{
			4492799,
			"f8084c61b6a238acec985b59310b6ecec49c0ab8352249afd7268da5cff2a457",
		},
		// Last block of epoch 235 (Shelley era)
		"allegra": []interface{}{
			16588737,
			"4e9bbbb67e3ae262133d94c3da5bffce7b1127fc436e7433b87668dba34c354a",
		},
		// Last block of epoch 250 (Allegra era)
		"mary": []interface{}{
			23068793,
			"69c44ac1dda2ec74646e4223bc804d9126f719b1c245dadc2ad65e8de1b276d7",
		},
		// Last block of epoch 289 (Mary era)
		"alonzo": []interface{}{
			39916796,
			"e72579ff89dc9ed325b723a33624b596c08141c7bd573ecfff56a1f7229e4d09",
		},
		// Last block of epoch 364 (Alonzo era)
		"babbage": []interface{}{
			72316796,
			"c58a24ba8203e7629422a24d9dc68ce2ed495420bf40d9dab124373655161a20",
		},
	},
	"preprod": map[string][]interface{}{
		"genesis": []interface{}{},
		"alonzo":  []interface{}{},
	},
	"preview": map[string][]interface{}{
		"genesis": []interface{}{},
		"alonzo":  []interface{}{},
		// Last block of epoch 3 (Alonzo era)
		"babbage": []interface{}{
			345594,
			"e47ac07272e95d6c3dc8279def7b88ded00e310f99ac3dfbae48ed9ff55e6001",
		},
	},
}

func buildChainSyncConfig() chainsync.Config {
	return chainsync.NewConfig(
		chainsync.WithRollBackwardFunc(chainSyncRollBackwardHandler),
		chainsync.WithRollForwardFunc(chainSyncRollForwardHandler),
	)
}

func buildBlockFetchConfig() blockfetch.Config {
	return blockfetch.NewConfig(
		blockfetch.WithBlockFunc(blockFetchBlockHandler),
	)
}

func testChainSync(f *globalFlags) {
	chainSyncFlags := newChainSyncFlags()
	err := chainSyncFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}

	var intersectPoint []interface{}
	if _, ok := eraIntersect[f.network]; !ok {
		if chainSyncFlags.startEra != "genesis" {
			fmt.Printf(
				"ERROR: only 'genesis' is supported for -start-era for unknown networks\n",
			)
			os.Exit(1)
		}
		intersectPoint = eraIntersect["unknown"]["genesis"]
	} else {
		if _, ok := eraIntersect[f.network][chainSyncFlags.startEra]; !ok {
			fmt.Printf("ERROR: unknown era '%s' specified as chain-sync start point\n", chainSyncFlags.startEra)
			os.Exit(1)
		}
		intersectPoint = eraIntersect[f.network][chainSyncFlags.startEra]
	}

	conn := createClientConnection(f)
	errorChan := make(chan error)
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
	}()
	oConn, err = ouroboros.New(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetworkMagic(uint32(f.networkMagic)),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(f.ntnProto),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithChainSyncConfig(buildChainSyncConfig()),
		ouroboros.WithBlockFetchConfig(buildBlockFetchConfig()),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	// REMOVE: Test GetCurrentTip during chain sync.
	// for i := 0; i < 10; i++ {
	// 	go func() {
	// 		last := uint64(0)
	// 		count := 0

	// 		// REMOVE.
	// 		time.Sleep(3 * time.Second)

	// 		fmt.Printf("Starting GetCurrentTip test\n")

	// 		for {
	// 			count++

	// 			tip, err := oConn.ChainSync().Client.GetCurrentTip()
	// 			if err != nil {
	// 				fmt.Printf("ERROR: GetCurrentTip: %v\n", err)
	// 				return
	// 			}

	// 			if tip.BlockNumber != last {
	// 				fmt.Printf("tip: block:%d count:%d\n", tip.BlockNumber, count)
	// 				last = tip.BlockNumber
	// 				count = 0
	// 			}

	// 			time.Sleep(10 * time.Millisecond)
	// 		}
	// 	}()
	// }

	var point common.Point
	if chainSyncFlags.tip {
		tip, err := oConn.ChainSync().Client.GetCurrentTip()
		if err != nil {
			fmt.Printf("ERROR: failed to get current tip: %s\n", err)
			os.Exit(1)
		}
		point = tip.Point
	} else if len(intersectPoint) > 0 {
		// Slot
		slot := uint64(intersectPoint[0].(int))
		// Block hash
		hash, _ := hex.DecodeString(intersectPoint[1].(string))
		point = common.NewPoint(slot, hash)
	} else {
		point = common.NewPointOrigin()
	}
	if chainSyncFlags.blockRange {
		fmt.Printf("client: requesting block range\n")

		start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange(
			[]common.Point{point},
		)
		fmt.Printf("client: block range: %d -> %d\n", start, end)
		if err != nil {
			fmt.Printf("ERROR: failed to get available block range: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("Start:     slot %d, hash %x\n", start.Slot, start.Hash)
		fmt.Printf("End (tip): slot %d, hash %x\n", end.Slot, end.Hash)
		return
	} else if !f.ntnProto || !chainSyncFlags.bulk {
		if err := oConn.ChainSync().Client.Sync([]common.Point{point}); err != nil {
			fmt.Printf("ERROR: failed to start chain-sync: %s\n", err)
			os.Exit(1)
		}
	} else {
		start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange([]common.Point{point})
		if err != nil {
			fmt.Printf("ERROR: failed to get available block range: %s\n", err)
			os.Exit(1)
		}
		// Stop the chain-sync client to prevent the connection getting closed due to chain-sync idle timeout
		if err := oConn.ChainSync().Client.Stop(); err != nil {
			fmt.Printf("ERROR: failed to shutdown chain-sync: %s\n", err)
			os.Exit(1)
		}
		if err := oConn.BlockFetch().Client.GetBlockRange(start, end); err != nil {
			fmt.Printf("ERROR: failed to request block range: %s\n", err)
			os.Exit(1)
		}
	}
	// Wait forever...the rest of the sync operations are async
	select {}
}

func chainSyncRollBackwardHandler(ctx chainsync.CallbackContext, point common.Point, tip chainsync.Tip) error {
	fmt.Printf("roll backward: point = %#v, tip = %#v\n", point, tip)
	return nil
}

func chainSyncRollForwardHandler(
	ctx chainsync.CallbackContext,
	blockType uint,
	blockData interface{},
	tip chainsync.Tip,
) error {
	var block ledger.Block
	switch v := blockData.(type) {
	case ledger.Block:
		block = v
	case ledger.BlockHeader:
		blockSlot := v.SlotNumber()
		blockHash, _ := hex.DecodeString(v.Hash())
		var err error
		block, err = oConn.BlockFetch().Client.GetBlock(common.NewPoint(blockSlot, blockHash))
		if err != nil {
			return err
		}
	}
	// Display block info
	switch blockType {
	case ledger.BlockTypeByronEbb:
		byronEbbBlock := block.(*ledger.ByronEpochBoundaryBlock)
		fmt.Printf(
			"era = Byron (EBB), epoch = %d, slot = %d, id = %s\n",
			byronEbbBlock.Header.ConsensusData.Epoch,
			byronEbbBlock.SlotNumber(),
			byronEbbBlock.Hash(),
		)
	case ledger.BlockTypeByronMain:
		byronBlock := block.(*ledger.ByronMainBlock)
		fmt.Printf(
			"era = Byron, epoch = %d, slot = %d, id = %s\n",
			byronBlock.Header.ConsensusData.SlotId.Epoch,
			byronBlock.SlotNumber(),
			byronBlock.Hash(),
		)
	default:
		if block == nil {
			return fmt.Errorf("block is nil")
		}
		fmt.Printf(
			"era = %s, slot = %d, block_no = %d, id = %s\n",
			block.Era().Name,
			block.SlotNumber(),
			block.BlockNumber(),
			block.Hash(),
		)
	}
	return nil
}

func blockFetchBlockHandler(ctx blockfetch.CallbackContext, blockData ledger.Block) error {
	switch block := blockData.(type) {
	case *ledger.ByronEpochBoundaryBlock:
		fmt.Printf("era = Byron (EBB), epoch = %d, slot = %d, id = %s\n", block.Header.ConsensusData.Epoch, block.SlotNumber(), block.Hash())
	case *ledger.ByronMainBlock:
		fmt.Printf("era = Byron, epoch = %d, slot = %d, id = %s\n", block.Header.ConsensusData.SlotId.Epoch, block.SlotNumber(), block.Hash())
	case ledger.Block:
		fmt.Printf("era = %s, slot = %d, block_no = %d, id = %s\n", block.Era().Name, block.SlotNumber(), block.BlockNumber(), block.Hash())
	}
	return nil
}
