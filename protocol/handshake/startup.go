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

package handshake

import (
	"github.com/blinklabs-io/gouroboros/muxer"
)

// resumeWhileUnfinished returns the callback a handshake protocol runs when it
// needs another segment to complete a message. It lets a delayed-start muxer
// read one more segment only while the handshake is unfinished: once the
// protocol is done, leftover bytes behind the final message stay incomplete
// for the rest of the connection and must not keep the muxer reading before
// Start. It returns nil when there is no muxer.
func resumeWhileUnfinished(m *muxer.Muxer, done func() bool) func() {
	if m == nil {
		return nil
	}
	return func() {
		if !done() {
			m.StartOnce()
		}
	}
}
