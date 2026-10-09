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

package chainsync

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/internal/testdata"
	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/pipeline"
	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

const chainSyncTestShelleyEta0 = "829749cb2701843214ae3aee67ae12ec9bdb3502e060ac0b75275d0f52af349c"

func validatedPipelineOptions(
	t *testing.T,
	apply pipeline.ApplyFunc,
) []pipeline.PipelineOption {
	t.Helper()
	verifyConfig, err := testdata.ShelleyVerifyConfig()
	require.NoError(t, err)
	return []pipeline.PipelineOption{
		pipeline.WithDecodeWorkers(1),
		pipeline.WithValidateWorkers(1),
		pipeline.WithBlockTypeResolver(
			func(context.Context, []byte) (uint, error) {
				return chainSyncShelleyBlock(t).BlockType, nil
			},
		),
		pipeline.WithEta0(chainSyncTestShelleyEta0),
		pipeline.WithSlotsPerKesPeriod(129600),
		pipeline.WithVerifyConfig(verifyConfig),
		pipeline.WithChainContextValidator(
			func(context.Context, *pipeline.BlockItem) error { return nil },
		),
		pipeline.WithApplyFunc(apply),
	}
}

func chainSyncShelleyBlock(t *testing.T) testdata.TestBlock {
	t.Helper()
	for _, block := range testdata.GetTestBlocks() {
		if block.Name == "Shelley" {
			return block
		}
	}
	t.Fatal("Shelley test block not found")
	return testdata.TestBlock{}
}

func TestExactTipCallbacksExposeIntersectionBeforeAwaitReply(t *testing.T) {
	intersect := pcommon.NewPoint(100, testPointHash(0x01))
	tip := Tip{
		Point:       pcommon.NewPoint(100, testPointHash(0x02)),
		BlockNumber: 10,
	}
	var callbacks []string
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{
			IntersectFoundFunc: func(
				_ CallbackContext,
				gotIntersect pcommon.Point,
				gotTip Tip,
			) error {
				require.Equal(t, intersect, gotIntersect)
				require.Equal(t, tip, gotTip)
				callbacks = append(callbacks, "intersect")
				return nil
			},
			AwaitReplyFunc: func(CallbackContext) error {
				callbacks = append(callbacks, "await")
				return nil
			},
		},
	)

	require.NoError(
		t,
		client.handleIntersectFound(NewMsgIntersectFound(intersect, tip)),
	)
	require.NoError(t, client.handleAwaitReply())
	require.Equal(t, []string{"intersect", "await"}, callbacks)
}

func TestAtTipCallbackErrorsPropagate(t *testing.T) {
	awaitReplyErr := errors.New("await reply callback failed")
	intersectFoundErr := errors.New("intersect found callback failed")
	intersect := pcommon.NewPoint(100, testPointHash(0x01))
	tip := Tip{
		Point:       pcommon.NewPoint(100, testPointHash(0x02)),
		BlockNumber: 10,
	}

	t.Run("await reply", func(t *testing.T) {
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{
				AwaitReplyFunc: func(CallbackContext) error {
					return awaitReplyErr
				},
			},
		)
		require.ErrorIs(
			t,
			client.messageHandler(NewMsgAwaitReply()),
			awaitReplyErr,
		)
	})

	t.Run("intersect found", func(t *testing.T) {
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{
				IntersectFoundFunc: func(
					CallbackContext,
					pcommon.Point,
					Tip,
				) error {
					return intersectFoundErr
				},
			},
		)
		require.ErrorIs(
			t,
			client.messageHandler(NewMsgIntersectFound(intersect, tip)),
			intersectFoundErr,
		)
	})
}

