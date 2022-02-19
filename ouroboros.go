package ouroboros

import (
	"github.com/cloudstruct/go-ouroboros-network/muxer"
	"github.com/cloudstruct/go-ouroboros-network/protocol/blockfetch"
	"github.com/cloudstruct/go-ouroboros-network/protocol/chainsync"
	"github.com/cloudstruct/go-ouroboros-network/protocol/handshake"
	"io"
	"net"
)

type Ouroboros struct {
	conn               io.ReadWriteCloser
	networkMagic       uint32
	waitForHandshake   bool
	useNodeToNodeProto bool
	handshakeComplete  bool
	muxer              *muxer.Muxer
	ErrorChan          chan error
	// Mini-protocols
	Handshake                *handshake.Handshake
	ChainSync                *chainsync.ChainSync
	chainSyncCallbackConfig  *chainsync.ChainSyncCallbackConfig
	BlockFetch               *blockfetch.BlockFetch
	blockFetchCallbackConfig *blockfetch.BlockFetchCallbackConfig
}

type OuroborosOptions struct {
	Conn         io.ReadWriteCloser
	NetworkMagic uint32
	ErrorChan    chan error
	// Whether to wait for the other side to initiate the handshake. This is useful
	// for servers
	WaitForHandshake         bool
	UseNodeToNodeProtocol    bool
	ChainSyncCallbackConfig  *chainsync.ChainSyncCallbackConfig
	BlockFetchCallbackConfig *blockfetch.BlockFetchCallbackConfig
}

func New(options *OuroborosOptions) (*Ouroboros, error) {
	o := &Ouroboros{
		conn:                     options.Conn,
		networkMagic:             options.NetworkMagic,
		waitForHandshake:         options.WaitForHandshake,
		useNodeToNodeProto:       options.UseNodeToNodeProtocol,
		chainSyncCallbackConfig:  options.ChainSyncCallbackConfig,
		blockFetchCallbackConfig: options.BlockFetchCallbackConfig,
		ErrorChan:                options.ErrorChan,
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

func (o *Ouroboros) setupConnection() error {
	o.muxer = muxer.New(o.conn)
	// Start Goroutine to pass along errors from the muxer
	go func() {
		err := <-o.muxer.ErrorChan
		o.ErrorChan <- err
	}()
	// Perform handshake
	o.Handshake = handshake.New(o.muxer, o.ErrorChan, o.useNodeToNodeProto)
	var protoVersions []uint16
	if o.useNodeToNodeProto {
		protoVersions = GetProtocolVersionsNtN()
	} else {
		protoVersions = GetProtocolVersionsNtC()
	}
	// TODO: figure out better way to signify automatic handshaking and returning the chosen version
	if !o.waitForHandshake {
		err := o.Handshake.ProposeVersions(protoVersions, o.networkMagic)
		if err != nil {
			return err
		}
	}
	o.handshakeComplete = <-o.Handshake.Finished
	// TODO: register additional mini-protocols
	if o.useNodeToNodeProto {
		//versionNtN := GetProtocolVersionNtN(o.Handshake.Version)
		o.ChainSync = chainsync.New(o.muxer, o.ErrorChan, o.useNodeToNodeProto, o.chainSyncCallbackConfig)
		o.BlockFetch = blockfetch.New(o.muxer, o.ErrorChan, o.blockFetchCallbackConfig)
	} else {
		//versionNtC := GetProtocolVersionNtC(o.Handshake.Version)
		o.ChainSync = chainsync.New(o.muxer, o.ErrorChan, o.useNodeToNodeProto, o.chainSyncCallbackConfig)
	}
	return nil
}
