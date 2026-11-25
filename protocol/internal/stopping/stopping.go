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

// Package stopping holds the stop sequence shared by mini-protocol clients
// that end their run by sending a terminating message.
package stopping

import (
	"context"
	"time"

	"github.com/blinklabs-io/gouroboros/protocol"
)

// DoneTimeout bounds how long SendDone waits for the terminating message to
// be written.
const DoneTimeout = 250 * time.Millisecond

// SendDone queues msg, the message that ends the client's run, and waits until
// it is written, DoneTimeout passes or ctx is done. Stopping the protocol
// while the message is still queued would drop it.
func SendDone(ctx context.Context, p *protocol.Protocol, msg protocol.Message) error {
	sendCtx, cancel := context.WithTimeout(ctx, DoneTimeout)
	defer cancel()
	return p.SendMessageContextAndWait(sendCtx, msg)
}
