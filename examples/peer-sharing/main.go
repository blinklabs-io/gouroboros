// Copyright 2025 Blink Labs Software
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
	"math"
	"os"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/kelseyhightower/envconfig"
)

// We parse environment variables using envconfig into this struct
type Config struct {
	Address      string
	Network      string
	NetworkMagic uint32 `split_words:"true"`
	Peers        uint
}

// This code will be executed when run
func main() {
	// Set config defaults
	cfg := Config{
		Address:      "backbone.cardano.iog.io:3001",
		Network:      "mainnet",
		NetworkMagic: 0,
		Peers:        10,
	}
	// Parse environment variables
	if err := envconfig.Process("peer_sharing", &cfg); err != nil {
		panic(err)
	}
	// Error check peers
	if cfg.Peers > math.MaxUint8 {
		panic(
			fmt.Sprintf(
				"peers not within range: given: %d, max: %d",
				cfg.Peers,
				math.MaxUint8,
			),
		)
	}
	// Configure NetworkMagic
	if cfg.NetworkMagic == 0 {
		network, ok := ouroboros.NetworkByName(cfg.Network)
		if !ok {
			fmt.Printf("invalid network specified: %v\n", cfg.Network)
			os.Exit(1)
		}
		cfg.NetworkMagic = network.NetworkMagic
	}
	// Create error channel
	errorChan := make(chan error)
	// start error handler
	go func() {
		for {
			err := <-errorChan
			panic(err)
		}
	}()
	// Configure Ouroboros
	o, err := ouroboros.NewConnection(
		ouroboros.WithNetworkMagic(cfg.NetworkMagic),
		ouroboros.WithErrorChan(errorChan),
		ouroboros.WithNodeToNode(true),
		ouroboros.WithKeepAlive(true),
		ouroboros.WithPeerSharing(true),
		ouroboros.WithFullDuplex(true),
	)
	if err != nil {
		panic(err)
	}
	// Connect to Node address
	if err = o.Dial("tcp", cfg.Address); err != nil {
		panic(err)
	}
	// Get requested number of peers from Node via NtN PeerSharing
	peers, err := o.PeerSharing().Client.GetPeers(uint8(cfg.Peers))
	if err != nil {
		panic(err)
	}

	fmt.Println("Peers:")
	fmt.Println()
	for _, peer := range peers {
		fmt.Printf("%s:%d\n", peer.IP.String(), peer.Port)
	}
}
