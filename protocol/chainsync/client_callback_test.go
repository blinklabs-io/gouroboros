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
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/require"
)

func TestExactTipCallbacksExposeIntersectionBeforeAwaitReply(t *testing.T) {
	intersect := pcommon.NewPoint(100, []byte("intersection"))
	tip := Tip{Point: pcommon.NewPoint(100, []byte("tip")), BlockNumber: 10}
	var callbacks []string
	client := NewClient(
		protocol.ProtocolOptions{ConnectionId: testConnectionId()},
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

	require.NoError(t, client.handleIntersectFound(NewMsgIntersectFound(intersect, tip)))
	require.NoError(t, client.handleAwaitReply())
	require.Equal(t, []string{"intersect", "await"}, callbacks)
}

func TestAtTipCallbackErrorsPropagate(t *testing.T) {
	awaitReplyErr := errors.New("await reply callback failed")
	intersectFoundErr := errors.New("intersect found callback failed")
	intersect := pcommon.NewPoint(100, []byte("intersection"))
	tip := Tip{Point: pcommon.NewPoint(100, []byte("tip")), BlockNumber: 10}

	t.Run("await reply", func(t *testing.T) {
		client := NewClient(
			protocol.ProtocolOptions{ConnectionId: testConnectionId()},
			&Config{
				AwaitReplyFunc: func(CallbackContext) error {
					return awaitReplyErr
				},
			},
		)
		require.ErrorIs(t, client.messageHandler(NewMsgAwaitReply()), awaitReplyErr)
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
