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
	"github.com/blinklabs-io/gouroboros/protocol/txsubmission"
)

// The Ouroboros type is a wrapper around a net.Conn object that handles communication using the Ouroboros network protocol over that connection
type Ouroboros struct {
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
	closeMutex            sync.Mutex
	sendKeepAlives        bool
	delayMuxerStart       bool
	fullDuplex            bool
	// Mini-protocols
	handshake               *handshake.Handshake
	chainSync               *chainsync.ChainSync
	chainSyncConfig         *chainsync.Config
	blockFetch              *blockfetch.BlockFetch
	blockFetchConfig        *blockfetch.Config
	keepAlive               *keepalive.KeepAlive
	keepAliveConfig         *keepalive.Config
	localTxMonitor          *localtxmonitor.LocalTxMonitor
	localTxMonitorConfig    *localtxmonitor.Config
	localTxSubmission       *localtxsubmission.LocalTxSubmission
	localTxSubmissionConfig *localtxsubmission.Config
	localStateQuery         *localstatequery.LocalStateQuery
	localStateQueryConfig   *localstatequery.Config
	txSubmission            *txsubmission.TxSubmission
	txSubmissionConfig      *txsubmission.Config
}

// New returns a new Ouroboros object with the specified options. If a connection is provided, the
// handshake will be started. An error will be returned if the handshake fails
func New(options ...OuroborosOptionFunc) (*Ouroboros, error) {
	o := &Ouroboros{
		protoErrorChan:        make(chan error, 10),
		handshakeFinishedChan: make(chan interface{}),
		doneChan:              make(chan interface{}),
	}
	// Apply provided options functions
	for _, option := range options {
		option(o)
	}
	if o.errorChan == nil {
		o.errorChan = make(chan error, 10)
	}
	if o.conn != nil {
		if err := o.setupConnection(); err != nil {
			return nil, err
		}
	}
	return o, nil
}

// Muxer returns the muxer object for the Ouroboros connection
func (o *Ouroboros) Muxer() *muxer.Muxer {
	return o.muxer
}

// ErrorChan returns the channel for asynchronous errors
func (o *Ouroboros) ErrorChan() chan error {
	return o.errorChan
}

// Dial will establish a connection using the specified protocol and address. These parameters are
// passed to the [net.Dial] func. The handshake will be started when a connection is established.
// An error will be returned if the connection fails, a connection was already established, or the
// handshake fails
func (o *Ouroboros) Dial(proto string, address string) error {
	if o.conn != nil {
		return fmt.Errorf("a connection was already established")
	}
	conn, err := net.Dial(proto, address)
	if err != nil {
		return err
	}
	o.conn = conn
	if err := o.setupConnection(); err != nil {
		return err
	}
	return nil
}

// Close will shutdown the Ouroboros connection
func (o *Ouroboros) Close() error {
	// We use a mutex to prevent this function from being called multiple times
	// concurrently, which would cause a race condition
	o.closeMutex.Lock()
	defer o.closeMutex.Unlock()
	// Immediately return if we're already shutting down
	select {
	case <-o.doneChan:
		return nil
	default:
	}
	// Close doneChan to signify that we're shutting down
	close(o.doneChan)
	// Gracefully stop the muxer
	if o.muxer != nil {
		o.muxer.Stop()
	}
	// Close the underlying connection
	if o.conn != nil {
		if err := o.conn.Close(); err != nil {
			return err
		}
	}
	// Wait for other goroutines to finish
	o.waitGroup.Wait()
	// Close channels
	close(o.errorChan)
	close(o.protoErrorChan)
	// We can only close a channel once, so we have to jump through a few hoops
	select {
	// The channel is either closed or has an item pending
	case _, ok := <-o.handshakeFinishedChan:
		// We successfully retrieved an item
		// This will probably never happen, but it doesn't hurt to cover this case
		if ok {
			close(o.handshakeFinishedChan)
		}
	// The channel is open and has no pending items
	default:
		close(o.handshakeFinishedChan)
	}
	return nil
}

// ChainSync returns the chain-sync protocol handler
func (o *Ouroboros) ChainSync() *chainsync.ChainSync {
	return o.chainSync
}

// BlockFetch returns the block-fetch protocol handler
func (o *Ouroboros) BlockFetch() *blockfetch.BlockFetch {
	return o.blockFetch
}

// KeepAlive returns the keep-alive protocol handler
func (o *Ouroboros) KeepAlive() *keepalive.KeepAlive {
	return o.keepAlive
}

// LocalTxMonitor returns the local-tx-monitor protocol handler
func (o *Ouroboros) LocalTxMonitor() *localtxmonitor.LocalTxMonitor {
	return o.localTxMonitor
}

// LocalTxSubmission returns the local-tx-submission protocol handler
func (o *Ouroboros) LocalTxSubmission() *localtxsubmission.LocalTxSubmission {
	return o.localTxSubmission
}

// LocalStateQuery returns the local-state-query protocol handler
func (o *Ouroboros) LocalStateQuery() *localstatequery.LocalStateQuery {
	return o.localStateQuery
}

