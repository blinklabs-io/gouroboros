// Copyright 2023 Blink Labs Software
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
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"

	"github.com/blinklabs-io/gouroboros"
)

type globalFlags struct {
	flagset      *flag.FlagSet
	socket       string
	address      string
	useTls       bool
	ntnProto     bool
	network      string
	networkMagic int
}

func newGlobalFlags() *globalFlags {
	f := &globalFlags{
		flagset: flag.NewFlagSet(os.Args[0], flag.ExitOnError),
	}
	f.flagset.StringVar(
		&f.socket,
		"socket",
		"",
		"UNIX socket path to connect to",
	)
	f.flagset.StringVar(
		&f.address,
		"address",
		"",
		"TCP address to connect to in address:port format",
	)
	f.flagset.BoolVar(&f.useTls, "tls", false, "enable TLS")
	f.flagset.BoolVar(
		&f.ntnProto,
		"ntn",
		false,
		"use node-to-node protocol (defaults to node-to-client)",
	)
	f.flagset.StringVar(
		&f.network,
		"network",
		"preview",
		"specifies network that node is participating in",
	)
	f.flagset.IntVar(
		&f.networkMagic,
		"network-magic",
		0,
		"specifies network magic value. this overrides the -network option",
	)
	return f
}

func main() {
	f := newGlobalFlags()
	err := f.flagset.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("failed to parse command args: %s\n", err)
		os.Exit(1)
	}

	if f.networkMagic == 0 {
		network := ouroboros.NetworkByName(f.network)
		if network == ouroboros.NetworkInvalid {
			fmt.Printf("Invalid network specified: %s\n", f.network)
			os.Exit(1)
		}
		f.networkMagic = int(network.NetworkMagic)
	}

	if len(f.flagset.Args()) > 0 {
		switch f.flagset.Arg(0) {
		case "chain-sync":
			testChainSync(f)
		case "local-tx-submission":
			testLocalTxSubmission(f)
		case "server":
			testServer(f)
		case "query":
			testQuery(f)
		case "mem-usage":
			testMemUsage(f)
		default:
			fmt.Printf("Unknown subcommand: %s\n", f.flagset.Arg(0))
			os.Exit(1)
		}
	} else {
		fmt.Printf("You must specify a subcommand (chain-sync or local-tx-submission)\n")
		os.Exit(1)
	}
}

func createClientConnection(f *globalFlags) net.Conn {
	var err error
	var conn net.Conn
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
	return conn
}
