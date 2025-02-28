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
	"flag"
	"fmt"
	"net"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
)

type serverFlags struct {
	flagset *flag.FlagSet
	//txFile  string
}

func newServerFlags() *serverFlags {
	f := &serverFlags{
		flagset: flag.NewFlagSet("server", flag.ExitOnError),
	}
	//f.flagset.StringVar(&f.txFile, "tx-file", "", "path to the transaction file to submit")
	return f
}

func createListenerSocket(f *globalFlags) (net.Listener, error) {
	var err error
	var listen net.Listener

	switch {
	case f.socket != "":
		if err := os.Remove(f.socket); err != nil {
			return nil, fmt.Errorf("failed to remove existing socket: %w", err)
		}
		listen, err = net.Listen("unix", f.socket)
		if err != nil {
			return nil, fmt.Errorf("failed to open listening socket: %w", err)
		}
	case f.address != "":
		listen, err = net.Listen("tcp", f.address)
		if err != nil {
			return nil, fmt.Errorf("failed to open listening socket: %w", err)
		}
	default:
		return nil, fmt.Errorf("no listening address or socket specified")
	}

	return listen, nil
}

func testServer(f *globalFlags) {
	serverFlags := newServerFlags()
	err := serverFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}

	listen, err := createListenerSocket(f)
	if err != nil {
		fmt.Printf("ERROR: failed to create listener: %s\n", err)
		os.Exit(1)
	}

	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Printf("ERROR: failed to accept connection: %s\n", err)
			continue
		}
		errorChan := make(chan error)
		go func() {
			for {
				err := <-errorChan
				fmt.Printf("ERROR: %s\n", err)
			}
		}()
		_, err = ouroboros.New(
			ouroboros.WithConnection(conn),
			ouroboros.WithNetworkMagic(uint32(f.networkMagic)), // #nosec G115
			ouroboros.WithErrorChan(errorChan),
			ouroboros.WithNodeToNode(f.ntnProto),
			ouroboros.WithServer(true),
		)
		if err != nil {
			fmt.Printf("ERROR: %s\n", err)
		}
		fmt.Printf("handshake completed...disconnecting\n")
		conn.Close()
	}
}
