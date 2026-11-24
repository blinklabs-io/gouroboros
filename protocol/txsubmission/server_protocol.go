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

package txsubmission

import (
	"context"
	"log/slog"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// The server replaces its embedded protocol instance when it restarts after
// the client's done message. The methods below shadow the ones promoted from
// that embedded field, which would read it without protocolMu, so a caller's
// goroutine can use them while a restart runs on the receive loop.

// EnsureRegistered calls EnsureRegistered on the server's current protocol instance.
func (s *Server) EnsureRegistered() {
	s.ProtocolInstance().EnsureRegistered()
}

// SetIngressAllowance calls SetIngressAllowance on the server's current protocol instance.
func (s *Server) SetIngressAllowance(bytes int) {
	s.ProtocolInstance().SetIngressAllowance(bytes)
}

// SetIngressBackpressure calls SetIngressBackpressure on the server's current protocol instance.
func (s *Server) SetIngressBackpressure(enabled bool) {
	s.ProtocolInstance().SetIngressBackpressure(enabled)
}

// StopForRestart calls StopForRestart on the server's current protocol instance.
func (s *Server) StopForRestart() {
	s.ProtocolInstance().StopForRestart()
}

// CancelRestart calls CancelRestart on the server's current protocol instance.
func (s *Server) CancelRestart() {
	s.ProtocolInstance().CancelRestart()
}

// Logger calls Logger on the server's current protocol instance.
func (s *Server) Logger() *slog.Logger {
	return s.ProtocolInstance().Logger()
}

// Mode calls Mode on the server's current protocol instance.
func (s *Server) Mode() protocol.ProtocolMode {
	return s.ProtocolInstance().Mode()
}

// Role calls Role on the server's current protocol instance.
func (s *Server) Role() protocol.ProtocolRole {
	return s.ProtocolInstance().Role()
}

// DoneChan returns the done channel of the server's current protocol
// instance. It closes when that instance stops, including at a restart.
func (s *Server) DoneChan() <-chan struct{} {
	return s.ProtocolInstance().DoneChan()
}

// StopChan returns the stop channel of the server's current protocol
// instance. It closes when that instance is asked to stop, including at a
// restart.
func (s *Server) StopChan() <-chan struct{} {
	return s.ProtocolInstance().StopChan()
}

// IsDone calls IsDone on the server's current protocol instance.
func (s *Server) IsDone() bool {
	return s.ProtocolInstance().IsDone()
}

// IsStopping calls IsStopping on the server's current protocol instance.
func (s *Server) IsStopping() bool {
	return s.ProtocolInstance().IsStopping()
}

// IsInTerminalOrIdleState calls IsInTerminalOrIdleState on the server's current protocol instance.
func (s *Server) IsInTerminalOrIdleState() bool {
	return s.ProtocolInstance().IsInTerminalOrIdleState()
}

// WaitSendQueueDrained calls WaitSendQueueDrained on the server's current protocol instance.
func (s *Server) WaitSendQueueDrained(timeout time.Duration) bool {
	return s.ProtocolInstance().WaitSendQueueDrained(timeout)
}

// SendMessage calls SendMessage on the server's current protocol instance.
func (s *Server) SendMessage(msg protocol.Message) error {
	return s.ProtocolInstance().SendMessage(msg)
}

// SendMessageContext calls SendMessageContext on the server's current protocol instance.
func (s *Server) SendMessageContext(ctx context.Context, msg protocol.Message) error {
	return s.ProtocolInstance().SendMessageContext(ctx, msg)
}

// SendMessageAndWait calls SendMessageAndWait on the server's current protocol instance.
func (s *Server) SendMessageAndWait(msg protocol.Message) error {
	return s.ProtocolInstance().SendMessageAndWait(msg)
}

// SendMessageContextAndWait calls SendMessageContextAndWait on the server's current protocol instance.
func (s *Server) SendMessageContextAndWait(ctx context.Context, msg protocol.Message) error {
	return s.ProtocolInstance().SendMessageContextAndWait(ctx, msg)
}

// SendError calls SendError on the server's current protocol instance.
func (s *Server) SendError(err error) {
	s.ProtocolInstance().SendError(err)
}

// RunLoop calls RunLoop on the server's current protocol instance.
func (s *Server) RunLoop(where string, loop func()) {
	s.ProtocolInstance().RunLoop(where, loop)
}