func TestAwaitReplyHandlesPipelineShutdownState(t *testing.T) {
	t.Run("unrelated fence error propagates", func(t *testing.T) {
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			nil,
		)
		expectedErr := errors.New("fence failed")
		require.ErrorIs(
			t,
			client.handlePipelineFenceError(context.Background(), expectedErr),
			expectedErr,
		)
	})

	t.Run("stopped pipeline while running", func(t *testing.T) {
		p := pipeline.NewBlockPipeline(
			pipeline.WithTrustedDecodeOnly(),
		)
		require.NoError(t, p.Start(context.Background()))
		require.NoError(t, p.Stop())
		callbackCalled := false
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{
				Pipeline: p,
				AwaitReplyFunc: func(CallbackContext) error {
					callbackCalled = true
					return nil
				},
			},
		)
		client.lifecycleState = clientStateRunning
		require.ErrorIs(
			t,
			client.handleAwaitReply(),
			pipeline.ErrPipelineStopped,
		)
		require.False(t, callbackCalled)
	})

	t.Run("stopped pipeline while stopping", func(t *testing.T) {
		p := pipeline.NewBlockPipeline(
			pipeline.WithTrustedDecodeOnly(),
		)
		require.NoError(t, p.Start(context.Background()))
		require.NoError(t, p.Stop())
		callbackCalled := false
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{
				Pipeline: p,
				AwaitReplyFunc: func(CallbackContext) error {
					callbackCalled = true
					return nil
				},
			},
		)
		client.lifecycleState = clientStateStopping
		require.NoError(t, client.handleAwaitReply())
		require.False(t, callbackCalled)
	})

	t.Run("stopped pipeline while stopped", func(t *testing.T) {
		p := pipeline.NewBlockPipeline(
			pipeline.WithTrustedDecodeOnly(),
		)
		require.NoError(t, p.Start(context.Background()))
		require.NoError(t, p.Stop())
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{Pipeline: p},
		)
		client.lifecycleState = clientStateStopped
		require.NoError(t, client.handleAwaitReply())
	})

	t.Run("not-started pipeline while running", func(t *testing.T) {
		p := pipeline.NewBlockPipeline()
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{Pipeline: p},
		)
		client.lifecycleState = clientStateRunning
		require.ErrorIs(
			t,
			client.handleAwaitReply(),
			pipeline.ErrPipelineNotStarted,
		)
	})

	t.Run("not-started pipeline while stopping", func(t *testing.T) {
		p := pipeline.NewBlockPipeline()
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{Pipeline: p},
		)
		client.lifecycleState = clientStateStopping
		require.NoError(t, client.handleAwaitReply())
	})
}

func TestRollForwardWaitsForPipelineAcceptance(t *testing.T) {
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseApply) })
	}
	p := pipeline.NewBlockPipeline(validatedPipelineOptions(t,
		func(*pipeline.BlockItem) error {
			close(applyStarted)
			<-releaseApply
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	defer func() {
		release()
		require.NoError(t, p.Stop())
	}()

	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{
			Pipeline: p,
			RollForwardFunc: func(CallbackContext, uint, any, Tip) error {
				return nil
			},
		},
	)
	shelley := chainSyncShelleyBlock(t)
	rollForward, err := NewMsgRollForwardNtC(
		shelley.BlockType,
		shelley.Cbor,
		Tip{},
	)
	require.NoError(t, err)
	rollForwardDone := make(chan error, 1)
	go func() { rollForwardDone <- client.handleRollForward(rollForward) }()
	select {
	case <-applyStarted:
	case err := <-rollForwardDone:
		t.Fatalf("RollForward failed before apply: %v", err)
	case <-time.After(time.Second):
		t.Fatal("pipeline apply did not start")
	}
	select {
	case err := <-rollForwardDone:
		t.Fatalf("RollForward completed before ordered acceptance: %v", err)
	default:
	}

	release()
	select {
	case err := <-rollForwardDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("RollForward did not finish after ordered acceptance")
	}
}

func TestRollForwardReturnsOrderedValidationFailure(t *testing.T) {
	applyCalled := false
	opts := validatedPipelineOptions(t, func(*pipeline.BlockItem) error {
		applyCalled = true
		return nil
	})
	opts = append(opts, pipeline.WithChainContextValidator(
		func(context.Context, *pipeline.BlockItem) error {
			return errors.New("wrong active era")
		},
	))
	p := pipeline.NewBlockPipeline(opts...)
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() { require.NoError(t, p.Stop()) })

	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{
			Pipeline: p,
			RollForwardFunc: func(CallbackContext, uint, any, Tip) error {
				return nil
			},
		},
	)
	shelley := chainSyncShelleyBlock(t)
	rollForward, err := NewMsgRollForwardNtC(
		shelley.BlockType,
		shelley.Cbor,
		Tip{},
	)
	require.NoError(t, err)
	require.ErrorIs(
		t,
		client.handleRollForward(rollForward),
		pipeline.ErrChainContextValidation,
	)
	require.False(t, applyCalled)
}

