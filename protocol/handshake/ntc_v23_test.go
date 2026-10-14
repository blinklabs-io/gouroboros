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

package handshake_test

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	ouroboros "github.com/blinklabs-io/gouroboros"
	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/blinklabs-io/gouroboros/protocol/handshake"
	ouroboros_mock "github.com/blinklabs-io/ouroboros-mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

const ntcVersion23 = 23 + protocol.ProtocolVersionNtCOffset

func ntcV23VersionData() protocol.VersionData {
	return protocol.VersionDataNtC15andUp{
		CborNetworkMagic: ouroboros_mock.MockNetworkMagic,
	}
}

func closeAndWait(t *testing.T, oConn *ouroboros.Connection) {
	t.Helper()
	require.NoError(t, oConn.Close())
	select {
	case <-oConn.ErrorChan():
	case <-time.After(10 * time.Second):
		t.Errorf("did not shutdown within timeout")
	}
}

// TestClientNtCAcceptsV23OnlyNode covers a node that accepts nothing below
// node-to-client version 23. The client proposes its whole version map and
// must accept the node's choice of 23.
func TestClientNtCAcceptsV23OnlyNode(t *testing.T) {
	defer goleak.VerifyNone(t)
	mockConn := ouroboros_mock.NewConnection(
		ouroboros_mock.ProtocolRoleClient,
		[]ouroboros_mock.ConversationEntry{
			ouroboros_mock.ConversationEntryHandshakeRequestGeneric,
			ouroboros_mock.ConversationEntryOutput{
				ProtocolId: handshake.ProtocolId,
				IsResponse: true,
				Messages: []protocol.Message{
					handshake.NewMsgAcceptVersion(
						ntcVersion23,
						ntcV23VersionData(),
					),
				},
			},
		},
	)
	oConn, err := ouroboros.New(
		ouroboros.WithConnection(mockConn),
		ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
	)
	require.NoError(t, err)
	go func() {
		err, ok := <-oConn.ErrorChan()
		if !ok {
			return
		}
		panic(fmt.Sprintf("unexpected Ouroboros connection error: %s", err))
	}()
	version, versionData := oConn.ProtocolVersion()
	require.Equal(t, uint16(ntcVersion23), version)
	require.Equal(t, ntcV23VersionData(), versionData)
	closeAndWait(t, oConn)
}

// TestServerNtCNegotiatesV23 covers both a peer that proposes only version 23
// and one that also proposes older versions: the server picks the highest
// version both sides support.
func TestServerNtCNegotiatesV23(t *testing.T) {
	for _, test := range []struct {
		name     string
		proposed []uint16
	}{
		{"v23_only", []uint16{23}},
		{"v21_to_v23", []uint16{21, 22, 23}},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer goleak.VerifyNone(t)
			proposed := protocol.ProtocolVersionMap{}
			for _, n := range test.proposed {
				proposed[n+protocol.ProtocolVersionNtCOffset] = ntcV23VersionData()
			}
			mockConn := ouroboros_mock.NewConnection(
				ouroboros_mock.ProtocolRoleServer,
				[]ouroboros_mock.ConversationEntry{
					ouroboros_mock.ConversationEntryOutput{
						ProtocolId: handshake.ProtocolId,
						Messages: []protocol.Message{
							handshake.NewMsgProposeVersions(proposed),
						},
					},
					ouroboros_mock.ConversationEntryInput{
						ProtocolId:      handshake.ProtocolId,
						IsResponse:      true,
						MsgFromCborFunc: handshake.NewMsgFromCbor,
						Message: handshake.NewMsgAcceptVersion(
							ntcVersion23,
							ntcV23VersionData(),
						),
					},
				},
			)
			oConn, err := ouroboros.New(
				ouroboros.WithConnection(mockConn),
				ouroboros.WithNetworkMagic(ouroboros_mock.MockNetworkMagic),
				ouroboros.WithServer(true),
			)
			require.NoError(t, err)
			go func() {
				err, ok := <-oConn.ErrorChan()
				if !ok {
					return
				}
				panic(fmt.Sprintf("unexpected Ouroboros error: %s", err))
			}()
			version, _ := oConn.ProtocolVersion()
			require.Equal(t, uint16(ntcVersion23), version)
			closeAndWait(t, oConn)
		})
	}
}

// TestProposeVersionsNtCAscending pins the proposal order. The Haskell
// handshake decoder fails the whole proposal on a key that is not greater
// than the previous one, so the highest version goes last, not first.
func TestProposeVersionsNtCAscending(t *testing.T) {
	msg := handshake.NewMsgProposeVersions(
		protocol.GetProtocolVersionMap(
			protocol.ProtocolModeNodeToClient,
			ouroboros_mock.MockNetworkMagic,
			protocol.DiffusionModeInitiatorOnly,
			false,
			false,
		),
	)
	data, err := cbor.Encode(msg)
	require.NoError(t, err)
	versions := protocol.GetProtocolVersionsNtC()
	require.Equal(
		t,
		uint16(ntcVersion23),
		versions[len(versions)-1],
	)
	prev := -1
	for _, version := range versions {
		key := []byte{0x19, byte(version >> 8), byte(version)}
		pos := bytes.Index(data, key)
		require.GreaterOrEqual(t, pos, 0, "version %#x not proposed", version)
		require.Greater(t, pos, prev, "version %#x out of order", version)
		prev = pos
	}
}
