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
	"net"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// newHeldMessageProtocol builds a three-state stand-in with a Busy/Idle pair
// like newPipelineRaceProtocol's, plus a Done state reachable only from Idle.
// This lets a test hold a message that fits neither the pipelined path (in
// Busy) nor an immediate ordinary send (it isn't legal from Busy at all) but
// does have a legal ordinary transition once real agency returns (from
// Idle). It also exposes the wire's raw byte traffic, unlike
// newPipelineRaceProtocol which only drains it, so a test can assert that
// nothing was written while the peer still holds agency.
func newHeldMessageProtocol(
	t *testing.T,
) (p *Protocol, errorChan chan error, wireChan chan []byte, busy, idle, done State) {
	t.Helper()
	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
	const msgTypeDone uint8 = 4
	busy = NewState(1, "Busy")
	idle = NewState(2, "Idle")
	done = NewState(3, "Done")
	stateMap := StateMap{
		busy: StateMapEntry{
			Agency:                AgencyServer,
			AllowPipelinedSend:    true,
			PipelinedMessageTypes: []uint8{msgTypeRequest},
			Transitions: []StateTransition{
				{MsgType: msgTypeBatchDone, NewState: idle},
			},
		},
		idle: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeRequest, NewState: busy},
				{MsgType: msgTypeDone, NewState: done},
			},
		},
		done: StateMapEntry{
			Agency: AgencyNone,
		},
	}

	localConn, peerConn := net.Pipe()
	t.Cleanup(func() {
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	m := muxer.New(localConn)
	m.Start()
	t.Cleanup(m.Stop)

	wireChan = make(chan []byte, 100)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := peerConn.Read(buf)
			if err != nil {
				return
			}
			got := make([]byte, n)
			copy(got, buf[:n])
			wireChan <- got
		}
	}()

	errorChan = make(chan error, 1)
	p = New(ProtocolConfig{
		Name:         "test-hold-for-agency",
		ErrorChan:    errorChan,
		Muxer:        m,
		Role:         ProtocolRoleClient,
		StateMap:     stateMap,
		InitialState: busy,
	})
	p.Start()
	t.Cleanup(p.Stop)
	return p, errorChan, wireChan, busy, idle, done
}

// TestSendLoopHoldsPipelinedDequeueUntilAgencyReturns reproduces
// blinklabs-io/gouroboros#2494's first defect in the fix for #2494 itself:
// resolvePipelinedDequeue can decide a dequeued message must wait for real
// agency (waitForAgency = true) because it fits neither the pipelined path
// nor an immediate ordinary send in the current state. sendLoop's top-level
// dispatch only waited for that condition on its *next* iteration; the
// iteration that just set it fell straight through into the
// read-send-queue section, which treats any non-nil pipelinedOutbound as
// ready to write regardless of waitForAgency. The message went out on the
// wire immediately, while the peer still held agency -- exactly the
// ordering violation the state check exists to prevent. In chainsync this
// let Stop's MsgDone go out while the server still held agency after a
// pipelined RequestNext.
func TestSendLoopHoldsPipelinedDequeueUntilAgencyReturns(t *testing.T) {
	t.Parallel()

	const msgTypeBatchDone uint8 = 3
	const msgTypeDone uint8 = 4
	p, errorChan, wireChan, busy, _, done := newHeldMessageProtocol(t)

	dequeued := make(chan struct{})
	proceed := make(chan struct{})
	p.pipelinedDequeueHook = func() {
		close(dequeued)
		<-proceed
	}

	sendResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sendResult <- p.SendMessageContextAndWait(
			ctx,
			&MessageBase{MessageType: msgTypeDone},
		)
	}()

	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("sendLoop never reached the pipelined dequeue check")
	}
	close(proceed)

	// The message must not be written to the wire, nor its delivery
	// completed, while the peer still holds agency in Busy. This is a
	// does-not-happen assertion: it gets safer under load, not flakier, so a
	// bounded wait is the right tool here rather than an unsynchronized poll.
	select {
	case got := <-wireChan:
		t.Fatalf(
			"message was written to the wire (%d bytes) while the peer "+
				"still held agency in %s",
			len(got),
			busy,
		)
	case err := <-sendResult:
		t.Fatalf(
			"SendMessageContextAndWait returned (err=%v) before agency "+
				"returned",
			err,
		)
	case <-time.After(300 * time.Millisecond):
	}
	require.Equal(
		t,
		busy,
		p.getCurrentState(),
		"state must not have moved on its own",
	)

	// Now grant real agency the way the peer's own terminal event would.
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)

	select {
	case err := <-sendResult:
		require.NoError(t, err, "the held message must send once agency returns")
	case <-time.After(5 * time.Second):
		t.Fatal("SendMessageContextAndWait never returned after agency returned")
	}
	select {
	case <-wireChan:
	case <-time.After(time.Second):
		t.Fatal("the held message was never actually written to the wire")
	}
	// The held message's own transition (Idle -> Done) applies as part of
	// sending it; sendResult and the wire write above already confirm that
	// happened, so by now the state may already be Done rather than the
	// intermediate Idle.
	require.Eventually(
		t,
		func() bool { return p.getCurrentState() == done },
		time.Second,
		time.Millisecond,
	)

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