func TestRollForwardFenceStopsWithClientLifecycle(t *testing.T) {
	for _, protocolStop := range []bool{false, true} {
		name := "client lifecycle"
		if protocolStop {
			name = "protocol stop"
		}
		t.Run(name, func(t *testing.T) {
			applyStarted := make(chan struct{})
			releaseApply := make(chan struct{})
			var releaseOnce sync.Once
			release := func() { releaseOnce.Do(func() { close(releaseApply) }) }
			p := pipeline.NewBlockPipeline(validatedPipelineOptions(t,
				func(*pipeline.BlockItem) error {
					close(applyStarted)
					<-releaseApply
					return nil
				},
			)...)
			require.NoError(t, p.Start(context.Background()))
			t.Cleanup(func() {
				release()
				require.NoError(t, p.Stop())
			})
			client := NewClient(
				protocol.ProtocolOptions{
					ConnectionId: testConnectionId(),
					Mode:         protocol.ProtocolModeNodeToClient,
				},
				&Config{
					Pipeline: p,
					RollForwardFunc: func(CallbackContext, uint, any, Tip) error {
						return nil
					},
				},
			)
			shelley := chainSyncShelleyBlock(t)
			rollForward, err := NewMsgRollForwardNtC(
				shelley.BlockType,
				shelley.Cbor,
				Tip{},
			)
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() { done <- client.handleRollForward(rollForward) }()
			select {
			case <-applyStarted:
			case err := <-done:
				t.Fatalf("RollForward failed before apply: %v", err)
			case <-time.After(time.Second):
				t.Fatal("pipeline apply did not start")
			}
			if protocolStop {
				proto := client.ProtocolInstance()
				client.startPipelineStopWatcher(proto)
				proto.Stop()
			} else {
				client.lifecycleMutex.Lock()
				client.lifecycleState = clientStateStopping
				client.awaitReplyCancel()
				client.lifecycleMutex.Unlock()
			}
			select {
			case err := <-done:
				require.NoError(t, err)
			case <-time.After(time.Second):
				t.Fatal("shutdown did not unblock RollForward fence")
			}
		})
	}
}

func newBlockedApplyPipeline(t *testing.T) *pipeline.BlockPipeline {
	t.Helper()
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseApply) })
	}
	p := pipeline.NewBlockPipeline(validatedPipelineOptions(t,
		func(*pipeline.BlockItem) error {
			close(applyStarted)
			<-releaseApply
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() {
		release()
		if err := p.Stop(); err != nil {
			t.Errorf("stop pipeline: %v", err)
		}
	})
	shelley := chainSyncShelleyBlock(t)
	require.NoError(
		t,
		p.Submit(
			context.Background(),
			shelley.BlockType,
			shelley.Cbor,
			Tip{},
		),
	)
	select {
	case <-applyStarted:
	case <-time.After(time.Second):
		t.Fatal("pipeline apply did not start")
	}
	return p
}

func TestRollBackwardReturnsDrainErrorWithoutCallback(t *testing.T) {
	p := newBlockedApplyPipeline(t)
	callbackCalled := false
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&Config{
			Pipeline:             p,
			PipelineDrainTimeout: 25 * time.Millisecond,
			RollBackwardFunc: func(CallbackContext, pcommon.Point, Tip) error {
				callbackCalled = true
				return nil
			},
		},
	)

	err := client.handleRollBackward(NewMsgRollBackward(pcommon.Point{}, Tip{}))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.False(t, callbackCalled)
}

func TestRollBackwardShutdownSkipsCallback(t *testing.T) {
	p := newBlockedApplyPipeline(t)
	callbackCalled := false
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
		&Config{
			Pipeline:             p,
			PipelineDrainTimeout: time.Second,
			RollBackwardFunc: func(CallbackContext, pcommon.Point, Tip) error {
				callbackCalled = true
				return nil
			},
		},
	)
	rollbackDone := make(chan error, 1)
	go func() {
		rollbackDone <- client.handleRollBackward(
			NewMsgRollBackward(pcommon.Point{}, Tip{}),
		)
	}()
	client.Protocol.Stop()

	select {
	case err := <-rollbackDone:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		t.Fatal("roll-back handler did not stop with the protocol")
	}
	require.False(t, callbackCalled)
}

