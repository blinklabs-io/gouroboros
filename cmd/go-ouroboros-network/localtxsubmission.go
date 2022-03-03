package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	ouroboros "github.com/cloudstruct/go-ouroboros-network"
	"github.com/cloudstruct/go-ouroboros-network/block"
	"github.com/cloudstruct/go-ouroboros-network/protocol/localtxsubmission"
	"io/ioutil"
	"os"
)

type localTxSubmissionState struct {
	submitResponse chan bool
}

var localTxSubmitState localTxSubmissionState

type localTxSubmissionFlags struct {
	flagset *flag.FlagSet
	txFile  string
}

func newLocalTxSubmissionFlags() *localTxSubmissionFlags {
	f := &localTxSubmissionFlags{
		flagset: flag.NewFlagSet("local-tx-submission", flag.ExitOnError),
	}
	f.flagset.StringVar(&f.txFile, "tx-file", "", "path to the transaction file to submit")
	return f
}

func buildLocalTxSubmissionCallbackConfig() *localtxsubmission.CallbackConfig {
	return &localtxsubmission.CallbackConfig{
		AcceptTxFunc: localTxSubmissionAcceptTxHandler,
		RejectTxFunc: localTxSubmissionRejectTxHandler,
	}
}

func testLocalTxSubmission(f *globalFlags) {
	localTxSubmissionFlags := newLocalTxSubmissionFlags()
	err := localTxSubmissionFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}

	localTxSubmitState.submitResponse = make(chan bool)

	conn := createClientConnection(f)
	errorChan := make(chan error)
	oOpts := &ouroboros.OuroborosOptions{
		Conn:                            conn,
		NetworkMagic:                    uint32(f.networkMagic),
		ErrorChan:                       errorChan,
		UseNodeToNodeProtocol:           f.ntnProto,
		SendKeepAlives:                  true,
		LocalTxSubmissionCallbackConfig: buildLocalTxSubmissionCallbackConfig(),
	}
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}
	}()
	o, err := ouroboros.New(oOpts)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	txData, err := ioutil.ReadFile(localTxSubmissionFlags.txFile)
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

	txBytes, err := hex.DecodeString(jsonData["cborHex"])
	if err != nil {
		fmt.Printf("failed to decode transaction: %s\n", err)
		os.Exit(1)
	}

	if err = o.LocalTxSubmission.SubmitTx(block.TX_TYPE_ALONZO, txBytes); err != nil {
		fmt.Printf("Error submitting transaction: %s\n", err)
		os.Exit(1)
	}

	// Wait for response
	<-localTxSubmitState.submitResponse
}

func localTxSubmissionAcceptTxHandler() error {
	fmt.Print("The transaction was accepted\n")
	localTxSubmitState.submitResponse <- true
	return nil
}

func localTxSubmissionRejectTxHandler(reason interface{}) error {
	fmt.Printf("The transaction was rejected: %#v\n", reason)
	os.Exit(1)
	return nil
}
