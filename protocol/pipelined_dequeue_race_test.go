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
	"runtime"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// TestSendLoopPromotesRacedPipelinedMessageInsteadOfRejecting reproduces
// blinklabs-io/gouroboros#2494. sendLoop reads the current protocol state
// twice: once in pipelinedSendAllowed() at the top of the loop, and again
// after a message is dequeued, to check its type against the (re-read)
// state's PipelinedMessageTypes. A concurrent stateLoop transition -- driven
// in production by the peer's own event, e.g. block-fetch's BatchDone moving
// Busy/Streaming to Idle -- can land between those two reads. The message
// was validly queued while pipelining was eligible, and remains perfectly
// valid to send the ordinary way in the new state, but the unconditional
// dequeue-time rejection tore the connection down anyway.
//
// This test builds a two-state stand-in for block-fetch's Busy/Idle pair
// (protocol/blockfetch can't be imported here without a cycle, since it
// imports this package). pipelinedDequeueHook pins the goroutine schedule at
// the exact point the race needs; forcing the window with a sleep would only
// be probabilistic, and it is the reproduction the issue's third probe used.
func TestSendLoopPromotesRacedPipelinedMessageInsteadOfRejecting(t *testing.T) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3

	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
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
	// Drain whatever sendLoop writes so its segment write never blocks on
	// an unread net.Pipe; this test only cares about state and errors.
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
		Name:         "test-pipeline-race",
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

	p.Start()
	t.Cleanup(p.Stop)

	sendResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		sendResult <- p.SendMessageContextAndWait(
			ctx,
			&MessageBase{MessageType: msgTypeRequest},
		)
	}()

	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("sendLoop never reached the pipelined dequeue check")
	}

	// Simulate the peer's own terminal event (block-fetch's BatchDone)
	// landing while the message dequeued above is still mid-check -- exactly
	// the window between pipelinedSendAllowed()'s read and the per-message
	// recheck.
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)
	close(proceed)

	select {
	case err := <-sendResult:
		require.NoError(
			t,
			err,
			"a message validly queued while pipelining was eligible must "+
				"still be sent once state has legitimately advanced, not "+
				"rejected as a protocol violation",
		)
	case <-time.After(5 * time.Second):
		t.Fatal("SendMessageContextAndWait never returned")
	}

	require.False(
		t,
		p.IsStopping(),
		"a racily-dequeued pipelined message must not tear down the connection",
	)
	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}

	require.Equal(
		t,
		busy,
		p.getCurrentState(),
		"the promoted RequestRange-equivalent message must still drive "+
			"Idle -> Busy like an ordinary send",
	)
}

// newPipelineRaceProtocol builds the same two-state Busy/Idle stand-in used
// by TestSendLoopPromotesRacedPipelinedMessageInsteadOfRejecting, factored
// out for reuse by the Defect A and Defect B regression tests below.
func newPipelineRaceProtocol(
	t *testing.T,
	busy, idle State,
) (p *Protocol, errorChan chan error) {
	t.Helper()
	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
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

	errorChan = make(chan error, 1)
	p = New(ProtocolConfig{
		Name:         "test-pipeline-race",
		ErrorChan:    errorChan,
		Muxer:        m,
		Role:         ProtocolRoleClient,
		StateMap:     stateMap,
		InitialState: busy,
	})
	p.Start()
	t.Cleanup(p.Stop)
	return p, errorChan
}

