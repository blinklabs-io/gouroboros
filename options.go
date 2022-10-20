package ouroboros

import (
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