// TestSendLoopDoesNotDropHeldMessageOnLaterDequeue reproduces
// blinklabs-io/gouroboros#2494's second defect: once sendLoop consumes the
// sendReadyChan token that grants a held pipelinedOutbound message real
// agency, it clears waitForAgency immediately, even though a
// queuedStateTransitions backlog from an earlier pipelined send may still be
// unapplied. Flushing that backlog can take another full round trip through
// the outer loop (the "Check for queued state transitions" continue). With
// the top-level dispatch keyed on waitForAgency alone, that next iteration
// no longer recognized the held message as pending and instead read a newly
// enqueued message from sendQueueChan, silently overwriting and dropping
// the held one.
//
// Req is sent first and pipelined while Busy, deferring its own transition
// (Idle -> Busy) into the backlog. Done is then queued behind it: Done does
// not fit pipelined in Busy but does have a legal ordinary transition from
// Idle, so sendLoop holds it (waitForAgency = true) rather than sending it or
// rejecting it. The first BatchDone grants real agency (Busy -> Idle),
// clearing Done's waitForAgency, but Req's own deferred transition
// (Idle -> Busy) must flush first, moving the state back to Busy -- a state
// where this role may again pipeline. A second Req sent at that point must
// not be read ahead of the still-held Done. A second BatchDone then grants
// agency again, with the backlog now empty, and only then must Done actually
// be delivered.
func TestSendLoopDoesNotDropHeldMessageOnLaterDequeue(t *testing.T) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
	const msgTypeDone uint8 = 4
	p, errorChan, wireChan, busy, _, done := newHeldMessageProtocol(t)

	dequeued := make(chan struct{})
	proceed := make(chan struct{})
	p.pipelinedDequeueHook = func() {
		close(dequeued)
		<-proceed
	}

	require.NoError(t, p.SendMessage(&MessageBase{MessageType: msgTypeRequest}))
	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("sendLoop never reached the first pipelined dequeue")
	}

	// Done does not fit pipelined in Busy but does have a legal ordinary
	// transition (from Idle), so it is held (waitForAgency = true) behind
	// Req's still-unapplied deferred transition.
	doneCtx, doneCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer doneCancel()
	doneResult := make(chan error, 1)
	go func() {
		doneResult <- p.SendMessageContextAndWait(
			doneCtx,
			&MessageBase{MessageType: msgTypeDone},
		)
	}()
	require.Eventually(
		t,
		func() bool { return len(p.sendQueueChan) == 1 },
		time.Second,
		time.Millisecond,
		"Done must be queued behind Req before Req's dequeue is released",
	)
	close(proceed)
	// Only Req's own dequeue should pause: a later dequeue in the buggy
	// version below (the second Req, sent further down) must not hit this
	// same hook and double-close dequeued.
	p.pipelinedDequeueHook = nil

	// Req's bytes only reach the wire after Done has already been examined
	// and held: the batch write happens once sendLoop breaks out of the
	// read-send-queue section, which only happens after Done's check sets
	// waitForAgency and pipelinedOutbound. Waiting for them is therefore a
	// deterministic proxy for "Done is now held", without touching sendLoop's
	// private state.
	select {
	case <-wireChan:
	case <-time.After(2 * time.Second):
		t.Fatal("Req was never written to the wire")
	}
	require.Equal(t, busy, p.getCurrentState())

	// Grant real agency: this clears Done's waitForAgency, but Req's own
	// deferred transition (Idle -> Busy) must flush before Done can be sent,
	// landing back in a state where this role may pipeline again.
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)
	require.Eventually(
		t,
		func() bool { return p.getCurrentState() == busy },
		time.Second,
		time.Millisecond,
		"Req's deferred transition must flush once agency returns",
	)

	// A later, unrelated message becomes eligible to pipeline now that state
	// is back in Busy. If the held Done message were dropped by this
	// dequeue, sendLoop would send this one in its place and never fulfill
	// doneResult; if Done survives, this message just sits queued until the
	// next real agency grant.
	require.NoError(t, p.SendMessage(&MessageBase{MessageType: msgTypeRequest}))

	select {
	case err := <-doneResult:
		t.Fatalf(
			"Done must still be waiting for agency, not already resolved "+
				"(err=%v)",
			err,
		)
	case <-time.After(200 * time.Millisecond):
	}

	// Grant agency a second time, with the backlog now empty: only now must
	// the held Done message actually be delivered.
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)

	select {
	case err := <-doneResult:
		require.NoError(
			t,
			err,
			"the held Done message must not be dropped by the later Req dequeue",
		)
	case <-time.After(3 * time.Second):
		t.Fatal("held Done was never delivered; it was silently dropped")
	}
	require.Eventually(
		t,
		func() bool { return p.getCurrentState() == done },
		time.Second,
		time.Millisecond,
	)

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
	require.False(t, p.IsStopping())
}