// TestSendLoopConsumesAgencyTokenOnRacedPromotion asserts that promoting a
// racily-dequeued pipelined message consumes the sendReadyChan token that
// stateLoop's setState put there when it transitioned into the state that
// granted the promotion. An unconsumed token would be read by the next
// iteration of sendLoop's outer loop as a fresh agency grant for whatever
// state came next -- here, Busy, where the client has no ordinary agency at
// all -- and the following ordinary pipelined message would be sent as
// though the client held agency, fail its own state-transition check, and
// tear the connection down via SendError.
//
// A message racily promoted must consume the token itself, so a second,
// entirely unrelated, ordinary pipelined message sent afterward is
// unaffected.
func TestSendLoopConsumesAgencyTokenOnRacedPromotion(t *testing.T) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
	busy, idle := NewState(1, "Busy"), NewState(2, "Idle")
	p, errorChan := newPipelineRaceProtocol(t, busy, idle)

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
			&MessageBase{MessageType: msgTypeRequest},
		)
	}()

	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("sendLoop never reached the pipelined dequeue check")
	}

	// Land the peer's own terminal event while the first message is
	// mid-check, exactly as in the base race test: this is what grants the
	// racing promotion and is what produces the sendReadyChan token that
	// must be consumed.
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)
	close(proceed)

	select {
	case err := <-sendResult:
		require.NoError(t, err, "the racily promoted message must still send")
	case <-time.After(5 * time.Second):
		t.Fatal("first SendMessageContextAndWait never returned")
	}

	// Drop the hook: only the first message's dequeue should race. Wait
	// until the agency token the race produced has actually been drained --
	// by the fix itself if it consumes it synchronously, or by sendLoop's
	// own next iteration if a stale token is left behind (the defect this
	// test targets) -- before sending the second message. Sending it too
	// early would let it race the stale token for the same select instead
	// of deterministically colliding with it.
	p.pipelinedDequeueHook = nil
	deadline := time.Now().Add(3 * time.Second)
	for len(p.sendReadyChan) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("sendReadyChan token was never drained")
		}
		time.Sleep(time.Millisecond)
	}

	secondResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		secondResult <- p.SendMessageContextAndWait(
			ctx,
			&MessageBase{MessageType: msgTypeRequest},
		)
	}()

	select {
	case err := <-secondResult:
		require.NoError(
			t,
			err,
			"an ordinary pipelined message sent after a racily promoted one "+
				"must not be killed by a stale leftover agency token",
		)
	case <-time.After(5 * time.Second):
		t.Fatal("second SendMessageContextAndWait never returned")
	}

	require.False(
		t,
		p.IsStopping(),
		"a stale agency token must not tear down the connection on the "+
			"following ordinary pipelined message",
	)
	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

// TestSendLoopFlushesPartialBacklogOnRacedPromotion asserts that a
// racing message's legality is checked against the state a partial flush
// actually reaches, not the state resolvePipelinedDequeue started in. Each
// deferred transition needs its own real round trip with the peer, so once
// more than one pipelined message is queued ahead of a race, the backlog
// can flush only as far as the single concurrent transition that landed
// justifies; the remainder must stay queued rather than being applied as an
// ordinary transition in a state where it is only ever legal pipelined, and
// a flush that stops there for that reason is not a fatal error.
func TestSendLoopFlushesPartialBacklogOnRacedPromotion(t *testing.T) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
	busy, idle := NewState(1, "Busy"), NewState(2, "Idle")
	p, errorChan := newPipelineRaceProtocol(t, busy, idle)

	// Queue two ordinary pipelined messages first, each fully sent (bytes
	// on the wire) before the next is issued, so each gets its own
	// top-level dequeue and both land in sendLoop's deferred-transition
	// backlog (queuedStateTransitions) with none of it flushed yet -- state
	// remains Busy throughout, since neither message's transition is legal
	// to apply until real agency returns.
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := p.SendMessageContextAndWait(
			ctx,
			&MessageBase{MessageType: msgTypeRequest},
		)
		cancel()
		require.NoError(t, err, "backlog setup message must send")
	}
	require.Equal(t, busy, p.getCurrentState())

	// Now race a third message's dequeue against the peer's terminal event,
	// exactly as in the base race test, with the two-deep backlog already
	// in place.
	dequeued := make(chan struct{})
	proceed := make(chan struct{})
	p.pipelinedDequeueHook = func() {
		close(dequeued)
		<-proceed
	}

	thirdResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		thirdResult <- p.SendMessageContextAndWait(
			ctx,
			&MessageBase{MessageType: msgTypeRequest},
		)
	}()

	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("sendLoop never reached the pipelined dequeue check")
	}

	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)
	close(proceed)

	select {
	case err := <-thirdResult:
		require.NoError(
			t,
			err,
			"the racing message must still send once the backlog is "+
				"flushed as far as it legitimately can be",
		)
	case <-time.After(5 * time.Second):
		t.Fatal("third SendMessageContextAndWait never returned")
	}

	require.False(
		t,
		p.IsStopping(),
		"a partially flushable backlog must not tear down the connection",
	)
	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}

	// Exactly one of the two backlogged messages could legitimately flush
	// against the single real transition that landed (Idle -> Busy); the
	// state must reflect that, not a further, unjustified advance.
	require.Equal(
		t,
		busy,
		p.getCurrentState(),
		"only the one deferred transition the real BatchDone justified "+
			"should have flushed",
	)
}

