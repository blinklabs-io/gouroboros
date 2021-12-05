package ouroboros

import (
	"fmt"
	"github.com/cloudstruct/go-ouroboros-network/handshake"
	"io"
	"net"
)

type Ouroboros struct {
	conn              io.ReadWriteCloser
	networkMagic      uint32
	handshakeComplete bool
	muxer             *Muxer
	ErrorChan         chan error
	// Mini-protocols
	Handshake *handshake.Handshake
}

type OuroborosOptions struct {
	Conn         io.ReadWriteCloser
	NetworkMagic uint32
}

func New(options *OuroborosOptions) (*Ouroboros, error) {
	o := &Ouroboros{
		conn:         options.Conn,
		networkMagic: options.NetworkMagic,
		ErrorChan:    make(chan error, 10),
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
	o.muxer = NewMuxer(o.conn)
	// Start Goroutine to pass along errors from the muxer
	go func() {
		err := <-o.muxer.errorChan
		o.ErrorChan <- err
	}()
	// Perform handshake
	handshakeSendChan, handshakeRecvChan := o.muxer.registerProtocol(handshake.PROTOCOL_ID_SENDER, handshake.PROTOCOL_ID_RECEIVER)
	o.Handshake = handshake.New(handshakeSendChan, handshakeRecvChan)
	// TODO: create a proper version map
	protoVersion, err := o.Handshake.Start([]uint16{1, 32778}, o.networkMagic)
	if err != nil {
		return err
	}
	o.handshakeComplete = true
	fmt.Printf("negotiated protocol version %d\n", protoVersion)
	// TODO: register additional mini-protocols
	return nil
}
