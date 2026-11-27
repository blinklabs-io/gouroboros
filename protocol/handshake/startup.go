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
	"sync/atomic"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol"
)

// startupCallbacks returns the MessageReceivedFunc and MessageIncompleteFunc
// that let a delayed-start muxer read the segments of the one message a
// handshake role receives, and nothing behind it. Before Start the muxer
// reads one segment per StartOnce, so a message split across segments asks
// for each next one when reassembly comes up short. Each role receives
// exactly one handshake message; bytes behind it never complete a valid one
// and must not keep the muxer reading before Start.
//
// The protocol's Done state cannot gate this: the state loop applies it after
// the read loop has already scanned the bytes behind the final message. Both
// callbacks run on the read loop, which decodes a message before scanning
// what follows it, so every scan after the message sees received set. It
// returns nil callbacks when there is no muxer.
func startupCallbacks(m *muxer.Muxer) (func(protocol.Message), func()) {
	if m == nil {
		return nil, nil
	}
	var received atomic.Bool
	return func(protocol.Message) {
			received.Store(true)
		}, func() {
			if !received.Load() {
				m.StartOnce()
			}
		}
}
