// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package peersharing

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// Message types
const (
	MessageTypeShareRequest = 0
	MessageTypeSharePeers   = 1
	MessageTypeDone         = 2
)

// NewMsgFromCbor parses a PeerSharing message from CBOR
func NewMsgFromCbor(msgType uint, data []byte) (protocol.Message, error) {
	var ret protocol.Message
	switch msgType {
	case MessageTypeShareRequest:
		ret = &MsgShareRequest{}
	case MessageTypeSharePeers:
		ret = &MsgSharePeers{}
	case MessageTypeDone:
		ret = &MsgDone{}
	}
	if _, err := cbor.Decode(data, ret); err != nil {
		return nil, fmt.Errorf("%s: decode error: %w", ProtocolName, err)
	}
	if ret != nil {
		// Store the raw message CBOR
		ret.SetCbor(data)
	}
	return ret, nil
}

type MsgShareRequest struct {
	protocol.MessageBase
	Amount uint8
}

func NewMsgShareRequest(amount uint8) *MsgShareRequest {
	m := &MsgShareRequest{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeShareRequest,
		},
		Amount: amount,
	}
	return m
}

type MsgSharePeers struct {
	protocol.MessageBase
	PeerAddresses []PeerAddress
}

func NewMsgSharePeers(peerAddresses []PeerAddress) *MsgSharePeers {
	m := &MsgSharePeers{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeSharePeers,
		},
		PeerAddresses: peerAddresses,
	}
	return m
}

type MsgDone struct {
	protocol.MessageBase
}

func NewMsgDone() *MsgDone {
	m := &MsgDone{
		MessageBase: protocol.MessageBase{
			MessageType: MessageTypeDone,
		},
	}
	return m
}

type PeerAddress struct {
	IP   net.IP
	Port uint16
}

func (p PeerAddress) MarshalCBOR() ([]byte, error) {
	if ipv4 := p.IP.To4(); ipv4 != nil {
		tmpPeer := struct {
			cbor.StructAsArray
			PeerType int
			Address  uint32
			Port     uint16
		}{
			PeerType: 0,
			Address:  binary.LittleEndian.Uint32(ipv4),
			Port:     p.Port,
		}
		return cbor.Encode(tmpPeer)
	}

	ipv6 := p.IP.To16()
	if ipv6 == nil {
		return nil, fmt.Errorf("invalid IP address: %q", p.IP.String())
	}

	tmpPeer := struct {
		cbor.StructAsArray
		PeerType int
		Address1 uint32
		Address2 uint32
		Address3 uint32
		Address4 uint32
		Port     uint16
	}{
		PeerType: 1,
		Address1: binary.LittleEndian.Uint32(ipv6[0:4]),
		Address2: binary.LittleEndian.Uint32(ipv6[4:8]),
		Address3: binary.LittleEndian.Uint32(ipv6[8:12]),
		Address4: binary.LittleEndian.Uint32(ipv6[12:16]),
		Port:     p.Port,
	}
	return cbor.Encode(tmpPeer)
}

func (p *PeerAddress) UnmarshalCBOR(cborData []byte) error {
	peerType, err := cbor.DecodeIdFromList(cborData)
	if err != nil {
		return err
	}
	switch peerType {
	case 0:
		// IPv4
		tmpPeer := struct {
			cbor.StructAsArray
			PeerType int
			Address  uint32
			Port     uint16
		}{}
		if _, err := cbor.Decode(cborData, &tmpPeer); err != nil {
			return err
		}
		p.IP = make(net.IP, net.IPv4len)
		binary.LittleEndian.PutUint32(p.IP, tmpPeer.Address)
		p.Port = tmpPeer.Port
	case 1:
		// IPv6
		cborListLen, err := cbor.ListLength(cborData)
		if err != nil {
			return err
		}
		switch cborListLen {
		case 8:
			// V11-12
			tmpPeer := struct {
				cbor.StructAsArray
				PeerType int
				Address1 uint32
				Address2 uint32
				Address3 uint32
				Address4 uint32
				FlowInfo uint32 // ignored
				ScopeId  uint32 // ignored
				Port     uint16
			}{}
			if _, err := cbor.Decode(cborData, &tmpPeer); err != nil {
				return err
			}
			p.IP = make(net.IP, net.IPv6len)
			binary.LittleEndian.PutUint32(p.IP[0:], tmpPeer.Address1)
			binary.LittleEndian.PutUint32(p.IP[4:], tmpPeer.Address2)
			binary.LittleEndian.PutUint32(p.IP[8:], tmpPeer.Address3)
			binary.LittleEndian.PutUint32(p.IP[12:], tmpPeer.Address4)
			p.Port = tmpPeer.Port
		case 6:
			// V13+
			tmpPeer := struct {
				cbor.StructAsArray
				PeerType int
				Address1 uint32
				Address2 uint32
				Address3 uint32
				Address4 uint32
				Port     uint16
			}{}
			if _, err := cbor.Decode(cborData, &tmpPeer); err != nil {
				return err
			}
			p.IP = make(net.IP, net.IPv6len)
			binary.LittleEndian.PutUint32(p.IP[0:], tmpPeer.Address1)
			binary.LittleEndian.PutUint32(p.IP[4:], tmpPeer.Address2)
			binary.LittleEndian.PutUint32(p.IP[8:], tmpPeer.Address3)
			binary.LittleEndian.PutUint32(p.IP[12:], tmpPeer.Address4)
			p.Port = tmpPeer.Port
		default:
			return fmt.Errorf("invalid peer address length: %d", cborListLen)
		}
	default:
		return fmt.Errorf("unknown peer type: %d", peerType)
	}
	return nil
}
