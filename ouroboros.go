package ouroboros

import (
	"errors"
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/protocol"
	"github.com/cloudstruct/go-ouroboros-network/protocol/blockfetch"
	"github.com/cloudstruct/go-ouroboros-network/protocol/chainsync"
	"github.com/cloudstruct/go-ouroboros-network/protocol/handshake"
	"github.com/cloudstruct/go-ouroboros-network/protocol/keepalive"
	"github.com/cloudstruct/go-ouroboros-network/protocol/localstatequery"
	"github.com/cloudstruct/go-ouroboros-network/protocol/localtxsubmission"
	"github.com/cloudstruct/go-ouroboros-network/protocol/txsubmission"
	"io"
	"net"
)

type Ouroboros struct {
	conn               net.Conn
	networkMagic       uint32
	server             bool
	useNodeToNodeProto bool
	muxer              *muxer.Muxer
	ErrorChan          chan error
	protoErrorChan     chan error
	sendKeepAlives     bool
	delayMuxerStart    bool
	fullDuplex         bool
	// Mini-protocols
	Handshake               *handshake.Handshake
	ChainSync               *chainsync.ChainSync
	chainSyncConfig         *chainsync.Config
	BlockFetch              *blockfetch.BlockFetch
	blockFetchConfig        *blockfetch.Config
	KeepAlive               *keepalive.KeepAlive
	keepAliveConfig         *keepalive.Config
	LocalTxSubmission       *localtxsubmission.LocalTxSubmission
	localTxSubmissionConfig *localtxsubmission.Config
	LocalStateQuery         *localstatequery.LocalStateQuery
	localStateQueryConfig   *localstatequery.Config
	TxSubmission            *txsubmission.TxSubmission
	txSubmissionConfig      *txsubmission.Config
}

func New(options ...OuroborosOptionFunc) (*Ouroboros, error) {
	o := &Ouroboros{
		protoErrorChan: make(chan error, 10),
	}
	// Apply provided options functions
	for _, option := range options {
		option(o)
	}
	if o.ErrorChan == nil {
		o.ErrorChan = make(chan error, 10)
	}
	if o.conn != nil {
		if err := o.setupConnection(); err != nil {
			return nil, err
		}
	}
	return o, nil
}

func (o *Ouroboros) Muxer() *muxer.Muxer {
	return o.muxer
}

// Convenience function for creating a connection if you didn't provide one when
// calling New()
func (o *Ouroboros) Dial(proto string, address string) error {
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

func (o *Ouroboros) Close() error {
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
	return nil
}

func (o *Ouroboros) setupConnection() error {
	o.muxer = muxer.New(o.conn)
	// Start Goroutine to pass along errors from the muxer
	go func() {
		err, ok := <-o.muxer.ErrorChan
		// Break out of goroutine if muxer's error channel is closed
		if !ok {
			return
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			// Return a bare io.EOF error if error is EOF/ErrUnexpectedEOF
			o.ErrorChan <- io.EOF
		} else {
			// Wrap error message to denote it comes from the muxer
			o.ErrorChan <- fmt.Errorf("muxer error: %s", err)
		}
		// Close connection on muxer errors
		o.Close()
	}()
	protoOptions := protocol.ProtocolOptions{
		Muxer:     o.muxer,
		ErrorChan: o.protoErrorChan,
	}
	var protoVersions []uint16
	if o.useNodeToNodeProto {
		protoVersions = GetProtocolVersionsNtN()
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
	} else {
		protoVersions = GetProtocolVersionsNtC()
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
	}
	if o.server {
		protoOptions.Role = protocol.ProtocolRoleServer
	} else {
		protoOptions.Role = protocol.ProtocolRoleClient
	}
	// Perform handshake
	handshakeFinishedChan := make(chan interface{})
	var handshakeVersion uint16
	var handshakeFullDuplex bool
	handshakeConfig := &handshake.Config{
		ProtocolVersions: protoVersions,
		NetworkMagic:     o.networkMagic,
		ClientFullDuplex: o.fullDuplex,
		FinishedFunc: func(version uint16, fullDuplex bool) error {
			handshakeVersion = version
			handshakeFullDuplex = fullDuplex
			close(handshakeFinishedChan)
			return nil
		},
	}
	o.Handshake = handshake.New(protoOptions, handshakeConfig)
	if o.server {
		o.Handshake.Server.Start()
	} else {
		o.Handshake.Client.Start()
	}
	// Wait for handshake completion or error
	select {
	case err := <-o.protoErrorChan:
		return err
	case <-handshakeFinishedChan:
	}
	// Provide the negotiated protocol version to the various mini-protocols
	protoOptions.Version = handshakeVersion
	// Drop bit used to signify NtC protocol versions
	if protoOptions.Version > PROTOCOL_VERSION_NTC_FLAG {
		protoOptions.Version = protoOptions.Version - PROTOCOL_VERSION_NTC_FLAG
	}
	// Start Goroutine to pass along errors from the mini-protocols
	go func() {
		err := <-o.protoErrorChan
		o.ErrorChan <- fmt.Errorf("protocol error: %s", err)
		// Close connection on mini-protocol errors
		o.Close()
	}()
	// Configure the relevant mini-protocols
	if o.useNodeToNodeProto {
		versionNtN := GetProtocolVersionNtN(handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
		o.ChainSync = chainsync.New(protoOptions, o.chainSyncConfig)
		o.BlockFetch = blockfetch.New(protoOptions, o.blockFetchConfig)
		o.TxSubmission = txsubmission.New(protoOptions, o.txSubmissionConfig)
		if versionNtN.EnableKeepAliveProtocol {
			o.KeepAlive = keepalive.New(protoOptions, o.keepAliveConfig)
			if !o.server && o.sendKeepAlives {
				o.KeepAlive.Client.Start()
			}
		}
	} else {
		versionNtC := GetProtocolVersionNtC(handshakeVersion)
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
		o.ChainSync = chainsync.New(protoOptions, o.chainSyncConfig)
		o.LocalTxSubmission = localtxsubmission.New(protoOptions, o.localTxSubmissionConfig)
		if versionNtC.EnableLocalQueryProtocol {
			o.LocalStateQuery = localstatequery.New(protoOptions, o.localStateQueryConfig)
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