// TestSendLoopDefersBatchedMessageBehindUnflushedBacklog reproduces
// blinklabs-io/gouroboros#2494's third defect. Within a single batch pass,
// a message dequeued directly from sendQueueChan (not through
// resolvePipelinedDequeue) was allowed to have its transition applied
// immediately whenever the current state showed this role holding ordinary
// agency, with no check that queuedStateTransitions -- carrying an earlier
// message in the very same batch whose own transition is still deferred --
// was empty. A concurrent transition (e.g. a peer's own terminal event)
// landing between the two messages could make that check observe agency
// that has nothing to do with the backlog, applying the second message's
// transition out of order and leaving the first message's transition
// token/state pairing stranded.
//
// batchRecheckHook pins the goroutine schedule at the exact point the race
// needs; forcing the window with a sleep would only be probabilistic.
func TestSendLoopDefersBatchedMessageBehindUnflushedBacklog(t *testing.T) {
	t.Parallel()

	const msgTypeA uint8 = 2
	const msgTypeB uint8 = 3
	const msgTypeBatchDone uint8 = 4
	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
	mid := NewState(3, "Mid")
	doneState := NewState(4, "Done")
	stateMap := StateMap{
		busy: StateMapEntry{
			Agency:                AgencyServer,
			AllowPipelinedSend:    true,
			PipelinedMessageTypes: []uint8{msgTypeA, msgTypeB},
			Transitions: []StateTransition{
				{MsgType: msgTypeBatchDone, NewState: idle},
			},
		},
		idle: StateMapEntry{
			Agency: AgencyClient,
			// Deliberately no transition for msgTypeB: msgTypeB only becomes
			// legal from Mid, which msgTypeA's own deferred transition
			// produces. Applying msgTypeB's transition before msgTypeA's has
			// been applied is the out-of-order defect under test, and it is
			// illegal here by construction.
			Transitions: []StateTransition{
				{MsgType: msgTypeA, NewState: mid},
			},
		},
		mid: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeB, NewState: doneState},
			},
		},
		doneState: StateMapEntry{
			Agency: AgencyNone,
		},
	}

	localConn, peerConn := net.Pipe()
	t.Cleanup(func() {
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	m := muxer.New(localConn)
	m.Start()
	t.Cleanup(m.Stop)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := peerConn.Read(buf); err != nil {
				return
			}
		}
	}()

	errorChan := make(chan error, 1)
	p := New(ProtocolConfig{
		Name:         "test-batch-order",
		ErrorChan:    errorChan,
		Muxer:        m,
		Role:         ProtocolRoleClient,
		StateMap:     stateMap,
		InitialState: busy,
	})

	dequeued := make(chan struct{})
	proceed := make(chan struct{})
	p.pipelinedDequeueHook = func() {
		close(dequeued)
		<-proceed
	}

	hookCalled := false
	p.batchRecheckHook = func() {
		if hookCalled {
			return
		}
		hookCalled = true
		// Land the peer's own terminal event -- standing in for a real
		// concurrent stateLoop transition -- exactly between msgA's deferred
		// transition being appended and msgB's own recheck, then wait for it
		// to actually be visible before letting the recheck proceed.
		concurrentDone := make(chan struct{})
		go func() {
			defer close(concurrentDone)
			_ = p.transitionState(&MessageBase{MessageType: msgTypeBatchDone})
		}()
		deadline := time.Now().Add(2 * time.Second)
		for p.getCurrentState() != idle {
			if time.Now().After(deadline) {
				// This hook runs on the sendLoop goroutine, where
				// t.Fatal's runtime.Goexit would kill sendLoop instead of
				// the test.
				t.Error("concurrent BatchDone transition never became visible")
				return
			}
			time.Sleep(time.Millisecond)
		}
		<-concurrentDone
	}

	p.Start()
	t.Cleanup(p.Stop)

	require.NoError(t, p.SendMessage(&MessageBase{MessageType: msgTypeA}))
	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("sendLoop never reached the first pipelined dequeue")
	}
	// msgB must already be queued behind msgA before msgA's dequeue is
	// released, so both land in the same readSendQueueLoop batch pass.
	require.NoError(t, p.SendMessage(&MessageBase{MessageType: msgTypeB}))
	require.Eventually(
		t,
		func() bool { return len(p.sendQueueChan) == 1 },
		time.Second,
		time.Millisecond,
	)
	close(proceed)

	require.Eventually(
		t,
		func() bool { return p.getCurrentState() == doneState },
		time.Second,
		time.Millisecond,
		"both messages must flush in order: Idle -> Mid -> Done",
	)

	select {
	case err := <-errorChan:
		t.Fatalf(
			"unexpected protocol error (out-of-order transition applied): %v",
			err,
		)
	default:
	}
	require.False(t, p.IsStopping())
}

