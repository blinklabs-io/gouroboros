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

package localmessagenotification

import (
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestServerStopBeforeStartCompletesLifecycle(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)

	require.NoError(t, server.Stop())
	select {
	case <-server.DoneChan():
	default:
		t.Fatal("DoneChan was not closed after stopping before Start")
	}

	require.NoError(t, server.Stop())
	require.NotPanics(t, server.Start)
	select {
	case <-server.DoneChan():
	default:
		t.Fatal("Start restarted a stopped protocol")
	}
}

func TestServerRepeatedStopAfterStartCompletesLifecycle(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() {
		_ = remoteConn.Close()
	})
	protocolMuxer := muxer.New(localConn)
	t.Cleanup(protocolMuxer.Stop)
	server := NewServer(
		protocol.ProtocolOptions{Muxer: protocolMuxer},
		nil,
	)
	server.Start()

	require.NoError(t, server.Stop())
	require.NoError(t, server.Stop())
	select {
	case <-server.DoneChan():
	case <-time.After(time.Second):
		t.Fatal("started server did not complete shutdown")
	}
}

func TestServerStopUnblocksWaitingRequest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server := NewServer(protocol.ProtocolOptions{}, nil)
		result := make(chan error, 1)
		go func() { result <- server.WaitForMessage(0) }()

		// Wait until WaitForMessage is durably blocked in its select before
		// stopping the server.
		synctest.Wait()
		require.NoError(t, server.Stop())
		select {
		case err := <-result:
			require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
		case <-time.After(time.Second):
			t.Fatal("waiting request was not cancelled")
		}
	})
}

func TestServerStopStopsExpirationCleanerBeforeStart(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)
	require.NoError(t, server.Stop())

	select {
	case <-server.expirationStopChan:
	default:
		t.Fatal("expiration cleaner was not stopped")
	}
	select {
	case <-server.expirationDoneChan:
	case <-time.After(time.Second):
		t.Fatal("expiration cleaner did not exit")
	}
}

func TestServerStopJoinsExpirationCleaner(t *testing.T) {
	server := NewServer(protocol.ProtocolOptions{}, nil)
	require.NoError(t, server.Stop())

	select {
	case <-server.expirationDoneChan:
	default:
		t.Fatal("Stop returned before the expiration cleaner exited")
	}
}

func TestServerQueueFullDoesNotCommitHigherOpCert(t *testing.T) {
	expiresAt := uint32(time.Now().Add(time.Minute).Unix())
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"first", "higher", "later"},
		[]uint64{1, 2, 1},
		[]uint32{expiresAt, expiresAt, expiresAt},
	)
	cfg := NewConfig(
		WithAuthenticator(authenticator),
		WithMaxQueueSize(1),
	)
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	t.Cleanup(func() { require.NoError(t, server.Stop()) })

	require.NoError(t, server.AddMessage(&messages[0]))
	require.ErrorContains(t, server.AddMessage(&messages[1]), "queue full")
	server.lock.Lock()
	server.messageQueue = server.messageQueue[:0]
	server.lock.Unlock()
	require.NoError(t, server.AddMessage(&messages[2]))
}

func TestServerDuplicateDoesNotCommitHigherOpCert(t *testing.T) {
	expiresAt := uint32(time.Now().Add(time.Minute).Unix())
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"same", "same", "later"},
		[]uint64{1, 2, 1},
		[]uint32{expiresAt, expiresAt, expiresAt},
	)
	cfg := NewConfig(WithAuthenticator(authenticator))
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	t.Cleanup(func() { require.NoError(t, server.Stop()) })

	require.NoError(t, server.AddMessage(&messages[0]))
	server.lock.Lock()
	drained := server.drainValidMessagesLocked(time.Now())
	server.lock.Unlock()
	require.Len(t, drained, 1)
	require.ErrorContains(
		t,
		server.AddMessage(&messages[1]),
		"already acknowledged",
	)
	require.NoError(t, server.AddMessage(&messages[2]))
}

