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
	"crypto/tls"
	"fmt"
	"net"
	"os"
)

func CreateClientConnection(f *GlobalFlags) net.Conn {
	var err error
	var conn net.Conn
	var dialProto string
	var dialAddress string
	if f.Socket != "" {
		dialProto = "unix"
		dialAddress = f.Socket
	} else if f.Address != "" {
		dialProto = "tcp"
		dialAddress = f.Address
	} else {
		fmt.Printf("You must specify one of -socket or -address\n\n")
		f.Flagset.PrintDefaults()
		os.Exit(1)
	}
	if f.UseTls {
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