// TestSendLoopConsumesTokenWhenBatchedMessageBypassesDeferral batches two
// messages sent with ordinary agency, where the first lands in a state that
// also grants this role agency (Release: Acquired -> Idle) and the second is
// applied straight from there (Acquire: Idle -> Acquiring). The Idle token
// setState deposits must be consumed with the second transition; left in
// sendReadyChan it wakes sendLoop in Acquiring, where the peer holds agency,
// and the next message is sent and rejected there.
func TestSendLoopConsumesTokenWhenBatchedMessageBypassesDeferral(t *testing.T) {
	t.Parallel()

	const msgTypeRelease uint8 = 2
	const msgTypeAcquire uint8 = 3
	const msgTypeAcquired uint8 = 4
	acquired := NewState(1, "Acquired")
	idle := NewState(2, "Idle")
	acquiring := NewState(3, "Acquiring")
	stateMap := StateMap{
		acquired: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeRelease, NewState: idle},
			},
		},
		idle: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeAcquire, NewState: acquiring},
			},
		},
		acquiring: StateMapEntry{
			Agency: AgencyServer,
			Transitions: []StateTransition{
				{MsgType: msgTypeAcquired, NewState: acquired},
			},
		},
	}

	localConn, peerConn := net.Pipe()
	t.Cleanup(func() {
		_ = localConn.Close()
		_ = peerConn.Close()
	})
	m := muxer.New(localConn)
	m.Start()
	t.Cleanup(m.Stop)
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := peerConn.Read(buf); err != nil {
				return
			}
		}
	}()

	errorChan := make(chan error, 1)
	p := New(ProtocolConfig{
		Name:         "test-batch-bypass-token",
		ErrorChan:    errorChan,
		Muxer:        m,
		Role:         ProtocolRoleClient,
		StateMap:     stateMap,
		InitialState: acquiring,
	})
	p.Start()
	t.Cleanup(p.Stop)

	sendAndWait := func(msgType uint8) chan error {
		result := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			result <- p.SendMessageContextAndWait(
				ctx,
				&MessageBase{MessageType: msgType},
			)
		}()
		return result
	}

	// Queue both before agency arrives so they share one batch. The delivery
	// channel on Acquire ends that batch after it.
	require.NoError(t, p.SendMessage(&MessageBase{MessageType: msgTypeRelease}))
	acquireResult := sendAndWait(msgTypeAcquire)
	require.Eventually(
		t,
		func() bool { return len(p.sendQueueChan) == 2 },
		time.Second,
		time.Millisecond,
	)
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeAcquired}),
	)
	select {
	case err := <-acquireResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("batched Release/Acquire were never delivered")
	}
	require.Equal(t, acquiring, p.getCurrentState())

	// Release is only legal from Acquired, so it must wait for the peer.
	releaseResult := sendAndWait(msgTypeRelease)
	select {
	case err := <-errorChan:
		t.Fatalf("Release was sent while the peer held agency: %v", err)
	case err := <-releaseResult:
		t.Fatalf("Release returned (err=%v) before agency returned", err)
	case <-time.After(300 * time.Millisecond):
	}

	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeAcquired}),
	)
	select {
	case err := <-releaseResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Release was never delivered after agency returned")
	}
	require.Equal(t, idle, p.getCurrentState())
	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}
