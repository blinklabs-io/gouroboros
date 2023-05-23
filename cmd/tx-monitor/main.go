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
	"encoding/hex"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/cmd/common"

	"golang.org/x/crypto/blake2b"
)

func main() {
	// Parse commandline
	f := common.NewGlobalFlags()
	f.Parse()
	// Create connection
	conn := common.CreateClientConnection(f)
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

	capacity, size, numberOfTxs, err := o.LocalTxMonitor().Client.GetSizes()
	if err != nil {
		fmt.Printf("ERROR(GetSizes): %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Mempool size/capacity (bytes): %d / %d, TXs: %d\n", size, capacity, numberOfTxs)

	fmt.Printf("Transactions:\n\n")
	for {
		tx, err := o.LocalTxMonitor().Client.NextTx()
		if err != nil {
			fmt.Printf("ERROR(NextTx): %s\n", err)
			os.Exit(1)
		}
		if tx == nil {
			break
		}
		// Unwrap raw transaction bytes into a CBOR array
		var txUnwrap []cbor.RawMessage
		if _, err := cbor.Decode(tx, &txUnwrap); err != nil {
			fmt.Printf("ERROR(unwrap): %s\n", err)
			os.Exit(1)
		}
		// index 0 is the transaction body
		// Store index 0 (transaction body) as byte array
		txBody := txUnwrap[0]
		// Convert the body into a blake2b256 hash string
		txIdHash := blake2b.Sum256(txBody)
		// Encode hash string as byte array to hex string
		txIdHex := hex.EncodeToString(txIdHash[:])
		fmt.Printf("%s\n", txIdHex)
	}
}
