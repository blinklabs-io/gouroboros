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

package muxer_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/muxer"
	"github.com/blinklabs-io/gouroboros/protocol/blockfetch"
	"github.com/blinklabs-io/gouroboros/protocol/chainsync"
	"github.com/blinklabs-io/gouroboros/protocol/keepalive"
	"github.com/blinklabs-io/gouroboros/protocol/leiosfetch"
	"github.com/blinklabs-io/gouroboros/protocol/leiosnotify"
	"github.com/blinklabs-io/gouroboros/protocol/leiosvotes"
	"github.com/stretchr/testify/require"
)

func TestEgressClassOfProtocols(t *testing.T) {
	t.Parallel()
	for name, id := range map[string]uint16{
		"leiosnotify": leiosnotify.ProtocolId,
		"leiosfetch":  leiosfetch.ProtocolId,
		"leiosvotes":  leiosvotes.ProtocolId,
	} {
		require.Equal(t, muxer.EgressClassLeios, muxer.EgressClassOf(id), name)
	}
	for name, id := range map[string]uint16{
		"chainsync-ntn": chainsync.ProtocolIdNtN,
		"blockfetch":    blockfetch.ProtocolId,
		"keepalive":     keepalive.ProtocolId,
	} {
		require.Equal(t, muxer.EgressClassPraos, muxer.EgressClassOf(id), name)
	}
}
