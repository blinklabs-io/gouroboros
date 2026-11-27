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
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestPrepareCommitDuplicateAfterCounterRises: a message another connection
// accepted between this connection's prepare and commit is a duplicate at
// commit, and is not refused for its counter even after a later message raised
// the pool's counter. dmq-node drops a message it already holds before
// validating it.
func TestPrepareCommitDuplicateAfterCounterRises(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		MinSigDelay:    -1,
	})
	require.NoError(t, err)
	pool := newRecheckPool(t)
	poolID, err := poolKeyHash(pool.coldPub)
	require.NoError(t, err)
	stake.register(poolID, 1000)
	first := pool.message(t, 1, 0x01)

	commitAll, err := auth.PrepareMessages([]DmqMessage{first})
	require.NoError(t, err)
	errs, commitEach := auth.PrepareEach([]DmqMessage{first})
	require.NoError(t, errs[0])

	// Another connection accepts the same message, then the pool's next
	// one under a higher counter.
	require.NoError(t, auth.VerifyMessages([]DmqMessage{first}))
	require.NoError(t, auth.VerifyMessages([]DmqMessage{
		pool.message(t, 2, 0x02),
	}))

	require.NoError(t, commitAll())
	require.NoError(t, commitEach()[0])
	require.Equal(t, uint64(2), auth.kesOpCertCache[poolID])
}

// TestPrepareCommitTransitions: each row prepares one message of a pool, lets
// another connection act between prepare and commit, and checks the commit's
// verdict and the recorded counter. The recorded counter never goes backwards,
// a duplicate is never refused, and the recorded counter is the highest
// counter of an accepted message.
func TestPrepareCommitTransitions(t *testing.T) {
	tests := []struct {
		name string
		// issue is the prepared message's counter.
		issue uint64
		// held: the prepared message was accepted before the prepare.
		held bool
		// between runs on another connection between prepare and commit.
		between func(t *testing.T, a *MessageAuthenticator, p *recheckPool, m DmqMessage)
		// advance moves the clock after the prepare, before between.
		advance   time.Duration
		wantErr   string
		wantCache uint64
	}{
		{
			name:      "fresh message records its counter",
			issue:     2,
			wantCache: 2,
		},
		{
			name:  "counter below one recorded meanwhile is refused",
			issue: 1,
			between: func(t *testing.T, a *MessageAuthenticator, p *recheckPool, _ DmqMessage) {
				require.NoError(
					t,
					a.VerifyMessages([]DmqMessage{p.message(t, 2, 0x22)}),
				)
			},
			advance:   2 * time.Minute,
			wantErr:   "went backwards",
			wantCache: 2,
		},
		{
			name:  "duplicate accepted meanwhile under a lower counter is not refused",
			issue: 1,
			between: func(t *testing.T, a *MessageAuthenticator, p *recheckPool, m DmqMessage) {
				require.NoError(t, a.VerifyMessages([]DmqMessage{m}))
				a.now = func() time.Time { return recheckEpoch.Add(4 * time.Minute) }
				require.NoError(
					t,
					a.VerifyMessages([]DmqMessage{p.message(t, 2, 0x22)}),
				)
			},
			wantCache: 2,
		},
		{
			name:  "another message of the pool accepted meanwhile is too frequent",
			issue: 2,
			between: func(t *testing.T, a *MessageAuthenticator, p *recheckPool, _ DmqMessage) {
				require.NoError(
					t,
					a.VerifyMessages([]DmqMessage{p.message(t, 2, 0x22)}),
				)
			},
			wantErr:   "too soon",
			wantCache: 2,
		},
		{
			name:      "message held before the prepare records nothing",
			issue:     1,
			held:      true,
			wantCache: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stake := newStubStakeAuthority()
			auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
				StakeAuthority: stake,
				Now:            func() time.Time { return recheckEpoch },
			})
			require.NoError(t, err)
			pool := newRecheckPool(t)
			poolID, err := poolKeyHash(pool.coldPub)
			require.NoError(t, err)
			stake.register(poolID, 1000)
			msg := pool.message(t, tc.issue, 0x11)
			if tc.held {
				require.NoError(t, auth.VerifyMessages([]DmqMessage{msg}))
			}
			commit, err := auth.PrepareMessages([]DmqMessage{msg})
			require.NoError(t, err)
			if tc.advance > 0 {
				auth.now = func() time.Time { return recheckEpoch.Add(tc.advance) }
			}
			if tc.between != nil {
				tc.between(t, auth, pool, msg)
			}
			err = commit()
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Equal(t, tc.wantCache, auth.kesOpCertCache[poolID])
		})
	}
}

var recheckEpoch = time.Unix(1_700_000_000, 0)

// TestConcurrentCommitsKeepCounterInvariants prepares and commits two messages
// of one pool, under counters 1 and 2, on many goroutines at once, and checks
// the outcome: once the counter-1 message is accepted no commit of it is
// refused, and the recorded counter is the highest counter of an accepted
// message. Run with -race.
func TestConcurrentCommitsKeepCounterInvariants(t *testing.T) {
	stake := newStubStakeAuthority()
	auth, err := NewMessageAuthenticator(MessageAuthenticatorConfig{
		StakeAuthority: stake,
		MinSigDelay:    -1,
	})
	require.NoError(t, err)
	pool := newRecheckPool(t)
	poolID, err := poolKeyHash(pool.coldPub)
	require.NoError(t, err)
	stake.register(poolID, 1000)
	msgs := []DmqMessage{pool.message(t, 1, 0x01), pool.message(t, 2, 0x02)}

	const workers = 16
	const rounds = 20
	refusedFirst := make(chan error, workers*rounds)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			<-start
			for r := range rounds {
				msg := msgs[(w+r)%2]
				errs, commit := auth.PrepareEach([]DmqMessage{msg})
				if errs[0] != nil {
					continue
				}
				runtime.Gosched()
				if err := commit()[0]; err != nil &&
					msg.OperationalCertificate.IssueNumber == 1 {
					refusedFirst <- err
				}
			}
		})
	}
	close(start)
	wg.Wait()
	close(refusedFirst)

	auth.mu.Lock()
	defer auth.mu.Unlock()
	_, firstAccepted := auth.acceptedIDs[string(msgs[0].ID())]
	_, secondAccepted := auth.acceptedIDs[string(msgs[1].ID())]
	if firstAccepted {
		for err := range refusedFirst {
			t.Errorf("counter-1 message refused although accepted: %v", err)
		}
	}
	want := uint64(0)
	if firstAccepted {
		want = 1
	}
	if secondAccepted {
		want = 2
	}
	require.Equal(t, want, auth.kesOpCertCache[poolID])
}
