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

type globalFlags struct {
	flagset      *flag.FlagSet
	socket       string
	address      string
	useTls       bool
	ntnProto     bool
	networkMagic int
	testnet      bool
	mainnet      bool
	syncEra      string
}

func newGlobalFlags() *globalFlags {
	f := &globalFlags{
		flagset: flag.NewFlagSet(os.Args[0], flag.ExitOnError),
	}
	f.flagset.StringVar(&f.socket, "socket", "", "UNIX socket path to connect to")
	f.flagset.StringVar(&f.address, "address", "", "TCP address to connect to in address:port format")
	f.flagset.BoolVar(&f.useTls, "tls", false, "enable TLS")
	f.flagset.BoolVar(&f.ntnProto, "ntn", false, "use node-to-node protocol (defaults to node-to-client)")
	f.flagset.IntVar(&f.networkMagic, "network-magic", 0, "network magic value")
	f.flagset.BoolVar(&f.testnet, "testnet", false, fmt.Sprintf("alias for -network-magic=%d", TESTNET_MAGIC))
	f.flagset.BoolVar(&f.mainnet, "mainnet", false, fmt.Sprintf("alias for -network-magic=%d", MAINNET_MAGIC))
	f.flagset.StringVar(&f.syncEra, "sync-era", "byron", "era which to start chain-sync at")
	return f
}

func main() {
	f := newGlobalFlags()
	err := f.flagset.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("failed to parse command args: %s\n", err)
		os.Exit(1)
	}

	var conn io.ReadWriteCloser
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
		Conn:                            conn,
		NetworkMagic:                    uint32(f.networkMagic),
		ErrorChan:                       errorChan,
		UseNodeToNodeProtocol:           f.ntnProto,
		SendKeepAlives:                  true,
		ChainSyncCallbackConfig:         buildChainSyncCallbackConfig(),
		BlockFetchCallbackConfig:        buildBlockFetchCallbackConfig(),
		LocalTxSubmissionCallbackConfig: buildLocalTxSubmissionCallbackConfig(),
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
	if len(f.flagset.Args()) > 0 {
		switch f.flagset.Arg(0) {
		case "chain-sync":
			testChainSync(o, f)
		case "local-tx-submission":
			testLocalTxSubmission(o, f)
		default:
			fmt.Printf("Unknown subcommand: %s\n", f.flagset.Arg(0))
			os.Exit(1)
		}
	} else {
		fmt.Printf("You must specify a subcommand (chain-sync or local-tx-submission)\n")
		os.Exit(1)
	}
}
