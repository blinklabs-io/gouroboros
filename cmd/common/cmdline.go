package common

import (
	"flag"
	"fmt"
	"os"
)

const (
	TESTNET_MAGIC = 1097911063
	MAINNET_MAGIC = 764824073
	PREPROD_MAGIC = 1
	PREVIEW_MAGIC = 2
)

type GlobalFlags struct {
	Flagset      *flag.FlagSet
	Socket       string
	Address      string
	UseTls       bool
	NtnProto     bool
	NetworkMagic int
	Testnet      bool
	Mainnet      bool
	Preprod      bool
	Preview      bool
}

func NewGlobalFlags() *GlobalFlags {
	f := &GlobalFlags{
		Flagset: flag.NewFlagSet(os.Args[0], flag.ExitOnError),
	}
	f.Flagset.StringVar(&f.Socket, "socket", "", "UNIX socket path to connect to")
	f.Flagset.StringVar(&f.Address, "address", "", "TCP address to connect to in address:port format")
	f.Flagset.BoolVar(&f.UseTls, "tls", false, "enable TLS")
	f.Flagset.BoolVar(&f.NtnProto, "ntn", false, "use node-to-node protocol (defaults to node-to-client)")
	f.Flagset.IntVar(&f.NetworkMagic, "network-magic", 0, "network magic value")
	f.Flagset.BoolVar(&f.Testnet, "testnet", false, fmt.Sprintf("alias for -network-magic=%d", TESTNET_MAGIC))
	f.Flagset.BoolVar(&f.Mainnet, "mainnet", false, fmt.Sprintf("alias for -network-magic=%d", MAINNET_MAGIC))
	f.Flagset.BoolVar(&f.Preprod, "preprod", false, fmt.Sprintf("alias for -network-magic=%d", PREPROD_MAGIC))
	f.Flagset.BoolVar(&f.Preview, "preview", false, fmt.Sprintf("alias for -network-magic=%d", PREVIEW_MAGIC))
	return f
}

func (f *GlobalFlags) Parse() {
	if err := f.Flagset.Parse(os.Args[1:]); err != nil {
		fmt.Printf("failed to parse command args: %s\n", err)
		os.Exit(1)
	}
	if f.NetworkMagic == 0 {
		if f.Testnet {
			f.NetworkMagic = TESTNET_MAGIC
		} else if f.Mainnet {
			f.NetworkMagic = MAINNET_MAGIC
		} else if f.Preprod {
			f.NetworkMagic = PREPROD_MAGIC
		} else if f.Preview {
			f.NetworkMagic = PREVIEW_MAGIC
		} else {
			fmt.Printf("You must specify one of -testnet, -mainnet, -preprod, -preview, or -network-magic\n\n")
			f.Flagset.PrintDefaults()
			os.Exit(1)
		}
	}
}
