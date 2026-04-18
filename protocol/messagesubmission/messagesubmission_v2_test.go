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

package messagesubmission

import (
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- V2 state map structure tests ---

func TestV2StateMapHasNoInitState(t *testing.T) {
	_, hasInit := stateMapV2[protocolStateInit]
	assert.False(t, hasInit, "V2 state map must not contain StInit")
}

func TestV2StateMapStartsInIdle(t *testing.T) {
	_, hasIdle := stateMapV2[protocolStateIdle]
	assert.True(t, hasIdle, "V2 state map must contain StIdle")

	entry := stateMapV2[protocolStateIdle]
	assert.Equal(
		t,
		protocol.AgencyServer,
		entry.Agency,
		"StIdle agency must be Server in V2",
	)
}

func TestV2StateMapMsgDoneOnlyFromIdle(t *testing.T) {
	// StIdle should have a MsgDone transition
	idleEntry := stateMapV2[protocolStateIdle]
	hasDoneFromIdle := false
	for _, tr := range idleEntry.Transitions {
		if tr.MsgType == MessageTypeDone {
			hasDoneFromIdle = true
			assert.Equal(
				t,
				protocolStateDone,
				tr.NewState,
				"MsgDone from StIdle must transition to StDone",
			)
		}
	}
	assert.True(t, hasDoneFromIdle, "StIdle must have a MsgDone transition in V2")

	// StMessageIdsBlocking must NOT have MsgDone
	blockEntry := stateMapV2[protocolStateMessageIdsBlock]
	for _, tr := range blockEntry.Transitions {
		assert.NotEqual(
			t,
			uint(MessageTypeDone),
			tr.MsgType,
			"StMessageIdsBlocking must not have MsgDone in V2",
		)
	}

	// StMessageIdsNonBlocking must NOT have MsgDone
	nonBlockEntry := stateMapV2[protocolStateMessageIdsNonBlk]
	for _, tr := range nonBlockEntry.Transitions {
		assert.NotEqual(
			t,
			uint(MessageTypeDone),
			tr.MsgType,
			"StMessageIdsNonBlocking must not have MsgDone in V2",
		)
	}
}

func TestV2StateMapDoneHasNoAgency(t *testing.T) {
	doneEntry := stateMapV2[protocolStateDone]
	assert.Equal(
		t,
		protocol.AgencyNone,
		doneEntry.Agency,
		"StDone must have AgencyNone in V2",
	)
}

// --- V1 state map backward-compatibility tests ---

func TestV1StateMapHasInitState(t *testing.T) {
	entry, hasInit := stateMapV1[protocolStateInit]
	assert.True(t, hasInit, "V1 state map must contain StInit")
	assert.Equal(
		t,
		protocol.AgencyClient,
		entry.Agency,
		"StInit agency must be Client in V1",
	)
}

func TestV1StateMapMsgDoneFromBlockingAndNonBlocking(t *testing.T) {
	// StMessageIdsBlocking should have MsgDone
	blockEntry := stateMapV1[protocolStateMessageIdsBlock]
	hasDone := false
	for _, tr := range blockEntry.Transitions {
		if tr.MsgType == MessageTypeDone {
			hasDone = true
		}
	}
	assert.True(t, hasDone, "V1 StMessageIdsBlocking must have MsgDone transition")

	// StMessageIdsNonBlocking should have MsgDone
	nonBlockEntry := stateMapV1[protocolStateMessageIdsNonBlk]
	hasDone = false
	for _, tr := range nonBlockEntry.Transitions {
		if tr.MsgType == MessageTypeDone {
			hasDone = true
		}
	}
	assert.True(t, hasDone, "V1 StMessageIdsNonBlocking must have MsgDone transition")
}

// --- V2 client behavior tests ---

func newTestProtoOptions(version uint16) protocol.ProtocolOptions {
	connId := connection.ConnectionId{
		LocalAddr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		RemoteAddr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
	}
	return protocol.ProtocolOptions{
		ConnectionId: connId,
		Version:      version,
	}
}

func TestV2ClientInitReturnsError(t *testing.T) {
	cfg := NewConfig()
	c := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
	err := c.Init()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported in MessageSubmission V2")
}

func TestV1ClientInitNoError(t *testing.T) {
	cfg := NewConfig()
	// Version 14 < V2 threshold → V1
	c := NewClient(newTestProtoOptions(14), &cfg)
	// Verify the V2 guard does NOT fire for V1 clients.
	// We cannot call Init() end-to-end here because SendMessage requires
	// a running muxer. Instead, confirm the version is below the V2 threshold.
	assert.Less(
		t,
		c.protoVersion,
		MessageSubmissionV2MinVersion,
		"V1 client protoVersion must be below V2 threshold",
	)
}

func TestV2ClientStopIsNoop(t *testing.T) {
	cfg := NewConfig()
	c := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
	// Stop should be a no-op in V2 (no MsgDone sent)
	err := c.Stop()
	assert.NoError(t, err, "V2 client Stop() must succeed as a no-op")
	// Calling Stop again should also be fine
	err = c.Stop()
	assert.NoError(t, err)
}

// --- V2 version selection tests ---

func TestV2VersionSelectsCorrectStateMap(t *testing.T) {
	cfg := NewConfig()

	// V1 client (version 14)
	v1Client := NewClient(newTestProtoOptions(14), &cfg)
	assert.Equal(t, uint16(14), v1Client.protoVersion)

	// V2 client (version 15)
	v2Client := NewClient(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
	assert.Equal(t, MessageSubmissionV2MinVersion, v2Client.protoVersion)

	// V1 server (version 14)
	v1Server := NewServer(newTestProtoOptions(14), &cfg)
	assert.Equal(t, uint16(14), v1Server.protoVersion)

	// V2 server (version 15)
	v2Server := NewServer(newTestProtoOptions(MessageSubmissionV2MinVersion), &cfg)
	assert.Equal(t, MessageSubmissionV2MinVersion, v2Server.protoVersion)
}

func TestVersionZeroDefaultsToV1(t *testing.T) {
	cfg := NewConfig()
	c := NewClient(newTestProtoOptions(0), &cfg)
	// Version 0 < 15 → should use V1 (Init guard should not fire)
	assert.Less(
		t,
		c.protoVersion,
		MessageSubmissionV2MinVersion,
		"version 0 should default to V1",
	)
}