// newStateLoopOnlyProtocol builds a Protocol whose only running goroutine is
// stateLoop, driven directly by transitionState and resolvePipelinedDequeue
// calls from the test itself. It has no muxer, sendLoop, or recvLoop, so a
// test can call resolvePipelinedDequeue synchronously against a chosen
// backlog and state sequence without racing the protocol's own sendLoop for
// the same sendReadyChan token.
func newStateLoopOnlyProtocol(
	t *testing.T,
	stateMap StateMap,
	initialState State,
) (p *Protocol, errorChan chan error) {
	t.Helper()
	errorChan = make(chan error, 1)
	p = New(ProtocolConfig{
		Name:         "test-pipeline-flush-token",
		ErrorChan:    errorChan,
		Role:         ProtocolRoleClient,
		StateMap:     stateMap,
		InitialState: initialState,
	})
	p.sendReadyChan = make(chan bool, 1)
	stateTransitionChan := make(chan protocolStateTransition)
	p.stateTransitionChan = stateTransitionChan
	go p.stateLoop(stateTransitionChan)
	t.Cleanup(p.Stop)
	return p, errorChan
}

// TestResolvePipelinedDequeueDrainsTokenProducedByOwnFlush drives
// resolvePipelinedDequeue directly against a three-state chain where the
// entry token and a token produced by the flush's own intermediate
// transition are two distinct grants: Busy (server agency, pipelines
// msgTypeRequest) -- BatchDone --> Idle (client agency) -- msgTypeRequest
// --> Idle2 (client agency) -- msgTypeRequest --> Busy. Flushing the
// two-message backlog applies both deferred transitions in the same call:
// the first lands in Idle2, which grants this role its own fresh
// sendReadyChan token exactly as the entry transition into Idle did, before
// the second spends the backlog down to Busy, where the role has none.
//
// That second token must not survive the call. Left in place, the next
// ordinary sendLoop iteration reads it as agency in Busy and applies the
// racing message's own deferred transition there, failing with "message ...
// not allowed in current protocol state Busy" -- the same symptom as
// blinklabs-io/gouroboros#2494, reached through the flush step rather than
// through the entry race.
func TestResolvePipelinedDequeueDrainsTokenProducedByOwnFlush(t *testing.T) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
	idle2 := NewState(3, "Idle2")
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
				{MsgType: msgTypeRequest, NewState: idle2},
			},
		},
		idle2: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeRequest, NewState: busy},
			},
		},
	}
	p, errorChan := newStateLoopOnlyProtocol(t, stateMap, busy)

	// Two messages pipelined while Busy, each deferred to its own
	// queuedStateTransitions entry -- exactly what sendLoop's
	// readSendQueueLoop appends before either can apply.
	queuedStateTransitions := []Message{
		&MessageBase{MessageType: msgTypeRequest},
		&MessageBase{MessageType: msgTypeRequest},
	}

	// The peer's terminal event: Busy -> Idle, granting this role (client)
	// agency in Idle -- the token resolvePipelinedDequeue's entry drain
	// expects to find.
	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)
	require.Equal(t, 1, len(p.sendReadyChan))

	racingMessage := &outboundMessage{
		message: &MessageBase{MessageType: msgTypeRequest},
	}
	outbound, haveAgency, err := p.resolvePipelinedDequeue(
		racingMessage,
		false,
		&queuedStateTransitions,
	)
	require.NoError(t, err)
	require.NotNil(
		t,
		outbound,
		"the racing message must not be dropped once the backlog flushes",
	)
	require.False(
		t,
		haveAgency,
		"the racing message still fits the pipelined path in the state "+
			"the flush reached, so it is not promoted",
	)
	require.Empty(
		t,
		queuedStateTransitions,
		"both backlogged transitions must have flushed: Idle -> Idle2 -> Busy",
	)
	require.Equal(
		t,
		busy,
		p.getCurrentState(),
		"the flush must land back in Busy via the intermediate Idle2 state",
	)
	require.Equal(
		t,
		0,
		len(p.sendReadyChan),
		"the token setState placed for the intermediate Idle2 grant must "+
			"not survive resolvePipelinedDequeue",
	)

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

