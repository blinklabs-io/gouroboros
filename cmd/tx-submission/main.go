// Copyright 2024 Blink Labs Software
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
	"fmt"
	"os"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cmd/common"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

type txSubmissionFlags struct {
	*common.GlobalFlags
	txFile    string
	rawTxFile string
}

var txBytes []byte
var txHash [32]byte
var sentTx bool
var doneChan chan any

func main() {
	// Parse commandline
	f := txSubmissionFlags{
		GlobalFlags: common.NewGlobalFlags(),
	}
	f.Flagset.StringVar(
		&f.txFile,
		"tx-file",
		"",
		"path to the JSON transaction file to submit",
	)
	f.Flagset.StringVar(
		&f.rawTxFile,
		"raw-tx-file",
		"",
		"path to the raw transaction file to submit",
	)
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
		ouroboros.WithTxSubmissionConfig(
			txsubmission.NewConfig(
				txsubmission.WithRequestTxIdsFunc(handleRequestTxIds),
				txsubmission.WithRequestTxsFunc(handleRequestTxs),
			),
		),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	// Read the transaction file
	if f.txFile != "" {
		txData, err := os.ReadFile(f.txFile)
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
	} else if f.rawTxFile != "" {
		txBytes, err = os.ReadFile(f.rawTxFile)
		if err != nil {
			fmt.Printf("Failed to load transaction file: %s\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("You must specify one of -tx-file or -raw-tx-file\n")
		os.Exit(1)
	}

	// convert to tx
	txType, err := ledger.DetermineTransactionType(txBytes)
	if err != nil {
		fmt.Printf("ERROR: could not parse transaction to determine type: %s", err)
		os.Exit(1)
	}
	tx, err := ledger.NewTransactionFromCbor(txType, txBytes)
	if err != nil {
		fmt.Printf("failed to parse transaction CBOR: %s", err)
		os.Exit(1)
	}

	// Create our "done" channel
	doneChan = make(chan any)

	// Start the TxSubmission activity loop
	o.TxSubmission().Client.Init()

	// Wait until we're done
	<-doneChan

	fmt.Printf("Successfully sent transaction %x\n", tx.Hash())

	if err := o.Close(); err != nil {
		fmt.Printf("ERROR: failed to close connection: %s\n", err)
		os.Exit(1)
	}
}

func handleRequestTxIds(
	ctx txsubmission.CallbackContext,
	blocking bool,
	ack uint16,
	req uint16,
) ([]txsubmission.TxIdAndSize, error) {
	if sentTx {
		// Terrible syncronization hack for shutdown
		close(doneChan)
		time.Sleep(5 * time.Second)
	}
	ret := []txsubmission.TxIdAndSize{
		{
			TxId: txsubmission.TxId{
				EraId: 5,
				TxId:  txHash,
			},
			Size: uint32(len(txBytes)),
		},
	}
	return ret, nil
}

func handleRequestTxs(
	ctx txsubmission.CallbackContext,
	txIds []txsubmission.TxId,
) ([]txsubmission.TxBody, error) {
	ret := []txsubmission.TxBody{
		{
			EraId:  5,
			TxBody: txBytes,
		},
	}
	sentTx = true
	return ret, nil
}
