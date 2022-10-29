package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/cloudstruct/go-cardano-ledger"
	ouroboros "github.com/cloudstruct/go-ouroboros-network"
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

func buildLocalTxSubmissionConfig() localtxsubmission.Config {
	return localtxsubmission.Config{
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
	go func() {
		for {
			err := <-errorChan
			fmt.Printf("ERROR: %s\n", err)
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
	o.LocalTxSubmission.Client.Start()

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

	if err = o.LocalTxSubmission.Client.SubmitTx(ledger.TX_TYPE_ALONZO, txBytes); err != nil {
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

func localTxSubmissionRejectTxHandler(reasonCbor []byte) error {
	fmt.Printf("The transaction was rejected (reason in hex-encoded CBOR): %#v\n", reasonCbor)
	os.Exit(1)
	return nil
}
