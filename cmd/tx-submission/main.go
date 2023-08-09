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
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
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
		ouroboros.WithTxSubmissionConfig(
			txsubmission.NewConfig(
				txsubmission.WithRequestTxIdsFunc(
					// TODO: do something more useful
					func(blocking bool, ack uint16, req uint16) ([]txsubmission.TxIdAndSize, error) {
						return []txsubmission.TxIdAndSize{}, nil
					},
				),
				txsubmission.WithRequestTxsFunc(
					// TODO: do something more useful
					func(txIds []txsubmission.TxId) ([]txsubmission.TxBody, error) {
						return []txsubmission.TxBody{}, nil
					},
				),
			),
		),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	// Start the TxSubmission activity loop
	o.TxSubmission().Client.Init()

	// Wait forever
	select {}
}
