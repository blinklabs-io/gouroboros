// Copyright 2025 Blink Labs Software
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
	"encoding/json"
	"flag"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/kelseyhightower/envconfig"
)

// We parse environment variables using envconfig into this struct
type Config struct {
	Magic      uint32
	SocketPath string `split_words:"true"`
	Network    string
	TxFile     string
	RawTxFile  string
}

// This code will be executed when run
func main() {
	// Set config defaults
	cfg := Config{
		SocketPath: "/ipc/node.socket",
	}
	// Parse environment variables
	if err := envconfig.Process("cardano_node", &cfg); err != nil {
		panic(err)
	}

	// Parse command-line flags
	flag.StringVar(&cfg.TxFile, "tx-file", "", "path to the JSON transaction file to submit")
	flag.StringVar(&cfg.RawTxFile, "raw-tx-file", "", "path to the raw transaction file to submit")
	flag.Parse()

	// Validate that at least one transaction file is provided
	if cfg.TxFile == "" && cfg.RawTxFile == "" {
		fmt.Printf("ERROR: you must specify -tx-file or -raw-tx-file\n")
		os.Exit(1)
	}

	// Configure NetworkMagic
	if cfg.Magic == 0 {
		if cfg.Network == "" {
			// Default to preview network if not specified
			cfg.Network = "preview"
		}
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if !ok {
			fmt.Printf("ERROR: invalid network specified: %v\n", cfg.Network)
			os.Exit(1)
		}
		cfg.Magic = network.NetworkMagic
	}

	// Create error channel
	errorChan := make(chan error)
	// Start error handler
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
	}()

	// Configure Ouroboros
	o, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(cfg.Magic),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(false), // Use NtC protocol (UNIX socket)
		ouroboros.WithKeepAlive(true),
		ouroboros.WithLocalTxSubmissionConfig(localtxsubmission.NewConfig()),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	// Connect to Node socket
	if err = o.Dial("unix", cfg.SocketPath); err != nil {
		fmt.Printf("ERROR: connection failed: %s\n", err)
		os.Exit(1)
	}

	// Load transaction bytes
	var txBytes []byte
	if cfg.TxFile != "" {
		txData, err := os.ReadFile(cfg.TxFile)
		if err != nil {
			fmt.Printf("ERROR: failed to load transaction file: %s\n", err)
			os.Exit(1)
		}

		var jsonData map[string]string
		err = json.Unmarshal(txData, &jsonData)
		if err != nil {
			fmt.Printf("ERROR: failed to parse transaction file: %s\n", err)
			os.Exit(1)
		}

		txBytes, err = hex.DecodeString(jsonData["cborHex"])
		if err != nil {
			fmt.Printf("ERROR: failed to decode transaction: %s\n", err)
			os.Exit(1)
		}
	} else {
		txBytes, err = os.ReadFile(cfg.RawTxFile)
		if err != nil {
			fmt.Printf("ERROR: failed to load transaction file: %s\n", err)
			os.Exit(1)
		}
	}

	// Determine transaction type from raw bytes
	txType, err := ledger.DetermineTransactionType(txBytes)
	if err != nil {
		fmt.Printf("ERROR: failed to determine transaction type: %s\n", err)
		os.Exit(1)
	}

	// Submit transaction
	if err = o.LocalTxSubmission().Client.SubmitTx(uint16(txType) /* #nosec G115 */, txBytes); err != nil {
		fmt.Printf("ERROR: failed to submit transaction: %s\n", err)
		os.Exit(1)
	}

	fmt.Print("The transaction was accepted\n")
}
