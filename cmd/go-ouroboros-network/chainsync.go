package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	ouroboros "github.com/cloudstruct/go-ouroboros-network"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/protocol/blockfetch"
	"github.com/cloudstruct/go-ouroboros-network/protocol/chainsync"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"os"
)

type chainSyncState struct {
	oConn                 *ouroboros.Ouroboros
	nodeToNode            bool
	readyForNextBlockChan chan bool
	byronEpochBaseSlot    uint64
	byronEpochSlot        uint64
}

var syncState chainSyncState

type chainSyncFlags struct {
	flagset  *flag.FlagSet
	startEra string
}

func newChainSyncFlags() *chainSyncFlags {
	f := &chainSyncFlags{
		flagset: flag.NewFlagSet("chain-sync", flag.ExitOnError),
	}
	f.flagset.StringVar(&f.startEra, "start-era", "byron", "era which to start chain-sync at")
	return f
}

// Intersect points (last block of previous era) for each era on testnet/mainnet
var eraIntersect = map[int]map[string][]interface{}{
	TESTNET_MAGIC: map[string][]interface{}{
		"byron": []interface{}{},
		// Last block of epoch 73 (Byron era)
		"shelley": []interface{}{1598399, "7e16781b40ebf8b6da18f7b5e8ade855d6738095ef2f1c58c77e88b6e45997a4"},
		// Last block of epoch 101 (Shelley era)
		"allegra": []interface{}{13694363, "b596f9739b647ab5af901c8fc6f75791e262b0aeba81994a1d622543459734f2"},
		// Last block of epoch 111 (Allegra era)
		"mary": []interface{}{18014387, "9914c8da22a833a777d8fc1f735d2dbba70b99f15d765b6c6ee45fe322d92d93"},
		// Last block of epoch 153 (Mary era)
		"alonzo": []interface{}{36158304, "2b95ce628d36c3f8f37a32c2942b48e4f9295ccfe8190bcbc1f012e1e97c79eb"},
	},
	MAINNET_MAGIC: map[string][]interface{}{
		"byron": []interface{}{},
		// Last block of epoch 207 (Byron era)
		"shelley": []interface{}{4492799, "f8084c61b6a238acec985b59310b6ecec49c0ab8352249afd7268da5cff2a457"},
		// Last block of epoch 235 (Shelley era)
		"allegra": []interface{}{16588737, "4e9bbbb67e3ae262133d94c3da5bffce7b1127fc436e7433b87668dba34c354a"},
		// Last block of epoch 250 (Allegra era)
		"mary": []interface{}{23068793, "69c44ac1dda2ec74646e4223bc804d9126f719b1c245dadc2ad65e8de1b276d7"},
		// Last block of epoch 289 (Mary era)
		"alonzo": []interface{}{39916796, "e72579ff89dc9ed325b723a33624b596c08141c7bd573ecfff56a1f7229e4d09"},
	},
}

func buildChainSyncCallbackConfig() *chainsync.ChainSyncCallbackConfig {
	return &chainsync.ChainSyncCallbackConfig{
		AwaitReplyFunc:        chainSyncAwaitReplyHandler,
		RollBackwardFunc:      chainSyncRollBackwardHandler,
		RollForwardFunc:       chainSyncRollForwardHandler,
		IntersectFoundFunc:    chainSyncIntersectFoundHandler,
		IntersectNotFoundFunc: chainSyncIntersectNotFoundHandler,
		DoneFunc:              chainSyncDoneHandler,
	}
}

func buildBlockFetchCallbackConfig() *blockfetch.BlockFetchCallbackConfig {
	return &blockfetch.BlockFetchCallbackConfig{
		StartBatchFunc: blockFetchStartBatchHandler,
		NoBlocksFunc:   blockFetchNoBlocksHandler,
		BlockFunc:      blockFetchBlockHandler,
		BatchDoneFunc:  blockFetchBatchDoneHandler,
	}
}

