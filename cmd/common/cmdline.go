// Copyright 2023 Blink Labs, LLC.
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

package common

import (
	"flag"
	"fmt"
	"os"

	"github.com/blinklabs-io/gouroboros"
)

type GlobalFlags struct {
	Flagset      *flag.FlagSet
	Socket       string
	Address      string
	UseTls       bool
	NtnProto     bool
	Network      string
	NetworkMagic int
}

func NewGlobalFlags() *GlobalFlags {
	f := &GlobalFlags{
		Flagset: flag.NewFlagSet(os.Args[0], flag.ExitOnError),
	}
	f.Flagset.StringVar(
		&f.Socket,
		"socket",
		"",
		"UNIX socket path to connect to",
	)
	f.Flagset.StringVar(
		&f.Address,
		"address",
		"",
		"TCP address to connect to in address:port format",
	)
	f.Flagset.BoolVar(&f.UseTls, "tls", false, "enable TLS")
	f.Flagset.BoolVar(
		&f.NtnProto,
		"ntn",
		false,
		"use node-to-node protocol (defaults to node-to-client)",
	)
	f.Flagset.StringVar(
		&f.Network,
		"network",
		"preview",
		"specifies network that node is participating in",
	)
	f.Flagset.IntVar(
		&f.NetworkMagic,
		"network-magic",
		0,
		"specifies network magic value. this overrides the -network option",
	)
	return f
}

func (f *GlobalFlags) Parse() {
	if err := f.Flagset.Parse(os.Args[1:]); err != nil {
		fmt.Printf("failed to parse command args: %s\n", err)
		os.Exit(1)
	}
	if f.NetworkMagic == 0 {
		network := ouroboros.NetworkByName(f.Network)
		if network == ouroboros.NetworkInvalid {
			fmt.Printf("Invalid network specified: %s\n", f.Network)
			os.Exit(1)
		}
		f.NetworkMagic = int(network.NetworkMagic)
	}
}
