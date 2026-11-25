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

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/stretchr/testify/require"
)

// TestStoppedInstanceLeavesSuccessorIngress checks that a stopped instance
// cannot change the ingress settings of the instance registered after it.
// The muxer keys them by protocol ID and role, so a block-fetch client that
// resolves its old instance's requests after a restart would otherwise reset
// the new instance's allowance and backpressure.
func TestStoppedInstanceLeavesSuccessorIngress(t *testing.T) {
	conn, peer := net.Pipe()
	m := muxer.New(conn)
	m.Start()
	t.Cleanup(func() {
		_ = m.Stop(context.Background())
		_ = peer.Close()
	})
	idle := NewState(1, "Idle")
	cfg := ProtocolConfig{
		Name:         "ingress-owner",
		ProtocolId:   1,
		Muxer:        m,
		ErrorChan:    make(chan error, 4),
		Role:         ProtocolRoleClient,
		StateMap:     StateMap{idle: {Agency: AgencyClient}},
		InitialState: idle,
	}
	old := New(cfg)
	old.Start()
	require.NoError(t, old.Stop(context.Background()))

	successor := New(cfg)
	successor.Start()
	t.Cleanup(func() { _ = successor.Stop(context.Background()) })
	allowance := cfg.ingressLimit() + 2_509_011
	successor.SetIngressAllowance(allowance)
	successor.SetIngressBackpressure(true)

	old.SetIngressAllowance(0)
	old.SetIngressBackpressure(false)

	require.Equal(
		t,
		allowance,
		m.IngressLimit(1, muxer.ProtocolRoleInitiator),
		"the stopped instance reset its successor's ingress limit",
	)
	require.True(
		t,
		m.IngressBackpressure(1, muxer.ProtocolRoleInitiator),
		"the stopped instance cleared its successor's backpressure",
	)
}
