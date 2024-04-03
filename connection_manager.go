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

import "sync"

// ConnectionManagerConnClosedFunc is a function that takes a connection ID and an optional error
type ConnectionManagerConnClosedFunc func(ConnectionId, error)

// ConnectionManagerTag represents the various tags that can be associated with a host or connection
type ConnectionManagerTag uint16

const (
	ConnectionManagerTagNone ConnectionManagerTag = iota

	ConnectionManagerTagHostProducer
	ConnectionManagerTagHostLocalRoot
	ConnectionManagerTagHostPublicRoot
	ConnectionManagerTagHostP2PLedger
	ConnectionManagerTagHostP2PGossip

	ConnectionManagerTagRoleInitiator
	ConnectionManagerTagRoleResponder
	// TODO: add more tags
)

func (c ConnectionManagerTag) String() string {
	tmp := map[ConnectionManagerTag]string{
		ConnectionManagerTagHostProducer:   "HostProducer",
		ConnectionManagerTagHostLocalRoot:  "HostLocalRoot",
		ConnectionManagerTagHostPublicRoot: "HostPublicRoot",
		ConnectionManagerTagHostP2PLedger:  "HostP2PLedger",
		ConnectionManagerTagHostP2PGossip:  "HostP2PGossip",
		ConnectionManagerTagRoleInitiator:  "RoleInitiator",
		ConnectionManagerTagRoleResponder:  "RoleResponder",
		// TODO: add more tags to match those added above
	}
	ret, ok := tmp[c]
	if !ok {
		return "Unknown"
	}
	return ret
}

type ConnectionManager struct {
	config           ConnectionManagerConfig
	hosts            []ConnectionManagerHost
	connections      map[ConnectionId]*ConnectionManagerConnection
	connectionsMutex sync.Mutex
}

type ConnectionManagerConfig struct {
	ConnClosedFunc ConnectionManagerConnClosedFunc
}

type ConnectionManagerHost struct {
	Address string
	Port    uint
	Tags    map[ConnectionManagerTag]bool
}

func NewConnectionManager(cfg ConnectionManagerConfig) *ConnectionManager {
	return &ConnectionManager{
		config:      cfg,
		connections: make(map[ConnectionId]*ConnectionManagerConnection),
	}
}

func (c *ConnectionManager) AddHost(address string, port uint, tags ...ConnectionManagerTag) {
	tmpTags := map[ConnectionManagerTag]bool{}
	for _, tag := range tags {
		tmpTags[tag] = true
	}
	c.hosts = append(
		c.hosts,
		ConnectionManagerHost{
			Address: address,
			Port:    port,
			Tags:    tmpTags,
		},
	)
}

func (c *ConnectionManager) AddHostsFromTopology(topology *TopologyConfig) {
	for _, host := range topology.Producers {
		c.AddHost(host.Address, host.Port, ConnectionManagerTagHostProducer)
	}
	for _, localRoot := range topology.LocalRoots {
		for _, host := range localRoot.AccessPoints {
			c.AddHost(host.Address, host.Port, ConnectionManagerTagHostLocalRoot)
		}
	}
	for _, publicRoot := range topology.PublicRoots {
		for _, host := range publicRoot.AccessPoints {
			c.AddHost(host.Address, host.Port, ConnectionManagerTagHostPublicRoot)
		}
	}
}

func (c *ConnectionManager) AddConnection(conn *Connection) {
	connId := conn.Id()
	c.connectionsMutex.Lock()
	c.connections[connId] = &ConnectionManagerConnection{
		Conn: conn,
	}
	c.connectionsMutex.Unlock()
	go func() {
		err := <-conn.ErrorChan()
		// Call configured connection closed callback func
		c.config.ConnClosedFunc(connId, err)
	}()
}

func (c *ConnectionManager) RemoveConnection(connId ConnectionId) {
	c.connectionsMutex.Lock()
	delete(c.connections, connId)
	c.connectionsMutex.Unlock()
}

func (c *ConnectionManager) GetConnectionById(connId ConnectionId) *ConnectionManagerConnection {
	c.connectionsMutex.Lock()
	defer c.connectionsMutex.Unlock()
	return c.connections[connId]
}

func (c *ConnectionManager) GetConnectionsByTags(tags ...ConnectionManagerTag) []*ConnectionManagerConnection {
	var ret []*ConnectionManagerConnection
	c.connectionsMutex.Lock()
	for _, conn := range c.connections {
		skipConn := false
		for _, tag := range tags {
			if _, ok := conn.Tags[tag]; !ok {
				skipConn = true
				break
			}
		}
		if !skipConn {
			ret = append(ret, conn)
		}
	}
	c.connectionsMutex.Unlock()
	return ret
}

type ConnectionManagerConnection struct {
	Conn *Connection
	Tags map[ConnectionManagerTag]bool
}

func (c *ConnectionManagerConnection) AddTags(tags ...ConnectionManagerTag) {
	for _, tag := range tags {
		c.Tags[tag] = true
	}
}

func (c *ConnectionManagerConnection) RemoveTags(tags ...ConnectionManagerTag) {
	for _, tag := range tags {
		delete(c.Tags, tag)
	}
}
