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

import (
	"encoding/json"
	"io"
	"os"
)

// TopologyConfig represents a Cardano node topology config
type TopologyConfig struct {
	Producers          []TopologyConfigLegacyProducer `json:"Producers"`
	LocalRoots         []TopologyConfigP2PLocalRoot   `json:"localRoots"`
	PublicRoots        []TopologyConfigP2PPublicRoot  `json:"publicRoots"`
	UseLedgerAfterSlot uint64                         `json:"useLedgerAfterSlot"`
}

type TopologyConfigLegacyProducer struct {
	Address   string `json:"addr"`
	Port      uint   `json:"port"`
	Valency   uint   `json:"valency"`
	Continent string `json:"continent"`
	State     string `json:"state"`
}

type TopologyConfigP2PAccessPoint struct {
	Address string `json:"address"`
	Port    uint   `json:"port"`
}

type TopologyConfigP2PLocalRoot struct {
	AccessPoints []TopologyConfigP2PAccessPoint `json:"accessPoints"`
	Advertise    bool                           `json:"advertise"`
	Valency      uint                           `json:"valency"`
}

type TopologyConfigP2PPublicRoot struct {
	AccessPoints []TopologyConfigP2PAccessPoint `json:"accessPoints"`
	Advertise    bool                           `json:"advertise"`
	Valency      uint                           `json:"valency"`
}

func NewTopologyConfigFromFile(path string) (*TopologyConfig, error) {
	dataFile, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return NewTopologyConfigFromReader(dataFile)
}

func NewTopologyConfigFromReader(r io.Reader) (*TopologyConfig, error) {
	t := &TopologyConfig{}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, t); err != nil {
		return nil, err
	}
	return t, nil
}