// TestResolvePipelinedDequeuePromotesOnlyWhenBacklogNotEmpty asserts that
// resolvePipelinedDequeue never promotes a racing message while backlogged
// transitions remain queued. sendLoop applies any remaining queued
// transition before it ever sends a promoted message and then loops without
// sending it (its "haveAgency && len(queuedStateTransitions) > 0" branch), so
// a promotion returned alongside a non-empty backlog would be silently
// dropped by the caller rather than sent.
func TestResolvePipelinedDequeuePromotesOnlyWhenBacklogNotEmpty(t *testing.T) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeOther uint8 = 4
	const msgTypeBatchDone uint8 = 3
	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
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
			// Deliberately no transition for msgTypeRequest: the backlogged
			// message below cannot flush from Idle, so it stays queued.
			Transitions: []StateTransition{
				{MsgType: msgTypeOther, NewState: busy},
			},
		},
	}
	p, errorChan := newStateLoopOnlyProtocol(t, stateMap, busy)

	queuedStateTransitions := []Message{
		&MessageBase{MessageType: msgTypeRequest},
	}

	require.NoError(
		t,
		p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
	)

	// The racing message is of a different type than the stuck backlog
	// entry, and is legal as an ordinary transition from Idle -- the exact
	// condition that used to promote it regardless of the backlog.
	racingMessage := &outboundMessage{
		message: &MessageBase{MessageType: msgTypeOther},
	}
	outbound, haveAgency, err := p.resolvePipelinedDequeue(
		racingMessage,
		false,
		&queuedStateTransitions,
	)

	require.Len(
		t,
		queuedStateTransitions,
		1,
		"the backlog entry has no legal transition from Idle and must "+
			"stay queued",
	)
	require.False(
		t,
		haveAgency,
		"a non-empty backlog must never be promoted: sendLoop would apply "+
			"the queued transition first and discard this message unsent",
	)
	require.Nil(t, outbound)
	require.Error(
		t,
		err,
		"with a stuck backlog the racing message is rejected rather than "+
			"silently dropped",
	)

	select {
	case err := <-errorChan:
		t.Fatalf("unexpected protocol error: %v", err)
	default:
	}
}

