package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network"
	"github.com/cloudstruct/go-ouroboros-network/protocol/chainsync"
	"github.com/cloudstruct/go-ouroboros-network/protocol/common"
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
	for {
		resp, err := o.ChainSync.RequestNext()
		if err != nil {
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
		//fmt.Printf("resp = %#v, err = %#v\n", resp, err)
		switch resp.BlockType {
		case chainsync.BLOCK_TYPE_BYRON_EBB:
			fmt.Printf("found Byron EBB block\n")
		case chainsync.BLOCK_TYPE_BYRON_MAIN:
			block := resp.Block.(common.ByronMainBlock)
			fmt.Printf("epoch = %d, slot = %d, prevBlock = %s\n", block.Header.ConsensusData.SlotId.Epoch, block.Header.ConsensusData.SlotId.Slot, block.Header.PrevBlock)
		default:
			fmt.Printf("unsupported (so far) block type %d\n", resp.BlockType)
			fmt.Printf("%s\n", utils.DumpCborStructure(resp.Block, ""))
		}
	}
}
