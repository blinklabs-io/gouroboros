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

// Package ouroboros implements support for interacting with Cardano nodes using
// the Ouroboros network protocol.
//
// The Ouroboros network protocol consists of a muxer and multiple mini-protocols
// that provide various functions. A handshake and protocol versioning are used to
// ensure peer compatibility.
//
// This package is the main entry point into this library. The other packages can
// be used outside of this one, but it's not a primary design goal.
package ouroboros

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
	"github.com/blinklabs-io/gouroboros/protocol/localtxmonitor"
	"github.com/blinklabs-io/gouroboros/protocol/localtxsubmission"
	"github.com/blinklabs-io/gouroboros/protocol/peersharing"
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

const (
	// Default connection timeout
	DefaultConnectTimeout = 30 * time.Second
)

type ConnectionId = connection.ConnectionId

// The Connection type is a wrapper around a net.Conn object that handles communication using the Ouroboros network protocol over that connection
type Connection struct {
	id                    ConnectionId
	conn                  net.Conn
	networkMagic          uint32
	server                bool
	useNodeToNodeProto    bool
	logger                *slog.Logger
	muxer                 *muxer.Muxer
	errorChan             chan error
	protoErrorChan        chan error
	handshakeFinishedChan chan any
	handshakeVersion      uint16
	handshakeVersionData  protocol.VersionData
	doneChan              chan any
	connClosedChan        chan struct{}
	waitGroup             sync.WaitGroup
	onceClose             sync.Once
	sendKeepAlives        bool
	delayMuxerStart       bool
	delayProtocolStart    bool
	fullDuplex            bool
	peerSharingEnabled    bool
	// Mini-protocols
	blockFetch              *blockfetch.BlockFetch
	blockFetchConfig        *blockfetch.Config
	chainSync               *chainsync.ChainSync
	chainSyncConfig         *chainsync.Config
	handshake               *handshake.Handshake
	keepAlive               *keepalive.KeepAlive
	keepAliveConfig         *keepalive.Config
	localStateQuery         *localstatequery.LocalStateQuery
	localStateQueryConfig   *localstatequery.Config
	localTxMonitor          *localtxmonitor.LocalTxMonitor
	localTxMonitorConfig    *localtxmonitor.Config
	localTxSubmission       *localtxsubmission.LocalTxSubmission
	localTxSubmissionConfig *localtxsubmission.Config
	peerSharing             *peersharing.PeerSharing
	peerSharingConfig       *peersharing.Config
	txSubmission            *txsubmission.TxSubmission
	txSubmissionConfig      *txsubmission.Config
}

