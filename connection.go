// Copyright 2023 Blink Labs, LLC.
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
	"net"
	"sync"

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

// The Connection type is a wrapper around a net.Conn object that handles communication using the Ouroboros network protocol over that connection
type Connection struct {
	conn                  net.Conn
	networkMagic          uint32
	server                bool
	useNodeToNodeProto    bool
	muxer                 *muxer.Muxer
	errorChan             chan error
	protoErrorChan        chan error
	handshakeFinishedChan chan interface{}
	doneChan              chan interface{}
	waitGroup             sync.WaitGroup
	onceClose             sync.Once
	sendKeepAlives        bool
	delayMuxerStart       bool
	fullDuplex            bool
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
		handshakeFinishedChan: make(chan interface{}),
		doneChan:              make(chan interface{}),
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

// Muxer returns the muxer object for the Ouroboros connection
func (c *Connection) Muxer() *muxer.Muxer {
	return c.muxer
}

// ErrorChan returns the channel for asynchronous errors
func (c *Connection) ErrorChan() chan error {
	return c.errorChan
}

// Dial will establish a connection using the specified protocol and address. These parameters are
// passed to the [net.Dial] func. The handshake will be started when a connection is established.
// An error will be returned if the connection fails, a connection was already established, or the
// handshake fails
func (c *Connection) Dial(proto string, address string) error {
	if c.conn != nil {
		return fmt.Errorf("a connection was already established")
	}
	conn, err := net.Dial(proto, address)
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
	var err error
	c.onceClose.Do(func() {
		// Close doneChan to signify that we're shutting down
		close(c.doneChan)
		// Gracefully stop the muxer
		if c.muxer != nil {
			c.muxer.Stop()
		}
		// Wait for other goroutines to finish
		c.waitGroup.Wait()
		// Close channels
		close(c.errorChan)
		close(c.protoErrorChan)
		// We can only close a channel once, so we have to jump through a few hoops
		select {
		// The channel is either closed or has an item pending
		case _, ok := <-c.handshakeFinishedChan:
			// We successfully retrieved an item
			// This will probably never happen, but it doesn't hurt to cover this case
			if ok {
				close(c.handshakeFinishedChan)
			}
		// The channel is open and has no pending items
		default:
			close(c.handshakeFinishedChan)
		}
	})
	return err
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

// setupConnection establishes the muxer, configures and starts the handshake process, and initializes
// the appropriate mini-protocols
func (c *Connection) setupConnection() error {
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
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				// Return a bare io.EOF error if error is EOF/ErrUnexpectedEOF
				c.errorChan <- io.EOF
			} else {
				// Wrap error message to denote it comes from the muxer
				c.errorChan <- fmt.Errorf("muxer error: %s", err)
			}
			// Close connection on muxer errors
			c.Close()
		}
	}()
	protoOptions := protocol.ProtocolOptions{
		Muxer:     c.muxer,
		ErrorChan: c.protoErrorChan,
	}
	var protoVersions []uint16
	if c.useNodeToNodeProto {
		protoVersions = GetProtocolVersionsNtN()
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
	} else {
		protoVersions = GetProtocolVersionsNtC()
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
	}
	if c.server {
		protoOptions.Role = protocol.ProtocolRoleServer
	} else {
		protoOptions.Role = protocol.ProtocolRoleClient
	}
	// Check network magic value
	if c.networkMagic == 0 {
		return fmt.Errorf("invalid network magic value provided: %d\n", c.networkMagic)
	}
	// Perform handshake
	var handshakeVersion uint16
	var handshakeFullDuplex bool
	handshakeConfig := handshake.NewConfig(
		handshake.WithProtocolVersions(protoVersions),
		handshake.WithNetworkMagic(c.networkMagic),
		handshake.WithClientFullDuplex(c.fullDuplex),
		handshake.WithFinishedFunc(func(version uint16, fullDuplex bool) error {
			handshakeVersion = version
			handshakeFullDuplex = fullDuplex
			close(c.handshakeFinishedChan)
			return nil
		}),
	)
	c.handshake = handshake.New(protoOptions, &handshakeConfig)
	if c.server {
		c.handshake.Server.Start()
	} else {
		c.handshake.Client.Start()
	}
	// Wait for handshake completion or error
	select {
	case <-c.doneChan:
		// Return an error if we're shutting down
		return io.EOF
	case err := <-c.protoErrorChan:
		return err
	case <-c.handshakeFinishedChan:
		// This is purposely empty, but we need this case to break out when this channel is closed
	}
	// Provide the negotiated protocol version to the various mini-protocols
	protoOptions.Version = handshakeVersion
	// Drop bit used to signify NtC protocol versions
	if protoOptions.Version > protocolVersionNtCFlag {
		protoOptions.Version = protoOptions.Version - protocolVersionNtCFlag
	}
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
			c.errorChan <- fmt.Errorf("protocol error: %s", err)
			// Close connection on mini-protocol errors
			c.Close()
		}
	}()
	// Configure the relevant mini-protocols
	if c.useNodeToNodeProto {
		versionNtN := GetProtocolVersionNtN(handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
		c.chainSync = chainsync.New(protoOptions, c.chainSyncConfig)
		c.blockFetch = blockfetch.New(protoOptions, c.blockFetchConfig)
		c.txSubmission = txsubmission.New(protoOptions, c.txSubmissionConfig)
		if versionNtN.EnableKeepAliveProtocol {
			c.keepAlive = keepalive.New(protoOptions, c.keepAliveConfig)
			if !c.server && c.sendKeepAlives {
				c.keepAlive.Client.Start()
			}
		}
		if versionNtN.EnablePeerSharingProtocol {
			c.peerSharing = peersharing.New(protoOptions, c.peerSharingConfig)
		}
	} else {
		versionNtC := GetProtocolVersionNtC(handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
		c.chainSync = chainsync.New(protoOptions, c.chainSyncConfig)
		c.localTxSubmission = localtxsubmission.New(protoOptions, c.localTxSubmissionConfig)
		if versionNtC.EnableLocalQueryProtocol {
			c.localStateQuery = localstatequery.New(protoOptions, c.localStateQueryConfig)
		}
		if versionNtC.EnableLocalTxMonitorProtocol {
			c.localTxMonitor = localtxmonitor.New(protoOptions, c.localTxMonitorConfig)
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
