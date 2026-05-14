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

package peersharing

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestServerHandleShareRequestRefusesWhenLocalDisabled verifies that an
// incoming ShareRequest message is rejected with ErrLocalPeerSharingDisabled
// when the handshake recorded that this node advertised NoPeerSharing.
// A spec-compliant peer must not send ShareRequest in that case, so we treat
// it as a protocol violation regardless of whether a callback is configured.
func TestServerHandleShareRequestRefusesWhenLocalDisabled(t *testing.T) {
	called := false
	cfg := NewConfig(
		WithLocalDisabled(true),
		WithShareRequestFunc(
			func(CallbackContext, int) ([]PeerAddress, error) {
				called = true
				return nil, nil
			},
		),
	)
	server := NewServer(testProtocolOptions(), &cfg)

	err := server.handleShareRequest(NewMsgShareRequest(5))
	require.ErrorIs(t, err, ErrLocalPeerSharingDisabled)
	require.False(t, called, "ShareRequestFunc must not be invoked when local disabled")
}

// TestServerHandleShareRequestRequiresCallback verifies that the legacy
// "no callback configured" safeguard still fires when the local side is not
// disabled. This also exercises the zero-value (legacy permissive) default
// for LocalDisabled — the request reaches the callback check rather than
// being short-circuited by the disabled-guard.
func TestServerHandleShareRequestRequiresCallback(t *testing.T) {
	cfg := NewConfig() // zero-value: LocalDisabled is false
	server := NewServer(testProtocolOptions(), &cfg)

	err := server.handleShareRequest(NewMsgShareRequest(5))
	require.Error(t, err, "expected an error when no ShareRequestFunc is set")
	require.NotErrorIs(t, err, ErrLocalPeerSharingDisabled,
		"zero-value config must not trip ErrLocalPeerSharingDisabled")
}
