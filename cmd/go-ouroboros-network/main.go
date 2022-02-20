package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	ouroboros "github.com/cloudstruct/go-ouroboros-network"
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
	ntnProto     bool
	networkMagic int
	testnet      bool
	mainnet      bool
	syncEra      string
}

func main() {
	f := cmdFlags{}
	flag.StringVar(&f.socket, "socket", "", "UNIX socket path to connect to")
	flag.StringVar(&f.address, "address", "", "TCP address to connect to in address:port format")
	flag.BoolVar(&f.useTls, "tls", false, "enable TLS")
	flag.BoolVar(&f.ntnProto, "ntn", false, "use node-to-node protocol (defaults to node-to-client)")
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
	errorChan := make(chan error, 10)
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
	testChainSync(o, f)
}
