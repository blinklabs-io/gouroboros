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

package ouroboros

import "github.com/blinklabs-io/gouroboros/ledger/common"

// Network definitions
var (
	NetworkTestnet = Network{
		Id:           common.AddressNetworkTestnet,
		Name:         "testnet",
		NetworkMagic: 1097911063,
	}
	NetworkMainnet = Network{
		Id:                common.AddressNetworkMainnet,
		Name:              "mainnet",
		NetworkMagic:      764824073,
		PublicRootAddress: "backbone.cardano-mainnet.iohk.io",
		PublicRootPort:    3001,
	}
	NetworkPreprod = Network{
		Id:                common.AddressNetworkTestnet,
		Name:              "preprod",
		NetworkMagic:      1,
		PublicRootAddress: "preprod-node.world.dev.cardano.org",
		PublicRootPort:    30000,
	}
	NetworkPreview = Network{
		Id:                common.AddressNetworkTestnet,
		Name:              "preview",
		NetworkMagic:      2,
		PublicRootAddress: "preview-node.play.dev.cardano.org",
		PublicRootPort:    3001,
	}
	NetworkSancho = Network{
		Id:                common.AddressNetworkTestnet,
		Name:              "sanchonet",
		NetworkMagic:      4,
		PublicRootAddress: "sanchonet-node.play.dev.cardano.org",
		PublicRootPort:    3001,
	}

	NetworkInvalid = Network{
		Id:           0,
		Name:         "invalid",
		NetworkMagic: 0,
	} // NetworkInvalid is used as a return value for lookup functions when a network isn't found
)

// List of valid networks for use in lookup functions
var networks = []Network{
	NetworkTestnet,
	NetworkMainnet,
	NetworkPreprod,
	NetworkPreview,
	NetworkSancho,
}

// NetworkByName returns a predefined network by name
func NetworkByName(name string) Network {
	for _, network := range networks {
		if network.Name == name {
			return network
		}
	}
	return NetworkInvalid
}

// NetworkById returns a predefined network by ID
func NetworkById(id uint8) Network {
	for _, network := range networks {
		if network.Id == id {
			return network
		}
	}
	return NetworkInvalid
}

// NetworkByNetworkMagic returns a predefined network by network magic
func NetworkByNetworkMagic(networkMagic uint32) Network {
	for _, network := range networks {
		if network.NetworkMagic == networkMagic {
			return network
		}
	}
	return NetworkInvalid
}

// Network represents a Cardano network
type Network struct {
	Id                uint8 // network ID used for addresses
	Name              string
	NetworkMagic      uint32
	PublicRootAddress string
	PublicRootPort    uint
}

func (n Network) String() string {
	return n.Name
}