// TestResolvePipelinedDequeueDoesNotLeakTokenAcrossConcurrentRecvLoopTransition
// reproduces a fourth failure mode found reviewing the fix for
// blinklabs-io/gouroboros#2494, distinct from the three covered above:
// resolvePipelinedDequeue's post-flush drain of sendReadyChan and its
// subsequent read of the current state used to take the lock/channel
// separately, as two independent steps. A transition driven by recvLoop
// processing a genuine, concurrent peer reply -- not this function's own
// flush -- can land in the gap between them: the state write becomes visible
// to the read, but the token setState produced alongside it (written outside
// any lock, after Unlock, in the pre-fix code) has not yet reached the
// channel at drain time. The drain finds nothing, the read observes the new
// state anyway, and the token that belongs to that state is left to be
// misread by a later, unrelated sendLoop iteration as agency for whatever
// state comes next.
//
// Idle deliberately has no transition for the racing message, and Idle2 does:
// this makes resolvePipelinedDequeue's own return value name which state its
// post-flush read actually observed, which a construction where both states
// accept the message cannot distinguish (both promote identically either
// way, so nothing about the return value would depend on the race at all).
// A rejection (postFlushState read as Idle, before the concurrent transition
// committed) is a legitimate, race-free outcome and is not itself checked
// further. A promotion (postFlushState read as Idle2) means this call's own
// read observed a transition that -- per setState's contract -- always
// attempts a sendReadyChan token together with the state write; the
// invariant under test is that such an observation never happens without
// this call also having consumed that token, checked the instant the call
// returns and its racing goroutine has finished (nothing else touches
// sendReadyChan in between).
//
// pipelinedDequeueHook cannot reach this window: it fires at sendLoop's
// top-level dequeue, before resolvePipelinedDequeue is even entered.
// resolvePipelinedDequeuePostFlushHook exists for exactly this window, but it
// only *positions* a genuinely concurrent goroutine as close as possible to
// the decision point -- it does not force a specific interleaving inside
// setState itself (state-write-then-token-write in the pre-fix code is not a
// step this package exposes a seam for), so whether that goroutine's
// transition lands early enough to still race the post-fix atomic drain-and-
// read remains a real scheduling race, and this loop still needs many
// iterations, not a single deterministic one, to exercise it.
func TestResolvePipelinedDequeueDoesNotLeakTokenAcrossConcurrentRecvLoopTransition(
	t *testing.T,
) {
	t.Parallel()

	const msgTypeRequest uint8 = 2
	const msgTypeBatchDone uint8 = 3
	const msgTypeNoBlocks uint8 = 5
	busy := NewState(1, "Busy")
	idle := NewState(2, "Idle")
	idle2 := NewState(3, "Idle2")
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
			// Deliberately no transition for msgTypeRequest: a promotion can
			// only happen if the post-flush read observed Idle2, never Idle.
			Transitions: []StateTransition{
				{MsgType: msgTypeNoBlocks, NewState: idle2},
			},
		},
		idle2: StateMapEntry{
			Agency: AgencyClient,
			Transitions: []StateTransition{
				{MsgType: msgTypeRequest, NewState: busy},
			},
		},
	}

	const iterations = 500
	promotions := 0
	for iter := range iterations {
		p, errorChan := newStateLoopOnlyProtocol(t, stateMap, busy)

		require.NoError(
			t,
			p.transitionState(&MessageBase{MessageType: msgTypeBatchDone}),
			"iteration %d: entry transition Busy -> Idle",
			iter,
		)

		var queuedStateTransitions []Message
		racingMessage := &outboundMessage{
			message: &MessageBase{MessageType: msgTypeRequest},
		}

		// Fire a real, concurrent stateLoop transition -- standing in for
		// recvLoop handling an independent peer reply (NoBlocks) -- from
		// inside the post-flush hook, then busy-wait right there for its
		// state write to become visible before letting
		// resolvePipelinedDequeue proceed to its own drain-and-read. This
		// puts the two goroutines as close together as a test can manage
		// without a seam inside setState itself: the hook returns at the
		// earliest instant the new state is observable, which is exactly
		// the boundary the pre-fix code's separate drain and read could
		// straddle inconsistently.
		concurrentDone := make(chan struct{})
		p.resolvePipelinedDequeuePostFlushHook = func() {
			go func() {
				defer close(concurrentDone)
				_ = p.transitionState(
					&MessageBase{MessageType: msgTypeNoBlocks},
				)
			}()
			deadline := time.Now().Add(2 * time.Second)
			for p.getCurrentState() != idle2 {
				if time.Now().After(deadline) {
					t.Fatalf(
						"iteration %d: concurrent NoBlocks transition never "+
							"became visible",
						iter,
					)
				}
				runtime.Gosched()
			}
		}

		outbound, haveAgency, err := p.resolvePipelinedDequeue(
			racingMessage,
			false,
			&queuedStateTransitions,
		)
		p.resolvePipelinedDequeuePostFlushHook = nil
		// Guaranteed to close only once the concurrent transitionState call
		// itself returns, which happens only once setState (state write and
		// token write both) has fully completed -- so by the time this
		// unblocks, nothing about the race is still in flight.
		<-concurrentDone

		if err != nil {
			// The read observed Idle before the concurrent transition
			// committed: a legitimate, race-free outcome. Nothing about the
			// window under test applies to this iteration.
			require.False(t, haveAgency, "iteration %d", iter)
			require.Nil(t, outbound, "iteration %d", iter)
			select {
			case unexpected := <-errorChan:
				t.Fatalf(
					"iteration %d: unexpected protocol error: %v",
					iter,
					unexpected,
				)
			default:
			}
			continue
		}

		promotions++
		require.True(
			t,
			haveAgency,
			"iteration %d: a nil error from resolvePipelinedDequeue always "+
				"means promotion",
			iter,
		)
		require.NotNil(t, outbound, "iteration %d", iter)
		require.Empty(t, queuedStateTransitions, "iteration %d", iter)

		// err == nil is only reachable via Idle2's transition (Idle has none
		// for this message), so this call's own read observed the concurrent
		// transition. setState always pairs a state write with an attempt to
		// write a sendReadyChan token for an AgencyClient state under this
		// role -- the invariant is that this call must have consumed that
		// token as part of the very read that used its result, not left it
		// for whichever unrelated sendLoop iteration wakes up next.
		require.Equal(
			t,
			0,
			len(p.sendReadyChan),
			"iteration %d: resolvePipelinedDequeue observed the concurrent "+
				"Idle -> Idle2 transition to decide this promotion but left "+
				"that transition's own sendReadyChan token stranded -- it "+
				"will be misread by a later, unrelated sendLoop iteration "+
				"as agency for whatever state comes next",
			iter,
		)

		select {
		case unexpected := <-errorChan:
			t.Fatalf(
				"iteration %d: unexpected protocol error: %v",
				iter,
				unexpected,
			)
		default:
		}
	}

	// The post-flush hook busy-waits for the concurrent transition's state
	// write to become visible before releasing resolvePipelinedDequeue, so
	// nearly every iteration should observe Idle2 and promote; a rejection
	// is only possible if that goroutine was itself unusually slow to start.
	// If none did, the hook's positioning failed entirely and the race
	// window this test targets was never reached at all.
	require.Greater(
		t,
		promotions,
		0,
		"no iteration observed the concurrent transition at all -- the race "+
			"window this test targets was never reached",
	)
}