func TestServerRejectsQueuedDuplicate(t *testing.T) {
	expiresAt := uint32(time.Now().Add(time.Minute).Unix())
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"same", "same", "later"},
		[]uint64{1, 2, 1},
		[]uint32{expiresAt, expiresAt, expiresAt},
	)
	cfg := NewConfig(WithAuthenticator(authenticator))
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	t.Cleanup(func() { require.NoError(t, server.Stop()) })

	require.NoError(t, server.AddMessage(&messages[0]))
	require.ErrorContains(t, server.AddMessage(&messages[1]), "already queued")
	require.NoError(t, server.AddMessage(&messages[2]))
	server.lock.Lock()
	drained := server.drainValidMessagesLocked(time.Now())
	server.lock.Unlock()
	require.Len(t, drained, 2)
}

func TestServerOwnsAuthenticatedQueuedMessage(t *testing.T) {
	expiresAt := uint32(time.Now().Add(time.Minute).Unix())
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"owned"},
		[]uint64{1},
		[]uint32{expiresAt},
	)
	cfg := NewConfig(WithAuthenticator(authenticator))
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	t.Cleanup(func() { require.NoError(t, server.Stop()) })
	expected := cloneDmqMessage(messages[0])

	require.NoError(t, server.AddMessage(&messages[0]))
	messages[0].MessageID[0] ^= 0xff
	messages[0].Payload.MessageID[0] ^= 0xff
	messages[0].Payload.MessageBody[0] ^= 0xff
	messages[0].KESSignature[0] ^= 0xff
	messages[0].OperationalCertificate.KESVerificationKey[0] ^= 0xff
	messages[0].OperationalCertificate.ColdSignature[0] ^= 0xff
	messages[0].ColdVerificationKey[0] ^= 0xff

	server.lock.Lock()
	drained := server.drainValidMessagesLocked(time.Now())
	server.lock.Unlock()
	require.Equal(t, []pcommon.DmqMessage{expected}, drained)
}

func TestServerExpiryDuringAuthenticationDoesNotCommitHigherOpCert(t *testing.T) {
	initial := time.Unix(1_000, 0)
	messages, authenticator := clientTestSignedMessages(
		t,
		[]string{"expiring", "later"},
		[]uint64{2, 1},
		[]uint32{uint32(initial.Unix()), uint32(initial.Add(time.Minute).Unix())},
	)
	cfg := NewConfig(WithAuthenticator(authenticator))
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	t.Cleanup(func() { require.NoError(t, server.Stop()) })
	times := []time.Time{
		initial,
		initial.Add(time.Second),
		initial.Add(time.Second),
		initial.Add(time.Second),
	}
	server.now = func() time.Time {
		now := times[0]
		times = times[1:]
		return now
	}

	require.ErrorContains(t, server.AddMessage(&messages[0]), "expired")
	require.NoError(t, server.AddMessage(&messages[1]))
}

func TestServerRequiresTTLValidator(t *testing.T) {
	cfg := Config{Authenticator: pcommon.NewNoOpAuthenticator(nil)}
	server := NewServer(protocol.ProtocolOptions{}, &cfg)
	t.Cleanup(func() { require.NoError(t, server.Stop()) })

	err := server.AddMessage(&pcommon.DmqMessage{})
	require.ErrorContains(t, err, "TTL validator not configured")
}

func TestConnectionDoneCancelsBlockingRequestAndCleaner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		connectionDone := make(chan any)
		server := NewServer(protocol.ProtocolOptions{
			ConnectionDoneChan: connectionDone,
		}, nil)
		result := make(chan error, 1)
		go func() { result <- server.handleBlockingRequest() }()

		synctest.Wait()
		close(connectionDone)
		select {
		case err := <-result:
			require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
		case <-time.After(time.Second):
			t.Fatal("blocking request did not observe connection shutdown")
		}
		select {
		case <-server.expirationDoneChan:
		case <-time.After(time.Second):
			t.Fatal("expiration cleaner did not observe connection shutdown")
		}
	})
}

func TestMuxerDoneCancelsStandaloneBlockingRequestAndCleaner(t *testing.T) {
	localConn, remoteConn := net.Pipe()
	t.Cleanup(func() { _ = remoteConn.Close() })
	protocolMuxer := muxer.New(localConn)
	server := NewServer(protocol.ProtocolOptions{Muxer: protocolMuxer}, nil)
	result := make(chan error, 1)
	go func() { result <- server.handleBlockingRequest() }()

	protocolMuxer.Stop()
	select {
	case err := <-result:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("blocking request did not observe muxer shutdown")
	}
	select {
	case <-server.expirationDoneChan:
	case <-time.After(time.Second):
		t.Fatal("expiration cleaner did not observe muxer shutdown")
	}
}
