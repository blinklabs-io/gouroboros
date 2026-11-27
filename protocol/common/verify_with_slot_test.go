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

// hookStakeAuthority runs onLookup, once, during the first stake lookup: the
// point where verification runs with the authenticator's lock released.
type hookStakeAuthority struct {
	*stubStakeAuthority
	onLookup func()
}

func (h *hookStakeAuthority) PoolActiveStake(
	poolKeyHash PoolKeyHash,
) (uint64, error) {
	if hook := h.onLookup; hook != nil {
		h.onLookup = nil
		hook()
	}
	return h.stubStakeAuthority.PoolActiveStake(poolKeyHash)
}

// TestVerifyMessageWithSlotRefusedRecordsNoCounter: a message
// VerifyMessageWithSlot refuses as too frequent, because another message of its
// pool was accepted while it was being verified, records no opcert counter for
// the pool.
func TestVerifyMessageWithSlotRefusedRecordsNoCounter(t *testing.T) {
	stake := &hookStakeAuthority{stubStakeAuthority: newStubStakeAuthority()}
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		Now:            func() time.Time { return withSlotEpoch },
	})
	require.NoError(t, err)
	pool := newRecheckPool(t)
	poolID, err := poolKeyHash(pool.coldPub)
	require.NoError(t, err)
	stake.register(poolID, 1000)

	stake.onLookup = func() {
		require.NoError(t, auth.VerifyMessages([]DmqMessage{
			pool.message(t, 1, 0x01),
		}))
	}
	second := pool.message(t, 2, 0x02)
	err = auth.VerifyMessageWithSlot(&second, 0)
	require.ErrorIs(t, err, ErrMessageTooFrequent)
	require.Equal(t, uint64(1), auth.kesOpCertCache[poolID])
}

var withSlotEpoch = time.Unix(1_700_000_000, 0)
