package ouroboros

import (
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
	"net"
)

type Ouroboros struct {
	conn               net.Conn
	networkMagic       uint32
	server             bool
	useNodeToNodeProto bool
	handshakeComplete  bool
	muxer              *muxer.Muxer
	ErrorChan          chan error
	protoErrorChan     chan error
	sendKeepAlives     bool
	delayMuxerStart    bool
	// Mini-protocols
	Handshake                       *handshake.Handshake
	ChainSync                       *chainsync.ChainSync
	chainSyncCallbackConfig         *chainsync.ChainSyncCallbackConfig
	BlockFetch                      *blockfetch.BlockFetch
	blockFetchCallbackConfig        *blockfetch.BlockFetchCallbackConfig
	KeepAlive                       *keepalive.KeepAlive
	keepAliveCallbackConfig         *keepalive.KeepAliveCallbackConfig
	LocalTxSubmission               *localtxsubmission.LocalTxSubmission
	localTxSubmissionCallbackConfig *localtxsubmission.CallbackConfig
	LocalStateQuery                 *localstatequery.LocalStateQuery
	localStateQueryCallbackConfig   *localstatequery.CallbackConfig
	TxSubmission                    *txsubmission.TxSubmission
	txSubmissionCallbackConfig      *txsubmission.CallbackConfig
}

type OuroborosOptions struct {
	Conn                            net.Conn
	NetworkMagic                    uint32
	ErrorChan                       chan error
	Server                          bool
	UseNodeToNodeProtocol           bool
	SendKeepAlives                  bool
	DelayMuxerStart                 bool
	ChainSyncCallbackConfig         *chainsync.ChainSyncCallbackConfig
	BlockFetchCallbackConfig        *blockfetch.BlockFetchCallbackConfig
	KeepAliveCallbackConfig         *keepalive.KeepAliveCallbackConfig
	LocalTxSubmissionCallbackConfig *localtxsubmission.CallbackConfig
	LocalStateQueryCallbackConfig   *localstatequery.CallbackConfig
}

func New(options *OuroborosOptions) (*Ouroboros, error) {
	o := &Ouroboros{
		conn:                            options.Conn,
		networkMagic:                    options.NetworkMagic,
		server:                          options.Server,
		useNodeToNodeProto:              options.UseNodeToNodeProtocol,
		chainSyncCallbackConfig:         options.ChainSyncCallbackConfig,
		blockFetchCallbackConfig:        options.BlockFetchCallbackConfig,
		keepAliveCallbackConfig:         options.KeepAliveCallbackConfig,
		localTxSubmissionCallbackConfig: options.LocalTxSubmissionCallbackConfig,
		localStateQueryCallbackConfig:   options.LocalStateQueryCallbackConfig,
		ErrorChan:                       options.ErrorChan,
		sendKeepAlives:                  options.SendKeepAlives,
		delayMuxerStart:                 options.DelayMuxerStart,
		protoErrorChan:                  make(chan error, 10),
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
	o.muxer.Stop()
	// Close the underlying connection
	if err := o.conn.Close(); err != nil {
		return err
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
		o.ErrorChan <- fmt.Errorf("muxer error: %s", err)
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
	o.Handshake = handshake.New(protoOptions, protoVersions)
	// TODO: figure out better way to signify automatic handshaking and returning the chosen version
	if !o.server {
		err := o.Handshake.ProposeVersions(protoVersions, o.networkMagic)
		if err != nil {
			return err
		}
	}
	// Wait for handshake completion or error
	select {
	case err := <-o.protoErrorChan:
		return err
	case finished := <-o.Handshake.Finished:
		o.handshakeComplete = finished
	}
	// Provide the negotiated protocol version to the various mini-protocols
	protoOptions.Version = o.Handshake.Version
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
		versionNtN := GetProtocolVersionNtN(o.Handshake.Version)
		protoOptions.Mode = protocol.ProtocolModeNodeToNode
		o.ChainSync = chainsync.New(protoOptions, o.chainSyncCallbackConfig)
		o.BlockFetch = blockfetch.New(protoOptions, o.blockFetchCallbackConfig)
		o.TxSubmission = txsubmission.New(protoOptions, o.txSubmissionCallbackConfig)
		if versionNtN.EnableKeepAliveProtocol {
			o.KeepAlive = keepalive.New(protoOptions, o.keepAliveCallbackConfig)
			if o.sendKeepAlives {
				o.KeepAlive.Start()
			}
		}
	} else {
		versionNtC := GetProtocolVersionNtC(o.Handshake.Version)
		protoOptions.Mode = protocol.ProtocolModeNodeToClient
		o.ChainSync = chainsync.New(protoOptions, o.chainSyncCallbackConfig)
		o.LocalTxSubmission = localtxsubmission.New(protoOptions, o.localTxSubmissionCallbackConfig)
		if versionNtC.EnableLocalQueryProtocol {
			o.LocalStateQuery = localstatequery.New(protoOptions, o.localStateQueryCallbackConfig)
		}
	}
	// Start muxer
	if !o.delayMuxerStart {
		o.muxer.Start()
	}
	return nil
}
