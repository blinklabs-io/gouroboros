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
	"flag"
	"fmt"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
)

type queryFlags struct {
	flagset *flag.FlagSet
}

func newQueryFlags() *queryFlags {
	f := &queryFlags{
		flagset: flag.NewFlagSet("query", flag.ExitOnError),
	}
	return f
}

func buildLocalStateQueryConfig() localstatequery.Config {
	return localstatequery.NewConfig()
}

func testQuery(f *globalFlags) {
	queryFlags := newQueryFlags()
	err := queryFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}
	if len(queryFlags.flagset.Args()) < 1 {
		fmt.Printf("ERROR: you must specify a query\n")
		os.Exit(1)
	}

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
		ouroboros.WithLocalStateQueryConfig(buildLocalStateQueryConfig()),
	)
	if err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	switch queryFlags.flagset.Args()[0] {
	case "current-era":
		era, err := o.LocalStateQuery().Client.GetCurrentEra()
		if err != nil {
			fmt.Printf("ERROR: failure querying current era: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("current-era: %d\n", era)
	case "tip":
		era, err := o.LocalStateQuery().Client.GetCurrentEra()
		if err != nil {
			fmt.Printf("ERROR: failure querying current era: %s\n", err)
			os.Exit(1)
		}
		epochNo, err := o.LocalStateQuery().Client.GetEpochNo()
		if err != nil {
			fmt.Printf("ERROR: failure querying current epoch: %s\n", err)
			os.Exit(1)
		}
		blockNo, err := o.LocalStateQuery().Client.GetChainBlockNo()
		if err != nil {
			fmt.Printf(
				"ERROR: failure querying current chain block number: %s\n",
				err,
			)
			os.Exit(1)
		}
		point, err := o.LocalStateQuery().Client.GetChainPoint()
		if err != nil {
			fmt.Printf("ERROR: failure querying current chain point: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf(
			"tip: era = %d, epoch = %d, blockNo = %d, slot = %d, hash = %x\n",
			era,
			epochNo,
			blockNo,
			point.Slot,
			point.Hash,
		)
	case "system-start":
		systemStart, err := o.LocalStateQuery().Client.GetSystemStart()
		if err != nil {
			fmt.Printf("ERROR: failure querying system start: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf(
			"system-start: year = %d, day = %d, picoseconds = %d\n",
			systemStart.Year,
			systemStart.Day,
			systemStart.Picoseconds,
		)
	case "era-history":
		eraHistory, err := o.LocalStateQuery().Client.GetEraHistory()
		if err != nil {
			fmt.Printf("ERROR: failure querying era history: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("era-history:\n")
		for eraId, era := range eraHistory {
			fmt.Printf(
				"id = %d, begin slot/epoch = %d/%d, end slot/epoch = %d/%d, epoch length = %d, slot length (ms) = %d, slots per KES period = %d\n",
				eraId,
				era.Begin.SlotNo,
				era.Begin.EpochNo,
				era.End.SlotNo,
				era.End.EpochNo,
				era.Params.EpochLength,
				era.Params.SlotLength,
				era.Params.SlotsPerKESPeriod.Value,
			)
		}
	case "protocol-params":
		protoParams, err := o.LocalStateQuery().Client.GetCurrentProtocolParams()
		if err != nil {
			fmt.Printf("ERROR: failure querying protocol params: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("protocol-params: %#v\n", protoParams)
	case "stake-distribution":
		stakeDistribution, err := o.LocalStateQuery().Client.GetStakeDistribution()
		if err != nil {
			fmt.Printf("ERROR: failure querying stake distribution: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("stake-distribution: %#v\n", *stakeDistribution)
	case "genesis-config":
		genesisConfig, err := o.LocalStateQuery().Client.GetGenesisConfig()
		if err != nil {
			fmt.Printf("ERROR: failure querying genesis config: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("genesis-config: %#v\n", *genesisConfig)
	default:
		fmt.Printf("ERROR: unknown query: %s\n", queryFlags.flagset.Args()[0])
		os.Exit(1)
	}
}
