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

type localTxSubmissionFlags struct {
	flagset   *flag.FlagSet
	txFile    string
	rawTxFile string
}

func newLocalTxSubmissionFlags() *localTxSubmissionFlags {
	f := &localTxSubmissionFlags{
		flagset: flag.NewFlagSet("local-tx-submission", flag.ExitOnError),
	}
	f.flagset.StringVar(&f.txFile, "tx-file", "", "path to the JSON transaction file to submit")
	f.flagset.StringVar(&f.rawTxFile, "raw-tx-file", "", "path to the raw transaction file to submit")
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
	if localTxSubmissionFlags.txFile == "" && localTxSubmissionFlags.rawTxFile == "" {
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
	o.LocalTxSubmission().Client.Start()

	var txBytes []byte
	if localTxSubmissionFlags.txFile != "" {
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

		txBytes, err = hex.DecodeString(jsonData["cborHex"])
		if err != nil {
			fmt.Printf("failed to decode transaction: %s\n", err)
			os.Exit(1)
		}
	} else {
		txBytes, err = ioutil.ReadFile(localTxSubmissionFlags.rawTxFile)
		if err != nil {
			fmt.Printf("Failed to load transaction file: %s\n", err)
			os.Exit(1)
		}
	}

	if err = o.LocalTxSubmission().Client.SubmitTx(ledger.TX_TYPE_ALONZO, txBytes); err != nil {
		fmt.Printf("Error submitting transaction: %s\n", err)
		os.Exit(1)
	}
	fmt.Print("The transaction was accepted\n")
}
