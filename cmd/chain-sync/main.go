// Copyright 2025 Blink Labs Software
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
	"errors"
	"flag"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/kelseyhightower/envconfig"
)

// We parse environment variables using envconfig into this struct
type Config struct {
	SocketPath string `split_words:"true"`
	Address    string
	Network    string
	Magic      uint32
	StartEra   string `split_words:"true"`
	Tip        bool
	Bulk       bool
	BlockRange bool `split_words:"true"`
}

// Intersect points (last block of previous era) for each era on testnet/mainnet
var eraIntersect = map[string]map[string][]any{
	"unknown": {
		"genesis": {},
	},
	"mainnet": {
		"genesis": {},
		// Chain genesis, but explicit
		"byron": {},
		// Last block of epoch 207 (Byron era)
		"shelley": {
			4492799,
			"f8084c61b6a238acec985b59310b6ecec49c0ab8352249afd7268da5cff2a457",
		},
		// Last block of epoch 235 (Shelley era)
		"allegra": {
			16588737,
			"4e9bbbb67e3ae262133d94c3da5bffce7b1127fc436e7433b87668dba34c354a",
		},
		// Last block of epoch 250 (Allegra era)
		"mary": {
			23068793,
			"69c44ac1dda2ec74646e4223bc804d9126f719b1c245dadc2ad65e8de1b276d7",
		},
		// Last block of epoch 289 (Mary era)
		"alonzo": {
			39916796,
			"e72579ff89dc9ed325b723a33624b596c08141c7bd573ecfff56a1f7229e4d09",
		},
		// Last block of epoch 364 (Alonzo era)
		"babbage": {
			72316796,
			"c58a24ba8203e7629422a24d9dc68ce2ed495420bf40d9dab124373655161a20",
		},
		// Last block of epoch 506 (Babbage era)
		"conway": []any{
			133660799,
			"e757d57eb8dc9500a61c60a39fadb63d9be6973ba96ae337fd24453d4d15c343",
		},
	},
	"preprod": {
		"genesis": {},
		"alonzo":  {},
	},
	"preview": {
		"genesis": {},
		"alonzo":  {},
		// Last block of epoch 3 (Alonzo era)
		"babbage": {
			345594,
			"e47ac07272e95d6c3dc8279def7b88ded00e310f99ac3dfbae48ed9ff55e6001",
		},
	},
}

var oConn *ouroboros.Connection

func buildChainSyncConfig() chainsync.Config {
	return chainsync.NewConfig(
		chainsync.WithRollBackwardFunc(chainSyncRollBackwardHandler),
		chainsync.WithRollForwardFunc(chainSyncRollForwardHandler),
		chainsync.WithPipelineLimit(10),
	)
}

func buildBlockFetchConfig() blockfetch.Config {
	return blockfetch.NewConfig(
		blockfetch.WithBlockFunc(blockFetchBlockHandler),
	)
}

func chainSyncRollBackwardHandler(
	ctx chainsync.CallbackContext,
	point pcommon.Point,
	tip chainsync.Tip,
) error {
	fmt.Printf("roll backward: point = %#v, tip = %#v\n", point, tip)
	return nil
}