func testChainSync(f *globalFlags) {
	chainSyncFlags := newChainSyncFlags()
	err := chainSyncFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}

	if _, ok := eraIntersect[f.networkMagic][chainSyncFlags.startEra]; !ok {
		fmt.Printf("ERROR: unknown era '%s' specified as chain-sync start point\n", chainSyncFlags.startEra)
		os.Exit(1)
	}

	conn := createClientConnection(f)
	errorChan := make(chan error)
	oOpts := &ouroboros.OuroborosOptions{
		Conn:                     conn,
		NetworkMagic:             uint32(f.networkMagic),
		ErrorChan:                errorChan,
		UseNodeToNodeProtocol:    f.ntnProto,
		SendKeepAlives:           true,
		ChainSyncCallbackConfig:  buildChainSyncCallbackConfig(),
		BlockFetchCallbackConfig: buildBlockFetchCallbackConfig(),
	}
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
	}()
	o, err := ouroboros.New(oOpts)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	syncState.oConn = o
	syncState.readyForNextBlockChan = make(chan bool)
	syncState.nodeToNode = f.ntnProto
	intersect := []interface{}{}
	if len(eraIntersect[f.networkMagic][chainSyncFlags.startEra]) > 0 {
		// Slot
		intersect = append(intersect, eraIntersect[f.networkMagic][chainSyncFlags.startEra][0])
		// Block hash
		hash, _ := hex.DecodeString(eraIntersect[f.networkMagic][chainSyncFlags.startEra][1].(string))
		intersect = append(intersect, hash)
	}
	if err := o.ChainSync.FindIntersect([]interface{}{intersect}); err != nil {
		fmt.Printf("ERROR: FindIntersect: %s\n", err)
		os.Exit(1)
	}
	// Wait until ready for next block
	<-syncState.readyForNextBlockChan
	for {
		err := o.ChainSync.RequestNext()
		if err != nil {
			fmt.Printf("ERROR: RequestNext: %s\n", err)
			os.Exit(1)
		}
		// Wait until ready for next block
		<-syncState.readyForNextBlockChan
	}
}

func chainSyncAwaitReplyHandler() error {
	return nil
}

func chainSyncRollBackwardHandler(point interface{}, tip interface{}) error {
	fmt.Printf("roll backward: point = %#v, tip = %#v\n", point, tip)
	syncState.readyForNextBlockChan <- true
	return nil
}

