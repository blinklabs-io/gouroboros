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
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
)

type localTxSubmissionFlags struct {
	flagset   *flag.FlagSet
	txFile    string
	rawTxFile string
}

func newLocalTxSubmissionFlags() *localTxSubmissionFlags {
	f := &localTxSubmissionFlags{
		flagset: flag.NewFlagSet("local-tx-submission", flag.ExitOnError),
	}
	f.flagset.StringVar(
		&f.txFile,
		"tx-file",
		"",
		"path to the JSON transaction file to submit",
	)
	f.flagset.StringVar(
		&f.rawTxFile,
		"raw-tx-file",
		"",
		"path to the raw transaction file to submit",
	)
	return f
}

func buildLocalTxSubmissionConfig() localtxsubmission.Config {
	return localtxsubmission.NewConfig()
}

func testLocalTxSubmission(f *globalFlags) {
	localTxSubmissionFlags := newLocalTxSubmissionFlags()
	err := localTxSubmissionFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}
	if localTxSubmissionFlags.txFile == "" &&
		localTxSubmissionFlags.rawTxFile == "" {
		fmt.Printf("you must specify -tx-file or -raw-tx-file\n")
		os.Exit(1)
	}

	conn := createClientConnection(f)
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
		ouroboros.WithNetworkMagic(uint32(f.networkMagic)),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(f.ntnProto),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithLocalTxSubmissionConfig(buildLocalTxSubmissionConfig()),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	var txBytes []byte
	if localTxSubmissionFlags.txFile != "" {
		txData, err := os.ReadFile(localTxSubmissionFlags.txFile)
		if err != nil {
			fmt.Printf("Failed to load transaction file: %s\n", err)
			os.Exit(1)
		}

		var jsonData map[string]string
		err = json.Unmarshal(txData, &jsonData)
		if err != nil {
			fmt.Printf("failed to parse transaction file: %s\n", err)
			os.Exit(1)
		}

		txBytes, err = hex.DecodeString(jsonData["cborHex"])
		if err != nil {
			fmt.Printf("failed to decode transaction: %s\n", err)
			os.Exit(1)
		}
	} else {
		txBytes, err = os.ReadFile(localTxSubmissionFlags.rawTxFile)
		if err != nil {
			fmt.Printf("Failed to load transaction file: %s\n", err)
			os.Exit(1)
		}
	}

	if err = o.LocalTxSubmission().Client.SubmitTx(ledger.TxTypeAlonzo, txBytes); err != nil {
		fmt.Printf("Error submitting transaction: %s\n", err)
		os.Exit(1)
	}
	fmt.Print("The transaction was accepted\n")
}
