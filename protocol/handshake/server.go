package handshake

import (
	"fmt"

	"github.com/cloudstruct/go-ouroboros-network/protocol"
)

// Server implements the Handshake server
type Server struct {
	*protocol.Protocol
	config *Config
}

// NewServer returns a new Handshake server object
func NewServer(protoOptions protocol.ProtocolOptions, cfg *Config) *Server {
	s := &Server{
		config: cfg,
	}
	protoConfig := protocol.ProtocolConfig{
		Name:                protocolName,
		ProtocolId:          protocolId,
		Muxer:               protoOptions.Muxer,
		ErrorChan:           protoOptions.ErrorChan,
		Mode:                protoOptions.Mode,
		Role:                protocol.ProtocolRoleServer,
		MessageHandlerFunc:  s.handleMessage,
		MessageFromCborFunc: NewMsgFromCbor,
		StateMap:            StateMap,
		InitialState:        statePropose,
	}
	s.Protocol = protocol.New(protoConfig)
	return s
}

func (s *Server) handleMessage(msg protocol.Message, isResponse bool) error {
	var err error
	switch msg.Type() {
	case MessageTypeProposeVersions:
		err = s.handleProposeVersions(msg)
	default:
		err = fmt.Errorf("%s: received unexpected message type %d", protocolName, msg.Type())
	}
	return err
}

func (s *Server) handleProposeVersions(msgGeneric protocol.Message) error {
	if s.config.FinishedFunc == nil {
		return fmt.Errorf("received handshake ProposeVersions message but no callback function is defined")
	}
	msg := msgGeneric.(*MsgProposeVersions)
	var highestVersion uint16
	var fullDuplex bool
	var versionData []interface{}
	for proposedVersion := range msg.VersionMap {
		if proposedVersion > highestVersion {
			for _, allowedVersion := range s.config.ProtocolVersions {
				if allowedVersion == proposedVersion {
					highestVersion = proposedVersion
					versionData = msg.VersionMap[proposedVersion].([]interface{})
					//nolint:gosimple
					if versionData[1].(bool) == DiffusionModeInitiatorAndResponder {
						fullDuplex = true
					} else {
						fullDuplex = false
					}
					break
				}
			}
		}
	}
	if highestVersion > 0 {
		resp := NewMsgAcceptVersion(highestVersion, versionData)
		if err := s.SendMessage(resp); err != nil {
			return err
		}
		return s.config.FinishedFunc(highestVersion, fullDuplex)
	} else {
		// TODO: handle failures
		// https://github.com/cloudstruct/go-ouroboros-network/issues/32
		return fmt.Errorf("handshake failed, but we don't yet support this")
	}
}
