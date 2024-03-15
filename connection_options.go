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
	"net"

	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

// ConnectionOptionFunc is a type that represents functions that modify the Connection config
type ConnectionOptionFunc func(*Connection)

// WithConnection specifies an existing connection to use. If none is provided, the Dial() function can be
// used to create one later
func WithConnection(conn net.Conn) ConnectionOptionFunc {
	return func(c *Connection) {
		c.conn = conn
	}
}

// WithNetwork specifies the network
func WithNetwork(network Network) ConnectionOptionFunc {
	return func(c *Connection) {
		c.networkMagic = network.NetworkMagic
	}
}

// WithNetworkMagic specifies the network magic value
func WithNetworkMagic(networkMagic uint32) ConnectionOptionFunc {
	return func(c *Connection) {
		c.networkMagic = networkMagic
	}
}

// WithErrorChan specifies the error channel to use. If none is provided, one will be created
func WithErrorChan(errorChan chan error) ConnectionOptionFunc {
	return func(c *Connection) {
		c.errorChan = errorChan
	}
}

// WithServer specifies whether to act as a server
func WithServer(server bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.server = server
	}
}

// WithNodeToNode specifies whether to use the node-to-node protocol. The default is to use node-to-client
func WithNodeToNode(nodeToNode bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.useNodeToNodeProto = nodeToNode
	}
}

// WithKeepAlives specifies whether to use keep-alives. This is disabled by default
func WithKeepAlive(keepAlive bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.sendKeepAlives = keepAlive
	}
}

// WithDelayMuxerStart specifies whether to delay the muxer start. This is useful if you need to take some
// custom actions before the muxer starts processing messages, generally when acting as a server
func WithDelayMuxerStart(delayMuxerStart bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.delayMuxerStart = delayMuxerStart
	}
}

// WithDelayProtocolStart specifies whether to delay the start of the relevant mini-protocols. This is useful
// if you are maintaining lots of connections and want to reduce resource overhead by only starting particular
// protocols
func WithDelayProtocolStart(delayProtocolStart bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.delayProtocolStart = delayProtocolStart
	}
}

// WithFullDuplex specifies whether to enable full-duplex mode when acting as a client
func WithFullDuplex(fullDuplex bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.fullDuplex = fullDuplex
	}
}

// WithPeerSharing specifies whether to enable peer sharing. This affects both the protocol handshake and
// whether the PeerSharing protocol is enabled
func WithPeerSharing(peerSharing bool) ConnectionOptionFunc {
	return func(c *Connection) {
		c.peerSharingEnabled = peerSharing
	}
}

// WithBlockFetchConfig specifies BlockFetch protocol config
func WithBlockFetchConfig(cfg blockfetch.Config) ConnectionOptionFunc {
	return func(c *Connection) {
		c.blockFetchConfig = &cfg
	}
}

// WithChainSyncConfig secifies ChainSync protocol config
func WithChainSyncConfig(cfg chainsync.Config) ConnectionOptionFunc {
	return func(c *Connection) {
		c.chainSyncConfig = &cfg
	}
}

// WithKeepAliveConfig specifies KeepAlive protocol config
func WithKeepAliveConfig(cfg keepalive.Config) ConnectionOptionFunc {
	return func(c *Connection) {
		c.keepAliveConfig = &cfg
	}
}

// WithLocalStateQueryConfig specifies LocalStateQuery protocol config
func WithLocalStateQueryConfig(
	cfg localstatequery.Config,
) ConnectionOptionFunc {
	return func(c *Connection) {
		c.localStateQueryConfig = &cfg
	}
}

// WithLocalTxMonitorConfig specifies LocalTxMonitor protocol config
func WithLocalTxMonitorConfig(
	cfg localtxmonitor.Config,
) ConnectionOptionFunc {
	return func(c *Connection) {
		c.localTxMonitorConfig = &cfg
	}
}

// WithLocalTxSubmissionConfig specifies LocalTxSubmission protocol config
func WithLocalTxSubmissionConfig(
	cfg localtxsubmission.Config,
) ConnectionOptionFunc {
	return func(c *Connection) {
		c.localTxSubmissionConfig = &cfg
	}
}

// WithPeerSharingConfig specifies PeerSharing protocol config
func WithPeerSharingConfig(cfg peersharing.Config) ConnectionOptionFunc {
	return func(c *Connection) {
		c.peerSharingConfig = &cfg
	}
}

// WithTxSubmissionConfig specifies TxSubmission protocol config
func WithTxSubmissionConfig(cfg txsubmission.Config) ConnectionOptionFunc {
	return func(c *Connection) {
		c.txSubmissionConfig = &cfg
	}
}