func chainSyncRollForwardHandler(
	ctx chainsync.CallbackContext,
	blockType uint,
	blockData any,
	tip chainsync.Tip,
) error {
	var block lcommon.Block
	switch v := blockData.(type) {
	case lcommon.Block:
		block = v
	case lcommon.BlockHeader:
		blockSlot := v.SlotNumber()
		blockHash := v.Hash().Bytes()
		var err error
		if oConn == nil {
			return errors.New("empty ouroboros connection, aborting")
		}
		block, err = oConn.BlockFetch().Client.GetBlock(pcommon.NewPoint(blockSlot, blockHash))
		if err != nil {
			return err
		}
	}
	// Display block info
	switch blockType {
	case ledger.BlockTypeByronEbb:
		byronEbbBlock := block.(*ledger.ByronEpochBoundaryBlock)
		fmt.Printf(
			"era = Byron (EBB), epoch = %d, slot = %d, block_no = %d, id = %s\n",
			byronEbbBlock.BlockHeader.ConsensusData.Epoch,
			byronEbbBlock.SlotNumber(),
			byronEbbBlock.BlockNumber(),
			byronEbbBlock.Hash(),
		)
	case ledger.BlockTypeByronMain:
		byronBlock := block.(*ledger.ByronMainBlock)
		fmt.Printf(
			"era = Byron, epoch = %d, slot = %d, block_no = %d, id = %s\n",
			byronBlock.BlockHeader.ConsensusData.SlotId.Epoch,
			byronBlock.SlotNumber(),
			byronBlock.BlockNumber(),
			byronBlock.Hash(),
		)
	default:
		if block == nil {
			return errors.New("block is nil")
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

func blockFetchBlockHandler(
	ctx blockfetch.CallbackContext,
	blockType uint,
	blockData lcommon.Block,
) error {
	switch block := blockData.(type) {
	case *ledger.ByronEpochBoundaryBlock:
		fmt.Printf("era = Byron (EBB), epoch = %d, slot = %d, block_no = %d, id = %s\n", block.BlockHeader.ConsensusData.Epoch, block.SlotNumber(), block.BlockNumber(), block.Hash())
	case *ledger.ByronMainBlock:
		fmt.Printf("era = Byron, epoch = %d, slot = %d, block_no = %d, id = %s\n", block.BlockHeader.ConsensusData.SlotId.Epoch, block.SlotNumber(), block.BlockNumber(), block.Hash())
	case lcommon.Block:
		fmt.Printf("era = %s, slot = %d, block_no = %d, id = %s\n", block.Era().Name, block.SlotNumber(), block.BlockNumber(), block.Hash())
	}
	return nil
}

// This code will be executed when run
func main() {
	// Set config defaults
	cfg := Config{
		SocketPath: "/ipc/node.socket",
		StartEra:   "genesis",
	}
	// Parse environment variables
	if err := envconfig.Process("cardano_node", &cfg); err != nil {
		panic(err)
	}

	// Parse command-line flags
	flag.StringVar(&cfg.StartEra, "start-era", cfg.StartEra, "era which to start chain-sync at")
	flag.BoolVar(&cfg.Tip, "tip", false, "start chain-sync at current chain tip")
	flag.BoolVar(&cfg.Bulk, "bulk", false, "use bulk chain-sync mode with NtN")
	flag.BoolVar(&cfg.BlockRange, "range", false, "show start/end block of range")
	flag.Parse()

	// Determine connection type: if Address is set, use NtN (TCP), otherwise use NtC (UNIX socket)
	isNtN := cfg.Address != ""
	networkType := "unix"
	address := cfg.SocketPath
	if isNtN {
		networkType = "tcp"
		address = cfg.Address
	}

	// Configure NetworkMagic
	if cfg.Magic == 0 {
		if cfg.Network == "" {
			// Default to preview network if not specified
			cfg.Network = "preview"
		}
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if !ok {
			fmt.Printf("ERROR: invalid network specified: %v\n", cfg.Network)
			os.Exit(1)
		}
		cfg.Magic = network.NetworkMagic
	}

	// Determine era intersection point
	var intersectPoint []any
	if _, ok := eraIntersect[cfg.Network]; !ok {
		if cfg.StartEra != "genesis" {
			fmt.Printf(
				"ERROR: only 'genesis' is supported for -start-era for unknown networks\n",
			)
			os.Exit(1)
		}
		intersectPoint = eraIntersect["unknown"]["genesis"]
	} else {
		if _, ok := eraIntersect[cfg.Network][cfg.StartEra]; !ok {
			fmt.Printf("ERROR: unknown era '%s' specified as chain-sync start point\n", cfg.StartEra)
			os.Exit(1)
		}
		intersectPoint = eraIntersect[cfg.Network][cfg.StartEra]
	}

	// Create error channel
	errorChan := make(chan error)
	// Start error handler
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
	}()

	// Configure Ouroboros
	o, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(cfg.Magic),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(isNtN),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithChainSyncConfig(buildChainSyncConfig()),
		ouroboros.WithBlockFetchConfig(buildBlockFetchConfig()),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}
	oConn = o

	// Connect to Node
	if err = o.Dial(networkType, address); err != nil {
		fmt.Printf("ERROR: connection failed: %s\n", err)
		os.Exit(1)
	}

	// Determine starting point
	var point pcommon.Point
	if cfg.Tip {
		tip, err := oConn.ChainSync().Client.GetCurrentTip()
		if err != nil {
			fmt.Printf("ERROR: failed to get current tip: %s\n", err)
			os.Exit(1)
		}
		point = tip.Point
	} else if len(intersectPoint) > 0 {
		// Slot
		slot := uint64(intersectPoint[0].(int)) // #nosec G115
		// Block hash
		hash, _ := hex.DecodeString(intersectPoint[1].(string))
		point = pcommon.NewPoint(slot, hash)
	} else {
		point = pcommon.NewPointOrigin()
	}

	// Handle different modes
	if cfg.BlockRange {
		start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange(
			[]pcommon.Point{point},
		)
		if err != nil {
			fmt.Printf("ERROR: failed to get available block range: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("Start:     slot %d, hash %x\n", start.Slot, start.Hash)
		fmt.Printf("End (tip): slot %d, hash %x\n", end.Slot, end.Hash)
		return
	} else if !isNtN || !cfg.Bulk {
		// Standard chain-sync mode (NtC or NtN non-bulk)
		if err := oConn.ChainSync().Client.Sync([]pcommon.Point{point}); err != nil {
			fmt.Printf("ERROR: failed to start chain-sync: %s\n", err)
			os.Exit(1)
		}
	} else {
		// Bulk mode (NtN only)
		start, end, err := oConn.ChainSync().Client.GetAvailableBlockRange([]pcommon.Point{point})
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