// TxSubmission returns the tx-submission protocol handler
func (o *Ouroboros) TxSubmission() *txsubmission.TxSubmission {
	return o.txSubmission
}

// setupConnection establishes the muxer, configures and starts the handshake process, and initializes
// the appropriate mini-protocols
func (o *Ouroboros) setupConnection() error {
	o.muxer = muxer.New(o.conn)
	// Start Goroutine to pass along errors from the muxer
	o.waitGroup.Add(1)
	go func() {
		defer o.waitGroup.Done()
		select {
		case <-o.doneChan:
			return
		case err, ok := <-o.muxer.ErrorChan():
			// Break out of goroutine if muxer's error channel is closed
			if !ok {
				return
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				// Return a bare io.EOF error if error is EOF/ErrUnexpectedEOF
				o.errorChan <- io.EOF
			} else {
				// Wrap error message to denote it comes from the muxer
				o.errorChan <- fmt.Errorf("muxer error: %s", err)
			}
			// Close connection on muxer errors
			o.Close()
		}
	}()
	protoOptions := protocol.ProtocolOptions{
		Muxer:     o.muxer,
		ErrorChan: o.protoErrorChan,
	}
	var protoVersions []uint16
	if o.useNodeToNodeProto {
		protoVersions = getProtocolVersionsNtN()
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
	} else {
		protoVersions = getProtocolVersionsNtC()
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
	}
	if o.server {
		protoOptions.Role = protocol.ProtocolRoleServer
	} else {
		protoOptions.Role = protocol.ProtocolRoleClient
	}
	// Check network magic value
	if o.networkMagic == 0 {
		return fmt.Errorf("invalid network magic value provided: %d\n", o.networkMagic)
	}
	// Perform handshake
	var handshakeVersion uint16
	var handshakeFullDuplex bool
	handshakeConfig := handshake.NewConfig(
		handshake.WithProtocolVersions(protoVersions),
		handshake.WithNetworkMagic(o.networkMagic),
		handshake.WithClientFullDuplex(o.fullDuplex),
		handshake.WithFinishedFunc(func(version uint16, fullDuplex bool) error {
			handshakeVersion = version
			handshakeFullDuplex = fullDuplex
			close(o.handshakeFinishedChan)
			return nil
		}),
	)
	o.handshake = handshake.New(protoOptions, &handshakeConfig)
	if o.server {
		o.handshake.Server.Start()
	} else {
		o.handshake.Client.Start()
	}
	// Wait for handshake completion or error
	select {
	case <-o.doneChan:
		// Return an error if we're shutting down
		return io.EOF
	case err := <-o.protoErrorChan:
		return err
	case <-o.handshakeFinishedChan:
		// This is purposely empty, but we need this case to break out when this channel is closed
	}
	// Provide the negotiated protocol version to the various mini-protocols
	protoOptions.Version = handshakeVersion
	// Drop bit used to signify NtC protocol versions
	if protoOptions.Version > protocolVersionNtCFlag {
		protoOptions.Version = protoOptions.Version - protocolVersionNtCFlag
	}
	// Start Goroutine to pass along errors from the mini-protocols
	o.waitGroup.Add(1)
	go func() {
		defer o.waitGroup.Done()
		err, ok := <-o.protoErrorChan
		// The channel is closed, which means we're already shutting down
		if !ok {
			return
		}
		o.errorChan <- fmt.Errorf("protocol error: %s", err)
		// Close connection on mini-protocol errors
		o.Close()
	}()
	// Configure the relevant mini-protocols
	if o.useNodeToNodeProto {
		versionNtN := getProtocolVersionNtN(handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
		o.chainSync = chainsync.New(protoOptions, o.chainSyncConfig)
		o.blockFetch = blockfetch.New(protoOptions, o.blockFetchConfig)
		o.txSubmission = txsubmission.New(protoOptions, o.txSubmissionConfig)
		if versionNtN.EnableKeepAliveProtocol {
			o.keepAlive = keepalive.New(protoOptions, o.keepAliveConfig)
			if !o.server && o.sendKeepAlives {
				o.keepAlive.Client.Start()
			}
		}
	} else {
		versionNtC := getProtocolVersionNtC(handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
		o.chainSync = chainsync.New(protoOptions, o.chainSyncConfig)
		o.localTxSubmission = localtxsubmission.New(protoOptions, o.localTxSubmissionConfig)
		if versionNtC.EnableLocalQueryProtocol {
			o.localStateQuery = localstatequery.New(protoOptions, o.localStateQueryConfig)
		}
		if versionNtC.EnableLocalTxMonitorProtocol {
			o.localTxMonitor = localtxmonitor.New(protoOptions, o.localTxMonitorConfig)
		}
	}
	// Start muxer
	diffusionMode := muxer.DiffusionModeInitiator
	if handshakeFullDuplex {
		diffusionMode = muxer.DiffusionModeInitiatorAndResponder
	} else if o.server {
		diffusionMode = muxer.DiffusionModeResponder
	}
	o.muxer.SetDiffusionMode(diffusionMode)
	if !o.delayMuxerStart {
		o.muxer.Start()
	}
	return nil
}
