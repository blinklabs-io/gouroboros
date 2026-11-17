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

package byron

import (
	"math/rand"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// referencePBFTWindow is the window rule applied by copying the whole window
// on every step.
type referencePBFTWindow struct {
	history       []common.Blake2b224
	securityParam int
	maxSignatures int
}

func (r referencePBFTWindow) transition(
	issuer common.Blake2b224,
	enforce bool,
) (referencePBFTWindow, bool) {
	next := r.history
	if len(next) >= r.securityParam {
		next = next[1:]
	}
	next = append(append([]common.Blake2b224(nil), next...), issuer)
	if enforce {
		count := 0
		for _, historicalIssuer := range next {
			if historicalIssuer == issuer {
				count++
			}
		}
		if count > r.maxSignatures {
			return r, false
		}
	}
	r.history = next
	return r, true
}

// TestPBFTStateMatchesCopyingWindow checks the shared-log window against the
// copy-per-step rule across rollover, rejected issuers, and branching from
// older states, and that no step changes a state already handed out.
func TestPBFTStateMatchesCopyingWindow(t *testing.T) {
	t.Parallel()
	const securityParam = 7
	issuers := make([]common.Blake2b224, 4)
	for i := range issuers {
		issuers[i] = common.Blake2b224Hash([]byte{byte(i)})
	}
	maxSignatures := int(
		pbftMaxSignatures(
			securityParam,
			DefaultPBFTSignatureThresholdNumerator,
			DefaultPBFTSignatureThresholdDenominator,
		),
	)
	start, err := NewPBFTState(nil, securityParam)
	require.NoError(t, err)

	type branch struct {
		state PBFTState
		ref   referencePBFTWindow
	}
	branches := []branch{{
		state: start,
		ref: referencePBFTWindow{
			securityParam: securityParam,
			maxSignatures: maxSignatures,
		},
	}}
	rng := rand.New(rand.NewSource(1))
	for step := range 4000 {
		// Mostly extend the newest state, as block application does, and
		// sometimes an older one, as a rollback does.
		from := len(branches) - 1
		if rng.Intn(10) == 0 {
			from = rng.Intn(len(branches))
		}
		issuer := issuers[rng.Intn(len(issuers))]
		enforce := rng.Intn(4) != 0
		parent := branches[from]
		var next PBFTState
		if enforce {
			next, err = parent.state.Transition(issuer)
		} else {
			next, err = parent.state.Observe(issuer)
		}
		wantRef, wantOK := parent.ref.transition(issuer, enforce)
		require.Equal(t, wantOK, err == nil, "step %d", step)
		// A rejected step must leave the parent usable.
		require.Equal(
			t, parent.ref.history, historyOrNil(parent.state), "step %d", step,
		)
		if !wantOK {
			continue
		}
		require.Equal(t, wantRef.history, historyOrNil(next), "step %d", step)
		branches = append(branches, branch{state: next, ref: wantRef})
		if len(branches) > 64 {
			branches = branches[len(branches)-32:]
		}
	}
	for i, b := range branches {
		require.Equal(t, b.ref.history, historyOrNil(b.state), "branch %d", i)
	}
}

// TestPBFTStateSiblingsAreSafeToUseConcurrently extends one state from many
// goroutines at once. Siblings share a log, so each must read only its own
// bounds without the log's lock. Run with -race.
func TestPBFTStateSiblingsAreSafeToUseConcurrently(t *testing.T) {
	t.Parallel()
	const securityParam = 5
	issuers := make([]common.Blake2b224, 3)
	for i := range issuers {
		issuers[i] = common.Blake2b224Hash([]byte{byte(i)})
	}
	state, err := NewPBFTStateWithThreshold(nil, securityParam, 1, 1)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for worker := range 8 {
		wg.Go(func() {
			cur := state
			for i := range 200 {
				issuer := issuers[(worker+i)%len(issuers)]
				var next PBFTState
				var err error
				if i%2 == 0 {
					next, err = cur.Transition(issuer)
				} else {
					next, err = cur.Observe(issuer)
				}
				if !assert.NoError(t, err) {
					return
				}
				assert.Equal(
					t, issuer, next.SignatureHistory()[len(next.SignatureHistory())-1],
				)
				cur = next
			}
		})
	}
	wg.Wait()
}

func historyOrNil(s PBFTState) []common.Blake2b224 {
	history := s.SignatureHistory()
	if len(history) == 0 {
		return nil
	}
	return history
}

func BenchmarkPBFTStateTransition(b *testing.B) {
	const securityParam = 2160
	issuers := make([]common.Blake2b224, 7)
	for i := range issuers {
		issuers[i] = common.Blake2b224Hash([]byte{byte(i)})
	}
	state, err := NewPBFTStateWithThreshold(nil, securityParam, 1, 1)
	require.NoError(b, err)
	for i := range securityParam {
		state, err = state.Observe(issuers[i%len(issuers)])
		require.NoError(b, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		state, err = state.Transition(issuers[i%len(issuers)])
		if err != nil {
			b.Fatal(err)
		}
	}
}
