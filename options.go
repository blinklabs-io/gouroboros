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

type OuroborosOptionFunc func(*Ouroboros)

func WithConnection(conn net.Conn) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.conn = conn
	}
}

func WithNetworkMagic(networkMagic uint32) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.networkMagic = networkMagic
	}
}

func WithErrorChan(errorChan chan error) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.ErrorChan = errorChan
	}
}

func WithServer(server bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.server = server
	}
}

func WithNodeToNode(nodeToNode bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.useNodeToNodeProto = nodeToNode
	}
}

func WithKeepAlive(keepAlive bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.sendKeepAlives = keepAlive
	}
}

func WithDelayMuxerStart(delayMuxerStart bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.delayMuxerStart = delayMuxerStart
	}
}

func WithFullDuplex(fullDuplex bool) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.fullDuplex = fullDuplex
	}
}

func WithBlockFetchConfig(cfg blockfetch.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.blockFetchConfig = &cfg
	}
}

func WithChainSyncConfig(cfg chainsync.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.chainSyncConfig = &cfg
	}
}

func WithKeepAliveConfig(cfg keepalive.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.keepAliveConfig = &cfg
	}
}

func WithLocalStateQueryConfig(cfg localstatequery.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.localStateQueryConfig = &cfg
	}
}

func WithLocalTxSubmissionConfig(cfg localtxsubmission.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.localTxSubmissionConfig = &cfg
	}
}

func WithTxSubmissionConfig(cfg txsubmission.Config) OuroborosOptionFunc {
	return func(o *Ouroboros) {
		o.txSubmissionConfig = &cfg
	}
}