func TestRollBackwardShutdownUsesStopRequestWhileReceiveLoopIsBlocked(
	t *testing.T,
) {
	applyStarted := make(chan struct{})
	releaseApply := make(chan struct{})
	var releaseApplyOnce sync.Once
	releaseApplyFunc := func() {
		releaseApplyOnce.Do(func() { close(releaseApply) })
	}
	p := pipeline.NewBlockPipeline(validatedPipelineOptions(t,
		func(*pipeline.BlockItem) error {
			close(applyStarted)
			<-releaseApply
			return nil
		},
	)...)
	require.NoError(t, p.Start(context.Background()))
	t.Cleanup(func() {
		releaseApplyFunc()
		if err := p.Stop(); err != nil {
			t.Errorf("stop pipeline: %v", err)
		}
	})
	clientConn, peerConn := net.Pipe()
	m := muxer.New(clientConn)
	m.Start()
	awaitStarted := make(chan struct{})
	releaseAwait := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseAwait) })
	}
	client := NewClient(
		protocol.ProtocolOptions{
			ConnectionId: testConnectionId(),
			Muxer:        m,
			Mode:         protocol.ProtocolModeNodeToClient,
		},
		&Config{
			Pipeline:             p,
			PipelineDrainTimeout: 5 * time.Second,
			AwaitReplyFunc: func(CallbackContext) error {
				close(awaitStarted)
				<-releaseAwait
				return nil
			},
		},
	)
	client.Start()
	peerDone := make(chan error, 1)
	go func() {
		header := make([]byte, 8)
		if _, err := io.ReadFull(peerConn, header); err != nil {
			peerDone <- err
			return
		}
		payloadLen := binary.BigEndian.Uint16(header[6:8])
		if _, err := io.CopyN(io.Discard, peerConn, int64(payloadLen)); err != nil {
			peerDone <- err
			return
		}
		payload, err := cbor.Encode(NewMsgAwaitReply())
		if err != nil {
			peerDone <- err
			return
		}
		segment := muxer.NewSegment(ProtocolIdNtC, payload, true)
		if err = binary.Write(peerConn, binary.BigEndian, segment.SegmentHeader); err != nil {
			peerDone <- err
			return
		}
		_, err = peerConn.Write(segment.Payload)
		peerDone <- err
	}()
	rollbackDone := make(chan error, 1)
	t.Cleanup(func() {
		release()
		client.Protocol.Stop()
		_ = peerConn.Close()
		_ = clientConn.Close()
		m.Stop()
		select {
		case <-client.DoneChan():
		case <-time.After(time.Second):
			t.Error(
				"protocol did not stop after releasing the receive callback",
			)
		}
	})

	require.NoError(t, client.Protocol.SendMessageAndWait(NewMsgRequestNext()))
	select {
	case err := <-peerDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("peer did not send AwaitReply")
	}
	select {
	case <-awaitStarted:
	case <-time.After(time.Second):
		t.Fatal("receive loop did not enter the blocked AwaitReply callback")
	}
	shelley := chainSyncShelleyBlock(t)
	require.NoError(t, p.Submit(
		context.Background(),
		shelley.BlockType,
		shelley.Cbor,
		Tip{},
	))
	select {
	case <-applyStarted:
	case <-time.After(time.Second):
		t.Fatal("pipeline apply did not start")
	}
	client.Protocol.Stop()
	select {
	case <-client.DoneChan():
		t.Fatal("DoneChan closed while the receive loop callback was blocked")
	default:
	}
	go func() {
		rollbackDone <- client.handleRollBackward(
			NewMsgRollBackward(pcommon.Point{}, Tip{}),
		)
	}()
	select {
	case err := <-rollbackDone:
		require.ErrorIs(t, err, protocol.ErrProtocolShuttingDown)
	case <-time.After(time.Second):
		release()
		select {
		case <-rollbackDone:
		case <-time.After(time.Second):
			t.Fatal(
				"roll-back handler remained blocked after receive loop release",
			)
		}
		t.Fatal(
			"roll-back handler waited for DoneChan instead of the stop request",
		)
	}
}
