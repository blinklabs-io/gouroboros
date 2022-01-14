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
}

type chainSyncState struct {
	readyForNextBlockChan chan bool
}

var syncState chainSyncState

func main() {
	f := cmdFlags{}
	flag.StringVar(&f.socket, "socket", "", "UNIX socket path to connect to")
	flag.StringVar(&f.address, "address", "", "TCP address to connect to in address:port format")
	flag.BoolVar(&f.useTls, "tls", false, "enable TLS")
	flag.IntVar(&f.networkMagic, "network-magic", 0, "network magic value")
	flag.BoolVar(&f.testnet, "testnet", false, fmt.Sprintf("alias for -network-magic=%d", TESTNET_MAGIC))
	flag.BoolVar(&f.mainnet, "mainnet", false, fmt.Sprintf("alias for -network-magic=%d", MAINNET_MAGIC))
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
	syncState.readyForNextBlockChan = make(chan bool, 0)
	secondToLastByronSlot := 1598398
	//lastByronBlockHash, _ := hex.DecodeString("7e16781b40ebf8b6da18f7b5e8ade855d6738095ef2f1c58c77e88b6e45997a4")
	secondToLastByronBlockHash, _ := hex.DecodeString("8542ae6166cc4affadefd44585488fef9a02aee7914e1e387ce5f7a33e6569c5")
	if err := o.ChainSync.FindIntersect([]interface{}{[]interface{}{secondToLastByronSlot, secondToLastByronBlockHash}}); err != nil {
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
		//fmt.Printf("resp = %#v, err = %#v\n", resp, err)
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
