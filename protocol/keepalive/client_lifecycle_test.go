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

package keepalive

import (
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
)

func TestSendKeepAliveDoesNotRearmDuringShutdown(t *testing.T) {
	conn, peer := net.Pipe()
	m := muxer.New(conn)
	m.Start()
	t.Cleanup(func() {
		m.Stop()
		_ = peer.Close()
	})
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  conn.LocalAddr(),
			RemoteAddr: conn.RemoteAddr(),
		},
		Logger: slog.Default(),
		Muxer:  m,
	}, &Config{Period: time.Hour})
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	client.scheduleHook = func() {
		close(entered)
		<-release
	}
	done := make(chan struct{})
	go func() {
		client.Start()
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("keepalive callback did not reach scheduling point")
	}
	stopDone := make(chan struct{})
	go func() {
		client.Protocol.Stop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		if !client.IsStopping() {
			t.Fatal("shutdown did not begin while callback was paused")
		}
	}
	releaseOnce.Do(func() { close(release) })
	_ = peer.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepalive callback did not finish")
	}
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("protocol shutdown did not complete")
	}
	client.timerMutex.Lock()
	timer := client.timer
	client.timerMutex.Unlock()
	if timer != nil {
		timer.Stop()
		t.Fatal("keepalive callback re-armed timer during shutdown")
	}
}

func TestStartTimerArmsWhileProtocolIsActive(t *testing.T) {
	client := NewClient(protocol.ProtocolOptions{}, &Config{Period: time.Hour})
	client.startTimer()
	if client.timer == nil {
		t.Fatal("startTimer did not arm an active client")
	}
	client.timer.Stop()
}

func TestIsStoppingTracksShutdownRequest(t *testing.T) {
	client := NewClient(protocol.ProtocolOptions{}, &Config{Period: time.Hour})
	if client.IsStopping() {
		t.Fatal("new protocol reports shutdown in progress")
	}
	client.Protocol.Stop()
	if !client.IsStopping() {
		t.Fatal("protocol did not report its shutdown request")
	}
	select {
	case <-client.DoneChan():
	default:
		t.Fatal("protocol shutdown request did not complete")
	}
}
