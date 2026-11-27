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

package protocol

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestWaitForStateExitDuringStartDoesNotRace waits for a state exit while
// another goroutine starts the protocol, which registers it with the muxer;
// run it with -race.
func TestWaitForStateExitDuringStartDoesNotRace(t *testing.T) {
	p, peer := newSendBeforeStartProtocol(t)
	go func() { _, _ = io.Copy(io.Discard, peer) }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	waited := make(chan error, 1)
	go func() { waited <- p.WaitForStateExit(ctx, p.config.InitialState) }()
	p.Start()
	require.NoError(t, p.SendMessageContextAndWait(ctx, &MessageBase{MessageType: 1}))
	require.NoError(t, <-waited)
}
