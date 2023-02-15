package ouroboros

import (
	"github.com/cloudstruct/go-ouroboros-network/protocol/blockfetch"
	"github.com/cloudstruct/go-ouroboros-network/protocol/chainsync"
	"github.com/cloudstruct/go-ouroboros-network/protocol/keepalive"
	"github.com/cloudstruct/go-ouroboros-network/protocol/localstatequery"
	"github.com/cloudstruct/go-ouroboros-network/protocol/localtxsubmission"
	"github.com/cloudstruct/go-ouroboros-network/protocol/txsubmission"
	"net"
)

// OuroborosOptionFunc is a type that represents functions that modify the Ouroboros config
type OuroborosOptionFunc func(*Ouroboros)

// WithConnection specifies an existing connection to use. If none is provided, the Dial() function can be
// used to create one later
func WithConnection(conn net.Conn) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.conn = conn
	}
}

// WithNetwork specifies the network
func WithNetwork(network Network) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.networkMagic = network.NetworkMagic
	}
}

// WithNetworkMagic specifies the network magic value
func WithNetworkMagic(networkMagic uint32) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.networkMagic = networkMagic
	}
}

// WithErrorChan specifies the error channel to use. If none is provided, one will be created
func WithErrorChan(errorChan chan error) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.errorChan = errorChan
	}
}

// WithServer specifies whether to act as a server
func WithServer(server bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.server = server
	}
}

// WithNodeToNode specifies whether to use the node-to-node protocol. The default is to use node-to-client
func WithNodeToNode(nodeToNode bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.useNodeToNodeProto = nodeToNode
	}
}

// WithKeepAlives specifies whether to use keep-alives. This is disabled by default
func WithKeepAlive(keepAlive bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.sendKeepAlives = keepAlive
	}
}

// WithDelayMuxerStart specifies whether to delay the muxer start. This is useful if you need to take some
// custom actions before the muxer starts processing messages, generally when acting as a server
func WithDelayMuxerStart(delayMuxerStart bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.delayMuxerStart = delayMuxerStart
	}
}

// WithFullDuplex specifies whether to enable full-duplex mode when acting as a client
func WithFullDuplex(fullDuplex bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.fullDuplex = fullDuplex
	}
}

// WithBlockFetchConfig specifies BlockFetch protocol config
func WithBlockFetchConfig(cfg blockfetch.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.blockFetchConfig = &cfg
	}
}

// WithChainSyncConfig secifies ChainSync protocol config
func WithChainSyncConfig(cfg chainsync.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.chainSyncConfig = &cfg
	}
}

// WithKeepAliveConfig specifies KeepAlive protocol config
func WithKeepAliveConfig(cfg keepalive.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.keepAliveConfig = &cfg
	}
}

// WithLocalStateQueryConfig specifies LocalStateQuery protocol config
func WithLocalStateQueryConfig(cfg localstatequery.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.localStateQueryConfig = &cfg
	}
}

// WithLocalTxSubmissionConfig specifies LocalTxSubmission protocol config
func WithLocalTxSubmissionConfig(cfg localtxsubmission.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.localTxSubmissionConfig = &cfg
	}
}

// WithTxSubmissionConfig specifies TxSubmission protocol config
func WithTxSubmissionConfig(cfg txsubmission.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.txSubmissionConfig = &cfg
	}
}
