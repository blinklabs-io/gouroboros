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
	"fmt"
	"math/big"
	"os"
	"reflect"
	"strconv"
	"strings"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/kelseyhightower/envconfig"
)

// We parse environment variables using envconfig into this struct
type Config struct {
	Magic      uint32
	Network    string
	SocketPath string `split_words:"true"`
}

// convertToJSONValue recursively converts values to JSON-serializable types
func convertToJSONValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			keyStr := fmt.Sprintf("%v", k)
			result[keyStr] = convertToJSONValue(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = convertToJSONValue(v)
		}
		return result
	default:
		// Use reflection to handle structs, maps, and other types
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Ptr {
			if rv.IsNil() {
				return nil
			}
			rv = rv.Elem()
		}
		//nolint:exhaustive // We handle all cases with default fallback
		switch rv.Kind() {
		case reflect.Map:
			// Convert map with any key type to map[string]interface{}
			result := make(map[string]interface{})
			for _, key := range rv.MapKeys() {
				keyStr := fmt.Sprintf("%v", key.Interface())
				result[keyStr] = convertToJSONValue(rv.MapIndex(key).Interface())
			}
			return result
		case reflect.Struct:
			// Convert struct to map for JSON
			result := make(map[string]interface{})
			rt := rv.Type()
			for i := 0; i < rv.NumField(); i++ {
				field := rt.Field(i)
				// Skip unexported fields
				if !field.IsExported() {
					continue
				}
				fieldValue := rv.Field(i).Interface()
				result[field.Name] = convertToJSONValue(fieldValue)
			}
			return result
		case reflect.Slice:
			// Convert slice to []interface{}
			if rv.IsNil() {
				return nil
			}
			result := make([]interface{}, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				result[i] = convertToJSONValue(rv.Index(i).Interface())
			}
			return result
		default:
			// For all other types (int, string, bool, etc.), return as-is
			return v
		}
	}
}

// convertStructToJSONSafe converts a struct to a JSON-safe representation
func convertStructToJSONSafe(v interface{}) interface{} {
	// Use reflection-based conversion which handles map[interface{}]interface{}
	return convertToJSONValue(v)
}

