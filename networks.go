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

package ouroboros

import "github.com/blinklabs-io/gouroboros/ledger/common"

// Network definitions
var (
	NetworkMainnet = Network{
		Id:           common.AddressNetworkMainnet,
		Name:         "mainnet",
		NetworkMagic: 764824073,
		BootstrapPeers: []NetworkBootstrapPeer{
			{
				Address: "backbone.cardano.iog.io",
				Port:    3001,
			},
			{
				Address: "backbone.mainnet.emurgornd.com",
				Port:    3001,
			},
			{
				Address: "backbone.mainnet.cardanofoundation.org",
				Port:    3001,
			},
		},
	}
	NetworkPreprod = Network{
		Id:           common.AddressNetworkTestnet,
		Name:         "preprod",
		NetworkMagic: 1,
		BootstrapPeers: []NetworkBootstrapPeer{
			{
				Address: "preprod-node.play.dev.cardano.org",
				Port:    3001,
			},
		},
	}
	NetworkPreview = Network{
		Id:           common.AddressNetworkTestnet,
		Name:         "preview",
		NetworkMagic: 2,
		BootstrapPeers: []NetworkBootstrapPeer{
			{
				Address: "preview-node.play.dev.cardano.org",
				Port:    3001,
			},
		},
	}
	NetworkSancho = Network{
		Id:           common.AddressNetworkTestnet,
		Name:         "sanchonet",
		NetworkMagic: 4,
		BootstrapPeers: []NetworkBootstrapPeer{
			{
				Address: "sanchonet-node.play.dev.cardano.org",
				Port:    3001,
			},
		},
	}
	NetworkDevnet = Network{
		Id:           common.AddressNetworkTestnet,
		Name:         "devnet",
		NetworkMagic: 42,
	}
)

// List of valid networks for use in lookup functions
var networks = []Network{
	NetworkMainnet,
	NetworkPreprod,
	NetworkPreview,
	NetworkSancho,
	NetworkDevnet,
}

// NetworkByName returns a predefined network by name
func NetworkByName(name string) (Network, bool) {
	for _, network := range networks {
		if network.Name == name {
			return network, true
		}
	}
	return Network{}, false
}

// NetworkById returns a predefined network by ID
func NetworkById(id uint8) (Network, bool) {
	for _, network := range networks {
		if network.Id == id {
			return network, true
		}
	}
	return Network{}, false
}

// NetworkByNetworkMagic returns a predefined network by network magic
func NetworkByNetworkMagic(networkMagic uint32) (Network, bool) {
	for _, network := range networks {
		if network.NetworkMagic == networkMagic {
			return network, true
		}
	}
	return Network{}, false
}

// Network represents a Cardano network
type Network struct {
	Id             uint8 // network ID used for addresses
	Name           string
	NetworkMagic   uint32
	BootstrapPeers []NetworkBootstrapPeer
}

type NetworkBootstrapPeer struct {
	Address string
	Port    uint
}

func (n Network) String() string {
	return n.Name
}
