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

package main

import (
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cmd/common"
)

type txSubmissionFlags struct {
	*common.GlobalFlags
}

func main() {
	// Parse commandline
	f := txSubmissionFlags{
		GlobalFlags: common.NewGlobalFlags(),
	}
	f.Parse()
	// Create connection
	conn := common.CreateClientConnection(f.GlobalFlags)
	errorChan := make(chan error)
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR(async): %s\n", err)
			os.Exit(1)
		}
	}()
	o, err := ouroboros.New(
		ouroboros.WithConnection(conn),
		ouroboros.WithNetworkMagic(uint32(f.NetworkMagic)),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(f.NtnProto),
		ouroboros.WithKeepAlive(true),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	tip, err := o.ChainSync().Client.GetCurrentTip()
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	fmt.Print("Current chain tip:\n\n")
	fmt.Printf("Block hash: %x\n", tip.Point.Hash)
	fmt.Printf("Slot number: %d\n", tip.Point.Slot)
}