// This code will be executed when run
func main() {
	// Set config defaults
	cfg := Config{
		Magic:      0,
		SocketPath: "/ipc/node.socket",
	}
	// Parse environment variables
	if err := envconfig.Process("cardano_node", &cfg); err != nil {
		panic(err)
	}
	// Auto-resolve network magic if not provided
	if cfg.Magic == 0 && cfg.Network != "" {
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if ok {
			cfg.Magic = network.NetworkMagic
		}
	}
	// Check that we have a query type
	if len(os.Args) < 2 {
		fmt.Println("Usage: state-query <query-type> [arguments...]")
		fmt.Println()
		fmt.Println("Query types:")
		fmt.Println("  current-era")
		fmt.Println("  tip")
		fmt.Println("  system-start")
		fmt.Println("  era-history")
		fmt.Println("  protocol-params")
		fmt.Println("  stake-distribution")
		fmt.Println("  stake-pools")
		fmt.Println("  genesis-config")
		fmt.Println("  pool-params <pool-id> [pool-id...]")
		fmt.Println("  utxos-by-address <address> [address...]")
		fmt.Println("  utxos-by-txin <txid#idx> [txid#idx...]")
		fmt.Println("  utxo-whole-result [limit]  (WARNING: May timeout on large networks)")
		os.Exit(1)
	}
	queryType := os.Args[1]
	// Create error channel
	errorChan := make(chan error, 1)
	// start error handler
	go func() {
		for {
			err := <-errorChan
			panic(err)
		}
	}()
	// Configure Ouroboros
	o, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(uint32(cfg.Magic)),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(false),
	)
	if err != nil {
		panic(err)
	}
	// Connect to Node socket
	if err = o.Dial("unix", cfg.SocketPath); err != nil {
		panic(err)
	}
	// Execute query based on type
	switch queryType {
	case "current-era":
		era, err := o.LocalStateQuery().Client.GetCurrentEra()
		if err != nil {
			panic(fmt.Errorf("failure querying current era: %w", err))
		}
		fmt.Printf("current-era: %d\n", era)
	case "tip":
		era, err := o.LocalStateQuery().Client.GetCurrentEra()
		if err != nil {
			panic(fmt.Errorf("failure querying current era: %w", err))
		}
		epochNo, err := o.LocalStateQuery().Client.GetEpochNo()
		if err != nil {
			panic(fmt.Errorf("failure querying current epoch: %w", err))
		}
		blockNo, err := o.LocalStateQuery().Client.GetChainBlockNo()
		if err != nil {
			panic(fmt.Errorf("failure querying current chain block number: %w", err))
		}
		point, err := o.LocalStateQuery().Client.GetChainPoint()
		if err != nil {
			panic(fmt.Errorf("failure querying current chain point: %w", err))
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
			panic(fmt.Errorf("failure querying system start: %w", err))
		}
		fmt.Printf(
			"system-start: year = %v, day = %d, picoseconds = %v\n",
			systemStart.Year,
			systemStart.Day,
			systemStart.Picoseconds,
		)
	case "era-history":
		eraHistory, err := o.LocalStateQuery().Client.GetEraHistory()
		if err != nil {
			panic(fmt.Errorf("failure querying era history: %w", err))
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
			panic(fmt.Errorf("failure querying protocol params: %w", err))
		}
		// Marshal to JSON for readable output
		jsonData, err := json.MarshalIndent(protoParams, "", "  ")
		if err != nil {
			panic(fmt.Errorf("failure marshaling protocol params to JSON: %w", err))
		}
		fmt.Printf("protocol-params:\n%s\n", string(jsonData))
	case "stake-distribution":
		stakeDistribution, err := o.LocalStateQuery().Client.GetStakeDistribution()
		if err != nil {
			panic(fmt.Errorf("failure querying stake distribution: %w", err))
		}
		fmt.Printf("stake-distribution:\n")
		for poolID, entry := range stakeDistribution.Results {
			stakePercent := "N/A"
			if entry.StakeFraction != nil {
				// Convert to percentage: (num/denom) * 100
				hundred := big.NewRat(100, 1)
				percent := new(big.Rat).Mul(entry.StakeFraction.Rat, hundred)
				stakePercent = percent.FloatString(6) + "%"
			}
			vrfHashStr := "N/A"
			if entry.VrfHash != (ledger.Blake2b256{}) {
				vrfHashStr = entry.VrfHash.String()
			}
			fmt.Printf("  Pool: %s, Stake: %s, VRF Hash: %s\n", poolID.String(), stakePercent, vrfHashStr)
		}
	case "stake-pools":
		stakePools, err := o.LocalStateQuery().Client.GetStakePools()
		if err != nil {
			panic(fmt.Errorf("failure querying stake pools: %w", err))
		}
		fmt.Printf("stake-pools:\n")
		for _, poolID := range stakePools.Results {
			fmt.Printf("  %s\n", poolID.String())
		}
	case "genesis-config":
		genesisConfig, err := o.LocalStateQuery().Client.GetGenesisConfig()
		if err != nil {
			panic(fmt.Errorf("failure querying genesis config: %w", err))
		}
		// Convert struct to JSON-safe representation
		jsonSafe := convertStructToJSONSafe(genesisConfig)
		jsonData, err := json.MarshalIndent(jsonSafe, "", "  ")
		if err != nil {
			panic(fmt.Errorf("failed to marshal genesis config to JSON: %w", err))
		}
		fmt.Printf("genesis-config:\n%s\n", jsonData)
	case "pool-params":
		if len(os.Args) < 3 {
			fmt.Println("ERROR: No pools specified")
			os.Exit(1)
		}

		tmpPools := make([]lcommon.PoolId, 0, len(os.Args[2:]))
		for _, pool := range os.Args[2:] {
			tmpPoolId, err := lcommon.NewPoolIdFromBech32(pool)
			if err != nil {
				fmt.Printf("ERROR: Invalid bech32 pool ID %q: %s\n", pool, err)
				os.Exit(1)
			}
			tmpPools = append(tmpPools, tmpPoolId)
		}
		poolParams, err := o.LocalStateQuery().Client.GetStakePoolParams(tmpPools)
		if err != nil {
			panic(fmt.Errorf("failure querying stake pool params: %w", err))
		}
		// Convert struct to JSON-safe representation
		jsonSafe := convertStructToJSONSafe(poolParams)
		jsonData, err := json.MarshalIndent(jsonSafe, "", "  ")
		if err != nil {
			panic(fmt.Errorf("failed to marshal pool params to JSON: %w", err))
		}
		fmt.Printf("pool-params:\n%s\n", jsonData)
	case "utxos-by-address":
		if len(os.Args) < 3 {
			fmt.Println("ERROR: No addresses specified")
			os.Exit(1)
		}
		tmpAddrs := make([]lcommon.Address, 0, len(os.Args[2:]))
		for _, addr := range os.Args[2:] {
			tmpAddr, err := lcommon.NewAddress(addr)
			if err != nil {
				fmt.Printf("ERROR: Invalid address %q: %s\n", addr, err)
				os.Exit(1)
			}
			tmpAddrs = append(tmpAddrs, tmpAddr)
		}
		utxos, err := o.LocalStateQuery().Client.GetUTxOByAddress(tmpAddrs)
		if err != nil {
			panic(fmt.Errorf("failure querying UTxOs by address: %w", err))
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
		if len(os.Args) < 3 {
			fmt.Println("ERROR: No UTxO IDs specified")
			os.Exit(1)
		}

		tmpTxIns := make([]lcommon.TransactionInput, 0, len(os.Args[2:]))
		for _, txIn := range os.Args[2:] {
			txInParts := strings.SplitN(txIn, `#`, 2)
			if len(txInParts) != 2 {
				fmt.Printf("ERROR: Invalid UTxO ID %q\n", txIn)
				os.Exit(1)
			}
			txIdHex, err := hex.DecodeString(txInParts[0])
			if err != nil {
				fmt.Printf("ERROR: Invalid UTxO ID %q: %s\n", txIn, err)
				os.Exit(1)
			}
			txOutputIdx, err := strconv.ParseUint(txInParts[1], 10, 32)
			if err != nil {
				fmt.Printf("ERROR: Invalid UTxO ID %q: %s\n", txIn, err)
				os.Exit(1)
			}
			tmpTxIn := ledger.ShelleyTransactionInput{
				TxId:        ledger.Blake2b256(txIdHex),
				OutputIndex: uint32(txOutputIdx),
			}
			tmpTxIns = append(tmpTxIns, tmpTxIn)
		}
		utxos, err := o.LocalStateQuery().Client.GetUTxOByTxIn(tmpTxIns)
		if err != nil {
			panic(fmt.Errorf("failure querying UTxOs by TxIn: %w", err))
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
	case "utxo-whole-result":
		limit := -1 // -1 means no limit
		if len(os.Args) >= 3 {
			limitVal, err := strconv.Atoi(os.Args[2])
			if err != nil {
				fmt.Printf("ERROR: Invalid limit value %q: %s\n", os.Args[2], err)
				os.Exit(1)
			}
			limit = limitVal
		}
		fmt.Fprintf(os.Stderr, "WARNING: utxo-whole-result queries the entire UTxO set and may timeout on large networks.\n")
		fmt.Fprintf(os.Stderr, "Consider using 'utxos-by-address' or 'utxos-by-txin' for specific queries instead.\n\n")
		utxos, err := o.LocalStateQuery().Client.GetUTxOWhole()
		if err != nil {
			if strings.Contains(err.Error(), "timeout") {
				fmt.Fprintf(os.Stderr, "\nERROR: Query timed out. The UTxO set is too large to query all at once.\n")
				fmt.Fprintf(os.Stderr, "This is a known limitation - the protocol doesn't support range queries for UTxO whole.\n")
				fmt.Fprintf(os.Stderr, "Please use one of these alternatives instead:\n")
				fmt.Fprintf(os.Stderr, "  - utxos-by-address <address> [address...]  - Query UTxOs for specific addresses\n")
				fmt.Fprintf(os.Stderr, "  - utxos-by-txin <txid#idx> [txid#idx...]  - Query specific UTxOs by transaction input\n")
				os.Exit(1)
			}
			panic(fmt.Errorf("failure querying UTxO whole: %w", err))
		}
		count := 0
		total := len(utxos.Results)
		for utxoId, utxo := range utxos.Results {
			if limit >= 0 && count >= limit {
				break
			}
			fmt.Println("---")
			fmt.Printf("UTxO ID: %s#%d\n", utxoId.Hash.String(), utxoId.Idx)
			fmt.Printf("Address: %x\n", utxo.Address())
			fmt.Printf("Amount: %d\n", utxo.Amount())
			assets := utxo.Assets()
			if assets != nil {
				fmt.Printf("Assets: %+v\n", assets)
			}
			datum := utxo.Datum()
			if datum != nil {
				if cborData := datum.Cbor(); cborData != nil {
					fmt.Printf("Datum CBOR: %x\n", cborData)
				} else {
					fmt.Printf("Datum present (error decoding)\n")
				}
			}
			count++
		}
		if limit >= 0 && count < total {
			fmt.Printf("\n(Showing %d of %d total UTxOs. Use 'utxo-whole-result <limit>' to specify limit)\n", count, total)
		}
	default:
		fmt.Printf("ERROR: unknown query: %s\n", queryType)
		fmt.Println()
		fmt.Println("Available query types:")
		fmt.Println("  current-era")
		fmt.Println("  tip")
		fmt.Println("  system-start")
		fmt.Println("  era-history")
		fmt.Println("  protocol-params")
		fmt.Println("  stake-distribution")
		fmt.Println("  stake-pools")
		fmt.Println("  genesis-config")
		fmt.Println("  pool-params <pool-id> [pool-id...]")
		fmt.Println("  utxos-by-address <address> [address...]")
		fmt.Println("  utxos-by-txin <txid#idx> [txid#idx...]")
		fmt.Println("  utxo-whole-result [limit]  (WARNING: May timeout on large networks)")
		os.Exit(1)
	}
}
