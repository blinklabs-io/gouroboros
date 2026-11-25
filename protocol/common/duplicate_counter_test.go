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

package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestHeldMessageSkipsCounterCheck verifies a message already accepted again
// after its pool's opcert counter has moved on, as when a second peer relays
// it. dmq-node never validates a held message again: the mempool writer
// drops a message it already holds before validating the rest
// (ouroboros-network 4646039f Ouroboros/Network/TxSubmission/Mempool/
// Simple.hs 183-245), so the counter check (dmq-node a267003c14e9
// Validate.hs 183-197) never reaches it.
func TestHeldMessageSkipsCounterCheck(t *testing.T) {
	stake := newStubStakeAuthority()
	clock := &rateClock{t: time.Unix(1_000_000, 0)}
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		Now:            clock.now,
	})
	require.NoError(t, err)
	pool := newRecheckPool(t)
	id, err := poolKeyHash(pool.coldPub)
	require.NoError(t, err)
	stake.register(id, 1000)

	older := pool.message(t, 1, 1)
	require.NoError(t, verifyOne(auth, older))
	clock.t = clock.t.Add(time.Minute)
	require.NoError(t, verifyOne(auth, pool.message(t, 2, 2)))

	require.NoError(t, verifyOne(auth, older), "batch of one")
	errs, commit := auth.PrepareEach([]DmqMessage{older})
	require.NoError(t, errs[0], "on its own")
	require.NoError(t, commit())
	// The counter recorded for the pool is still the higher one.
	require.Error(t, verifyOne(auth, pool.message(t, 1, 3)))
}
