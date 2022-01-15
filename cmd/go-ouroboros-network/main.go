package main

import (
	"crypto/tls"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/protocol/chainsync"
	"github.com/cloudstruct/go-ouroboros-network/utils"
	"io"
	"net"
	"os"
)

const (
	TESTNET_MAGIC = 1097911063
	MAINNET_MAGIC = 764824073
)

type cmdFlags struct {
	socket       string
	address      string
	useTls       bool
	networkMagic int
	testnet      bool
	mainnet      bool
	syncEra      string
}

type chainSyncState struct {
	readyForNextBlockChan chan bool
}

var syncState chainSyncState

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

func main() {
	f := cmdFlags{}
	flag.StringVar(&f.socket, "socket", "", "UNIX socket path to connect to")
	flag.StringVar(&f.address, "address", "", "TCP address to connect to in address:port format")
	flag.BoolVar(&f.useTls, "tls", false, "enable TLS")
	flag.IntVar(&f.networkMagic, "network-magic", 0, "network magic value")
	flag.BoolVar(&f.testnet, "testnet", false, fmt.Sprintf("alias for -network-magic=%d", TESTNET_MAGIC))
	flag.BoolVar(&f.mainnet, "mainnet", false, fmt.Sprintf("alias for -network-magic=%d", MAINNET_MAGIC))
	flag.StringVar(&f.syncEra, "sync-era", "byron", "era which to start chain-sync at")
	flag.Parse()

	var conn io.ReadWriteCloser
	var err error
	var dialProto string
	var dialAddress string
	if f.socket != "" {
		dialProto = "unix"
		dialAddress = f.socket
	} else if f.address != "" {
		dialProto = "tcp"
		dialAddress = f.address
	} else {
		fmt.Printf("You must specify one of -socket or -address\n\n")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if f.useTls {
		conn, err = tls.Dial(dialProto, dialAddress, nil)
	} else {
		conn, err = net.Dial(dialProto, dialAddress)
	}
	if err != nil {
		fmt.Printf("Connection failed: %s\n", err)
		os.Exit(1)
	}
	if f.networkMagic == 0 {
		if f.testnet {
			f.networkMagic = TESTNET_MAGIC
		} else if f.mainnet {
			f.networkMagic = MAINNET_MAGIC
		} else {
			fmt.Printf("You must specify one of -testnet, -mainnet, or -network-magic\n\n")
			flag.PrintDefaults()
			os.Exit(1)
		}
	}
	oOpts := &ouroboros.OuroborosOptions{
		Conn:         conn,
		NetworkMagic: uint32(f.networkMagic),
		ChainSyncCallbackConfig: &chainsync.ChainSyncCallbackConfig{
			AwaitReplyFunc:        chainSyncAwaitReplyHandler,
			RollBackwardFunc:      chainSyncRollBackwardHandler,
			RollForwardFunc:       chainSyncRollForwardHandler,
			IntersectFoundFunc:    chainSyncIntersectFoundHandler,
			IntersectNotFoundFunc: chainSyncIntersectNotFoundHandler,
			DoneFunc:              chainSyncDoneHandler,
		},
	}
	o, err := ouroboros.New(oOpts)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}
	go func() {
		for {
			err := <-o.ErrorChan
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
	}()
	// Test chain-sync
	if _, ok := eraIntersect[f.networkMagic][f.syncEra]; !ok {
		fmt.Printf("ERROR: unknown era '%s' specified as chain-sync start point\n", f.syncEra)
		os.Exit(1)
	}
	syncState.readyForNextBlockChan = make(chan bool, 0)
	intersect := []interface{}{}
	if len(eraIntersect[f.networkMagic][f.syncEra]) > 0 {
		// Slot
		intersect = append(intersect, eraIntersect[f.networkMagic][f.syncEra][0])
		// Block hash
		hash, _ := hex.DecodeString(eraIntersect[f.networkMagic][f.syncEra][1].(string))
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
	switch blockType {
	case block.BLOCK_TYPE_BYRON_EBB:
		fmt.Printf("found Byron EBB block\n")
	case block.BLOCK_TYPE_BYRON_MAIN:
		b := blockData.(block.ByronMainBlock)
		fmt.Printf("era = Byron, epoch = %d, slot = %d, prevBlock = %s\n", b.Header.ConsensusData.SlotId.Epoch, b.Header.ConsensusData.SlotId.Slot, b.Header.PrevBlock)
	case block.BLOCK_TYPE_SHELLEY:
		b := blockData.(block.ShelleyBlock)
		fmt.Printf("era = Shelley, slot = %d, block_no = %d, prevHash = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Header.Body.PrevHash)
	case block.BLOCK_TYPE_ALLEGRA:
		b := blockData.(block.AllegraBlock)
		fmt.Printf("era = Allegra, slot = %d, block_no = %d, prevHash = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Header.Body.PrevHash)
	case block.BLOCK_TYPE_MARY:
		b := blockData.(block.MaryBlock)
		fmt.Printf("era = Mary, slot = %d, block_no = %d, prevHash = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Header.Body.PrevHash)
	case block.BLOCK_TYPE_ALONZO:
		b := blockData.(block.AlonzoBlock)
		fmt.Printf("era = Alonzo, slot = %d, block_no = %d, prevHash = %s\n", b.Header.Body.Slot, b.Header.Body.BlockNumber, b.Header.Body.PrevHash)
	default:
		fmt.Printf("unsupported (so far) block type %d\n", blockType)
		fmt.Printf("%s\n", utils.DumpCborStructure(blockData, ""))
	}
	syncState.readyForNextBlockChan <- true
	return nil
}

func chainSyncIntersectFoundHandler(point interface{}, tip interface{}) error {
	fmt.Printf("found intersect: point = %#v, tip = %#v\n", point, tip)
	syncState.readyForNextBlockChan <- true
	return nil
}

func chainSyncIntersectNotFoundHandler() error {
	fmt.Printf("ERROR: failed to find intersection\n")
	os.Exit(1)
	return nil
}

func chainSyncDoneHandler() error {
	return nil
}
