package handshake

import (
	"time"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

const (
	PROTOCOL_NAME = "handshake"
	PROTOCOL_ID   = 0

	DIFFUSION_MODE_INITIATOR_ONLY          = false
	DIFFUSION_MODE_INITIATOR_AND_RESPONDER = true
)

var (
	STATE_PROPOSE = protocol.NewState(1, "Propose")
	STATE_CONFIRM = protocol.NewState(2, "Confirm")
	STATE_DONE    = protocol.NewState(3, "Done")
)

var StateMap = protocol.StateMap{
	STATE_PROPOSE: protocol.StateMapEntry{
		Agency: protocol.AgencyClient,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_PROPOSE_VERSIONS,
				NewState: STATE_CONFIRM,
			},
		},
	},
	STATE_CONFIRM: protocol.StateMapEntry{
		Agency: protocol.AgencyServer,
		Transitions: []protocol.StateTransition{
			{
				MsgType:  MESSAGE_TYPE_ACCEPT_VERSION,
				NewState: STATE_DONE,
			},
			{
				MsgType:  MESSAGE_TYPE_REFUSE,
				NewState: STATE_DONE,
			},
		},
	},
	STATE_DONE: protocol.StateMapEntry{
		Agency: protocol.AgencyNone,
	},
}

type Handshake struct {
	Client *Client
	Server *Server
}

type Config struct {
	ProtocolVersions []uint16
	NetworkMagic     uint32
	ClientFullDuplex bool
	FinishedFunc     FinishedFunc
	Timeout          time.Duration
}

type FinishedFunc func(uint16, bool) error

func New(protoOptions protocol.ProtocolOptions, cfg *Config) *Handshake {
	h := &Handshake{
		Client: NewClient(protoOptions, cfg),
		Server: NewServer(protoOptions, cfg),
	}
	return h
}

type HandshakeOptionFunc func(*Config)

func NewConfig(options ...HandshakeOptionFunc) Config {
	c := Config{
		Timeout: 5 * time.Second,
	}
	// Apply provided options functions
	for _, option := range options {
		option(&c)
	}
	return c
}

func WithProtocolVersions(versions []uint16) HandshakeOptionFunc {
	return func(c *Config) {
		c.ProtocolVersions = versions
	}
}

func WithNetworkMagic(networkMagic uint32) HandshakeOptionFunc {
	return func(c *Config) {
		c.NetworkMagic = networkMagic
	}
}

func WithClientFullDuplex(fullDuplex bool) HandshakeOptionFunc {
	return func(c *Config) {
		c.ClientFullDuplex = fullDuplex
	}
}

func WithFinishedFunc(finishedFunc FinishedFunc) HandshakeOptionFunc {
	return func(c *Config) {
		c.FinishedFunc = finishedFunc
	}
}

func WithTimeout(timeout time.Duration) HandshakeOptionFunc {
	return func(c *Config) {
		c.Timeout = timeout
	}
}