// NewConnection returns a new Connection object with the specified options. If a connection is provided, the
// handshake will be started. An error will be returned if the handshake fails
func NewConnection(options ...ConnectionOptionFunc) (*Connection, error) {
	c := &Connection{
		protoErrorChan:        make(chan error, 10),
		handshakeFinishedChan: make(chan any),
		connClosedChan:        make(chan struct{}),
		// Create a discard logger to throw away logs. We do this so
		// we don't have to add guards around every log operation if
		// a logger is not configured by the user.
		logger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
	}
	// Apply provided options functions
	for _, option := range options {
		option(c)
	}
	if c.errorChan == nil {
		c.errorChan = make(chan error, 10)
	}
	if c.conn != nil {
		if err := c.setupConnection(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// New is an alias to NewConnection for backward compatibility
func New(options ...ConnectionOptionFunc) (*Connection, error) {
	return NewConnection(options...)
}

// Id returns the connection ID
func (c *Connection) Id() ConnectionId {
	return c.id
}

// Muxer returns the muxer object for the Ouroboros connection
func (c *Connection) Muxer() *muxer.Muxer {
	return c.muxer
}

// ErrorChan returns the channel for asynchronous errors
func (c *Connection) ErrorChan() chan error {
	return c.errorChan
}

// Dial will establish a connection using the specified protocol and address. It works the same as [DialTimeout],
// except that it provides a default connect timeout
func (c *Connection) Dial(proto string, address string) error {
	return c.DialTimeout(proto, address, DefaultConnectTimeout)
}

// DialTimeout will establish a connection using the specified protocol, address, and timeout. These parameters are
// passed to the [net.DialTimeout] func. The handshake will be started when a connection is established.
// An error will be returned if the connection fails, a connection was already established, or the
// handshake fails
func (c *Connection) DialTimeout(
	proto string,
	address string,
	timeout time.Duration,
) error {
	if c.conn != nil {
		return errors.New("a connection was already established")
	}
	conn, err := net.DialTimeout(proto, address, timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	if err := c.setupConnection(); err != nil {
		return err
	}
	return nil
}

// Close will shutdown the Ouroboros connection
func (c *Connection) Close() error {
	c.onceClose.Do(func() {
		if c.doneChan == nil {
			return
		}
		// Close doneChan to signify that we're shutting down
		close(c.doneChan)
		// Wait for connection to be closed
		<-c.connClosedChan
	})
	return nil
}

// BlockFetch returns the block-fetch protocol handler
func (c *Connection) BlockFetch() *blockfetch.BlockFetch {
	return c.blockFetch
}

// ChainSync returns the chain-sync protocol handler
func (c *Connection) ChainSync() *chainsync.ChainSync {
	return c.chainSync
}

// Handshake returns the handshake protocol handler
func (c *Connection) Handshake() *handshake.Handshake {
	return c.handshake
}

// KeepAlive returns the keep-alive protocol handler
func (c *Connection) KeepAlive() *keepalive.KeepAlive {
	return c.keepAlive
}

// LocalStateQuery returns the local-state-query protocol handler
func (c *Connection) LocalStateQuery() *localstatequery.LocalStateQuery {
	return c.localStateQuery
}

// LocalTxMonitor returns the local-tx-monitor protocol handler
func (c *Connection) LocalTxMonitor() *localtxmonitor.LocalTxMonitor {
	return c.localTxMonitor
}

// LocalTxSubmission returns the local-tx-submission protocol handler
func (c *Connection) LocalTxSubmission() *localtxsubmission.LocalTxSubmission {
	return c.localTxSubmission
}

// PeerSharing returns the peer-sharing protocol handler
func (c *Connection) PeerSharing() *peersharing.PeerSharing {
	return c.peerSharing
}

// TxSubmission returns the tx-submission protocol handler
func (c *Connection) TxSubmission() *txsubmission.TxSubmission {
	return c.txSubmission
}

// ProtocolVersion returns the negotiated protocol version and the version data from the remote peer
func (c *Connection) ProtocolVersion() (uint16, protocol.VersionData) {
	return c.handshakeVersion, c.handshakeVersionData
}

// shutdown performs cleanup operations when the connection is shutdown, either due to explicit Close() or an error
func (c *Connection) shutdown() {
	// Gracefully stop the muxer
	if c.muxer != nil {
		c.muxer.Stop()
	}
	// Close channel to let Close() know that it can return
	close(c.connClosedChan)
	// Wait for other goroutines to finish
	c.waitGroup.Wait()
	// Close consumer error channel to signify connection shutdown
	close(c.errorChan)
}

// setupConnection establishes the muxer, configures and starts the handshake process, and initializes
// the appropriate mini-protocols
func (c *Connection) setupConnection() error {
	// Check network magic value
	if c.networkMagic == 0 {
		return fmt.Errorf(
			"invalid network magic value provided: %d",
			c.networkMagic,
		)
	}
	// Start Goroutine to shutdown when doneChan is closed
	c.doneChan = make(chan any)
	go func() {
		<-c.doneChan
		c.shutdown()
	}()
	// Populate connection ID
	c.id = ConnectionId{
		LocalAddr:  c.conn.LocalAddr(),
		RemoteAddr: c.conn.RemoteAddr(),
	}
	// Create muxer instance
	c.muxer = muxer.New(c.conn)
	// Start Goroutine to pass along errors from the muxer
	c.waitGroup.Add(1)
	go func() {
		defer c.waitGroup.Done()
		select {
		case <-c.doneChan:
			return
		case err, ok := <-c.muxer.ErrorChan():
			// Break out of goroutine if muxer's error channel is closed
			if !ok {
				return
			}
			var connErr *muxer.ConnectionClosedError
			if errors.As(err, &connErr) {
				// Pass through ConnectionClosedError from muxer
				c.errorChan <- err
			} else {
				// Wrap error message to denote it comes from the muxer
				c.errorChan <- fmt.Errorf("muxer error: %w", err)
			}
			// Close connection on muxer errors
			c.Close()
		}
	}()
	protoOptions := protocol.ProtocolOptions{
		ConnectionId: c.id,
		Muxer:        c.muxer,
		Logger:       c.logger,
		ErrorChan:    c.protoErrorChan,
	}
	if c.useNodeToNodeProto {
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
	} else {
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
	}
	if c.server {
		protoOptions.Role = protocol.ProtocolRoleServer
	} else {
		protoOptions.Role = protocol.ProtocolRoleClient
	}
	// Generate protocol version map for handshake
	handshakeDiffusionMode := protocol.DiffusionModeInitiatorOnly
	if c.fullDuplex {
		handshakeDiffusionMode = protocol.DiffusionModeInitiatorAndResponder
	}
	protoVersions := protocol.GetProtocolVersionMap(
		protoOptions.Mode,
		c.networkMagic,
		handshakeDiffusionMode,
		c.peerSharingEnabled,
		// TODO: make this configurable (#373)
		protocol.QueryModeDisabled,
	)
	// Perform handshake
	var handshakeFullDuplex bool
	handshakeConfig := handshake.NewConfig(
		handshake.WithProtocolVersionMap(protoVersions),
		handshake.WithFinishedFunc(
			func(ctx handshake.CallbackContext, version uint16, versionData protocol.VersionData) error {
				c.handshakeVersion = version
				c.handshakeVersionData = versionData
				if c.useNodeToNodeProto {
					if versionData.DiffusionMode() == protocol.DiffusionModeInitiatorAndResponder {
						handshakeFullDuplex = true
					}
				}
				close(c.handshakeFinishedChan)
				return nil
			},
		),
	)
	c.handshake = handshake.New(protoOptions, &handshakeConfig)
	if c.server {
		c.handshake.Server.Start()
	} else {
		c.handshake.Client.Start()
	}
	c.muxer.StartOnce()
	// Wait for handshake completion or error
	select {
	case <-c.doneChan:
		// Return an error if we're shutting down
		return fmt.Errorf("connection shutdown initiated: %w", io.EOF)
	case err := <-c.protoErrorChan:
		// Shutdown the connection and return the error
		c.Close()
		return err
	case <-c.handshakeFinishedChan:
		// This is purposely empty, but we need this case to break out when this channel is closed
	}
	// Provide the negotiated protocol version to the various mini-protocols
	protoOptions.Version = c.handshakeVersion
	// Start Goroutine to pass along errors from the mini-protocols
	c.waitGroup.Add(1)
	go func() {
		defer c.waitGroup.Done()
		select {
		case <-c.doneChan:
			// Return if we're shutting down
			return
		case err, ok := <-c.protoErrorChan:
			// The channel is closed, which means we're already shutting down
			if !ok {
				return
			}
			c.errorChan <- fmt.Errorf("protocol error: %w", err)
			// Close connection on mini-protocol errors
			c.Close()
		}
	}()
	// Configure the relevant mini-protocols
	if c.useNodeToNodeProto {
		versionNtN := protocol.GetProtocolVersion(c.handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
		c.chainSync = chainsync.New(protoOptions, c.chainSyncConfig)
		c.blockFetch = blockfetch.New(protoOptions, c.blockFetchConfig)
		c.txSubmission = txsubmission.New(protoOptions, c.txSubmissionConfig)
		if versionNtN.EnableKeepAliveProtocol {
			c.keepAlive = keepalive.New(protoOptions, c.keepAliveConfig)
		}
		if versionNtN.EnablePeerSharingProtocol {
			c.peerSharing = peersharing.New(protoOptions, c.peerSharingConfig)
		}
		// Start protocols
		if !c.delayProtocolStart {
			if (c.fullDuplex && handshakeFullDuplex) || !c.server {
				c.blockFetch.Client.Start()
				c.chainSync.Client.Start()
				c.txSubmission.Client.Start()
				if c.keepAlive != nil && c.sendKeepAlives {
					c.keepAlive.Client.Start()
				}
				if c.peerSharing != nil {
					c.peerSharing.Client.Start()
				}
			}
			if (c.fullDuplex && handshakeFullDuplex) || c.server {
				c.blockFetch.Server.Start()
				c.chainSync.Server.Start()
				c.txSubmission.Server.Start()
				if c.keepAlive != nil {
					c.keepAlive.Server.Start()
				}
				if c.peerSharing != nil {
					c.peerSharing.Server.Start()
				}
			}
		}
	} else {
		versionNtC := protocol.GetProtocolVersion(c.handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
		c.chainSync = chainsync.New(protoOptions, c.chainSyncConfig)
		c.localTxSubmission = localtxsubmission.New(protoOptions, c.localTxSubmissionConfig)
		if versionNtC.EnableLocalQueryProtocol {
			c.localStateQuery = localstatequery.New(protoOptions, c.localStateQueryConfig)
		}
		if versionNtC.EnableLocalTxMonitorProtocol {
			c.localTxMonitor = localtxmonitor.New(protoOptions, c.localTxMonitorConfig)
		}
		// Start protocols
		if !c.delayProtocolStart {
			if (c.fullDuplex && handshakeFullDuplex) || !c.server {
				c.chainSync.Client.Start()
				c.localTxSubmission.Client.Start()
				if c.localStateQuery != nil {
					c.localStateQuery.Client.Start()
				}
				if c.localTxMonitor != nil {
					c.localTxMonitor.Client.Start()
				}
			}
			if (c.fullDuplex && handshakeFullDuplex) || c.server {
				c.chainSync.Server.Start()
				c.localTxSubmission.Server.Start()
				if c.localStateQuery != nil {
					c.localStateQuery.Server.Start()
				}
				if c.localTxMonitor != nil {
					c.localTxMonitor.Server.Start()
				}
			}
		}
	}
	// Start muxer
	diffusionMode := muxer.DiffusionModeInitiator
	if handshakeFullDuplex {
		diffusionMode = muxer.DiffusionModeInitiatorAndResponder
	} else if c.server {
		diffusionMode = muxer.DiffusionModeResponder
	}
	c.muxer.SetDiffusionMode(diffusionMode)
	if !c.delayMuxerStart {
		c.muxer.Start()
	}
	return nil
}