func chainSyncRollForwardHandler(blockType uint, blockData interface{}) error {
	if syncState.nodeToNode {
		var blockSlot uint64
		var blockHash []byte
		switch blockType {
		case block.BLOCK_TYPE_BYRON_EBB:
			h := blockData.(*block.ByronEpochBoundaryBlockHeader)
			//fmt.Printf("era = Byron (EBB), epoch = %d, id = %s\n", h.ConsensusData.Epoch, h.Id())
			if syncState.byronEpochSlot > 0 {
				syncState.byronEpochBaseSlot += syncState.byronEpochSlot + 1
			}
			blockSlot = syncState.byronEpochBaseSlot
			blockHash, _ = hex.DecodeString(h.Id())
		case block.BLOCK_TYPE_BYRON_MAIN:
			h := blockData.(*block.ByronMainBlockHeader)
			//fmt.Printf("era = Byron, epoch = %d, slot = %d, id = %s\n", h.ConsensusData.SlotId.Epoch, h.ConsensusData.SlotId.Slot, h.Id())
			syncState.byronEpochSlot = uint64(h.ConsensusData.SlotId.Slot)
			blockSlot = syncState.byronEpochBaseSlot + syncState.byronEpochSlot
			blockHash, _ = hex.DecodeString(h.Id())
		default:
			h := blockData.(*block.ShelleyBlockHeader)
			blockSlot = h.Body.Slot
			blockHash, _ = hex.DecodeString(h.Id())
		}
		if err := syncState.oConn.BlockFetch.RequestRange([]interface{}{blockSlot, blockHash}, []interface{}{blockSlot, blockHash}); err != nil {
			fmt.Printf("error calling RequestRange: %s\n", err)
			return err
		}
	} else {
		switch blockType {
		case block.BLOCK_TYPE_BYRON_EBB:
			b := blockData.(*block.ByronEpochBoundaryBlock)
			fmt.Printf("era = Byron (EBB), epoch = %d, id = %s\n", b.Header.ConsensusData.Epoch, b.Id())
		case block.BLOCK_TYPE_BYRON_MAIN:
			b := blockData.(*block.ByronMainBlock)
			fmt.Printf("era = Byron, epoch = %d, slot = %d, id = %s\n", b.Header.ConsensusData.SlotId.Epoch, b.Header.ConsensusData.SlotId.Slot, b.Id())
		case block.BLOCK_TYPE_SHELLEY:
			b := blockData.(*block.ShelleyBlock)
			fmt.Printf("era = Shelley, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
		case block.BLOCK_TYPE_ALLEGRA:
			b := blockData.(*block.AllegraBlock)
			fmt.Printf("era = Allegra, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
		case block.BLOCK_TYPE_MARY:
			b := blockData.(*block.MaryBlock)
			fmt.Printf("era = Mary, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
		case block.BLOCK_TYPE_ALONZO:
			b := blockData.(*block.AlonzoBlock)
			fmt.Printf("era = Alonzo, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
		default:
			fmt.Printf("unsupported (so far) block type %d\n", blockType)
			fmt.Printf("%s\n", utils.DumpCborStructure(blockData, ""))
		}
		syncState.readyForNextBlockChan <- true
	}
	return nil
}

func chainSyncIntersectFoundHandler(point interface{}, tip interface{}) error {
	fmt.Printf("found intersect: point = %#v, tip = %#v\n", point, tip)
	syncState.readyForNextBlockChan <- true
	return nil
}

func chainSyncIntersectNotFoundHandler(tip interface{}) error {
	fmt.Printf("ERROR: failed to find intersection\n")
	os.Exit(1)
	return nil
}

func chainSyncDoneHandler() error {
	return nil
}

func blockFetchStartBatchHandler() error {
	return nil
}

func blockFetchNoBlocksHandler() error {
	fmt.Printf("blockFetchNoBlocksHandler()\n")
	return nil
}

func blockFetchBlockHandler(blockType uint, blockData interface{}) error {
	switch blockType {
	case block.BLOCK_TYPE_BYRON_EBB:
		b := blockData.(*block.ByronEpochBoundaryBlock)
		fmt.Printf("era = Byron (EBB), id = %s\n", b.Id())
	case block.BLOCK_TYPE_BYRON_MAIN:
		b := blockData.(*block.ByronMainBlock)
		fmt.Printf("era = Byron, epoch = %d, slot = %d, id = %s\n", b.Header.ConsensusData.SlotId.Epoch, b.Header.ConsensusData.SlotId.Slot, b.Id())
	case block.BLOCK_TYPE_SHELLEY:
		b := blockData.(*block.ShelleyBlock)
		fmt.Printf("era = Shelley, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
	case block.BLOCK_TYPE_ALLEGRA:
		b := blockData.(*block.AllegraBlock)
		fmt.Printf("era = Allegra, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
	case block.BLOCK_TYPE_MARY:
		b := blockData.(*block.MaryBlock)
		fmt.Printf("era = Mary, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
	case block.BLOCK_TYPE_ALONZO:
		b := blockData.(*block.AlonzoBlock)
		fmt.Printf("era = Alonzo, slot = %d, block_no = %d, id = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Id())
	default:
		fmt.Printf("unsupported (so far) block type %d\n", blockType)
		fmt.Printf("%s\n", utils.DumpCborStructure(blockData, ""))
	}
	return nil
}

func blockFetchBatchDoneHandler() error {
	syncState.readyForNextBlockChan <- true
	return nil
}
