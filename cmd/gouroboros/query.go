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
	"strconv"
	"strings"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
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
	case "stake-pools":
		stakePools, err := o.LocalStateQuery().Client.GetStakePools()
		if err != nil {
			fmt.Printf("ERROR: failure querying stake pools: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("stake-pools: %#v\n", *stakePools)
	case "genesis-config":
		genesisConfig, err := o.LocalStateQuery().Client.GetGenesisConfig()
		if err != nil {
			fmt.Printf("ERROR: failure querying genesis config: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("genesis-config: %#v\n", *genesisConfig)
	case "pool-params":
		var tmpPools []ledger.PoolId
		if len(queryFlags.flagset.Args()) <= 1 {
			fmt.Println("No pools specified")
			os.Exit(1)
		}
		for _, pool := range queryFlags.flagset.Args()[1:] {
			tmpPoolId, err := ledger.NewPoolIdFromBech32(pool)
			if err != nil {
				fmt.Printf("Invalid bech32 pool ID %q: %s", pool, err)
				os.Exit(1)
			}
			tmpPools = append(tmpPools, tmpPoolId)
		}
		poolParams, err := o.LocalStateQuery().Client.GetStakePoolParams(tmpPools)
		if err != nil {
			fmt.Printf("ERROR: failure querying stake pool params: %s\n", err)
			os.Exit(1)
		}
		fmt.Printf("pool-params: %#v\n", *poolParams)
	case "utxos-by-address":
		var tmpAddrs []ledger.Address
		if len(queryFlags.flagset.Args()) <= 1 {
			fmt.Println("No addresses specified")
			os.Exit(1)
		}
		for _, addr := range queryFlags.flagset.Args()[1:] {
			tmpAddr, err := ledger.NewAddress(addr)
			if err != nil {
				fmt.Printf("Invalid address %q: %s", addr, err)
				os.Exit(1)
			}
			tmpAddrs = append(tmpAddrs, tmpAddr)
		}
		utxos, err := o.LocalStateQuery().Client.GetUTxOByAddress(tmpAddrs)
		if err != nil {
			fmt.Printf("ERROR: failure querying UTxOs by address: %s\n", err)
			os.Exit(1)
		}
		for utxoId, utxo := range utxos.Results {
			fmt.Println("---")
			fmt.Printf("UTxO ID: %s#%d\n", utxoId.Hash.String(), utxoId.Idx)
			fmt.Printf("Amount: %d\n", utxo.OutputAmount.Amount)
			if utxo.OutputAmount.Assets != nil {
				assetsJson, err := json.Marshal(utxo.OutputAmount.Assets)
				if err != nil {
					fmt.Printf("ERROR: failed to marshal asset JSON: %s\n", err)
					os.Exit(1)
				}
				fmt.Printf("Assets: %s\n", assetsJson)
			}
		}
	case "utxos-by-txin":
		var tmpTxIns []ledger.TransactionInput
		if len(queryFlags.flagset.Args()) <= 1 {
			fmt.Println("No UTxO IDs specified")
			os.Exit(1)
		}
		for _, txIn := range queryFlags.flagset.Args()[1:] {
			txInParts := strings.SplitN(txIn, `#`, 2)
			if len(txInParts) != 2 {
				fmt.Printf("Invalid UTxO ID %q", txIn)
				os.Exit(1)
			}
			txIdHex, err := hex.DecodeString(txInParts[0])
			if err != nil {
				fmt.Printf("Invalid UTxO ID %q: %s", txIn, err)
				os.Exit(1)
			}
			txOutputIdx, err := strconv.ParseUint(txInParts[1], 10, 32)
			if err != nil {
				fmt.Printf("Invalid UTxO ID %q: %s", txIn, err)
			}
			tmpTxIn := ledger.ShelleyTransactionInput{
				TxId:        ledger.Blake2b256(txIdHex),
				OutputIndex: uint32(txOutputIdx),
			}
			tmpTxIns = append(tmpTxIns, tmpTxIn)
		}
		utxos, err := o.LocalStateQuery().Client.GetUTxOByTxIn(tmpTxIns)
		if err != nil {
			fmt.Printf("ERROR: failure querying UTxOs by TxIn: %s\n", err)
			os.Exit(1)
		}
		for utxoId, utxo := range utxos.Results {
			fmt.Println("---")
			fmt.Printf("UTxO ID: %s#%d\n", utxoId.Hash.String(), utxoId.Idx)
			fmt.Printf("Amount: %d\n", utxo.OutputAmount.Amount)
			if utxo.OutputAmount.Assets != nil {
				assetsJson, err := json.Marshal(utxo.OutputAmount.Assets)
				if err != nil {
					fmt.Printf("ERROR: failed to marshal asset JSON: %s\n", err)
					os.Exit(1)
				}
				fmt.Printf("Assets: %s\n", assetsJson)
			}
		}
	default:
		fmt.Printf("ERROR: unknown query: %s\n", queryFlags.flagset.Args()[0])
		os.Exit(1)
	}
}
