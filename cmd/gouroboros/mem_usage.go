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
	"log"
	"net/http"
	// #nosec G108
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
)

type memUsageFlags struct {
	flagset   *flag.FlagSet
	startEra  string
	tip       bool
	debugPort int
}

func newMemUsageFlags() *memUsageFlags {
	f := &memUsageFlags{
		flagset: flag.NewFlagSet("mem-usage", flag.ExitOnError),
	}
	f.flagset.StringVar(
		&f.startEra,
		"start-era",
		"genesis",
		"era which to start chain-sync at",
	)
	f.flagset.BoolVar(
		&f.tip,
		"tip",
		false,
		"start chain-sync at current chain tip",
	)
	f.flagset.IntVar(&f.debugPort, "debug-port", 8080, "pprof port")
	return f
}

func testMemUsage(f *globalFlags) {
	memUsageFlags := newMemUsageFlags()
	err := memUsageFlags.flagset.Parse(f.flagset.Args()[1:])
	if err != nil {
		fmt.Printf("failed to parse subcommand args: %s\n", err)
		os.Exit(1)
	}

	// Start pprof listener
	log.Printf(
		"Starting pprof listener on http://0.0.0.0:%d/debug/pprof\n",
		memUsageFlags.debugPort,
	)
	go func() {
		log.Println(
			// This is a test program, no timeouts
			// #nosec G114
			http.ListenAndServe(
				fmt.Sprintf(":%d", memUsageFlags.debugPort),
				nil,
			),
		)
	}()

	for i := 0; i < 10; i++ {
		showMemoryStats("open")

		conn := createClientConnection(f)
		errorChan := make(chan error)
		go func() {
			for {
				err, ok := <-errorChan
				if !ok {
					return
				}
				fmt.Printf("ERROR: %s\n", err)
				os.Exit(1)
			}
		}()
		o, err := ouroboros.New(
			ouroboros.WithConnection(conn),
			ouroboros.WithNetworkMagic(uint32(f.networkMagic)), // #nosec G115
			ouroboros.WithErrorChan(errorChan),
			ouroboros.WithNodeToNode(f.ntnProto),
			ouroboros.WithKeepAlive(true),
		)
		if err != nil {
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}

		tip, err := o.ChainSync().Client.GetCurrentTip()
		if err != nil {
			fmt.Printf("ERROR: %s\n", err)
			os.Exit(1)
		}

		log.Printf(
			"tip: slot = %d, hash = %x\n",
			tip.Point.Slot,
			tip.Point.Hash,
		)

		if err := o.Close(); err != nil {
			fmt.Printf("ERROR: %s\n", err)
		}

		showMemoryStats("close")

		time.Sleep(5 * time.Second)

		runtime.GC()

		showMemoryStats("after GC")
	}

	if err := pprof.Lookup("goroutine").WriteTo(os.Stdout, 1); err != nil {
		fmt.Printf("ERROR: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("waiting forever")
	select {}
}

func showMemoryStats(tag string) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("[%s] HeapAlloc: %dKiB\n", tag, m.HeapAlloc/1024)
}
