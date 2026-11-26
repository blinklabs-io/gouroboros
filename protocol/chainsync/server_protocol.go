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

package chainsync

// Start starts the server's current protocol instance.
func (s *Server) Start() {
	s.ProtocolInstance().Start()
}

// EnsureRegistered registers the server's current protocol instance with the
// muxer if it is not registered yet.
func (s *Server) EnsureRegistered() {
	s.ProtocolInstance().EnsureRegistered()
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

// IsDone reports whether the server's current protocol instance is done.
func (s *Server) IsDone() bool {
	return s.ProtocolInstance().IsDone()
}
