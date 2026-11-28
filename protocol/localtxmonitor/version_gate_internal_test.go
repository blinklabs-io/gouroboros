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

package localtxmonitor

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// MsgGetMeasures exists from node-to-client version 20. A protocol built
// without a negotiated version (0) reads as the newest version, as the
// local-state-query version checks do; connection.go always passes the
// negotiated node-to-client version, which carries bit 15.
func TestSupportsGetMeasuresByVersion(t *testing.T) {
	for _, test := range []struct {
		version uint16
		want    bool
	}{
		{0, true},
		{protocol.ProtocolVersionNtCOffset + 9, false},
		{protocol.ProtocolVersionNtCOffset + 19, false},
		{protocol.ProtocolVersionNtCOffset + 20, true},
		{protocol.ProtocolVersionNtCOffset + 23, true},
	} {
		require.Equal(t, test.want, supportsGetMeasures(test.version), "version %#x", test.version)
		require.Equal(t, test.want,
			NewClient(protocol.ProtocolOptions{Version: test.version}, nil).enableGetMeasures,
			"client, version %#x", test.version)
		require.Equal(t, test.want,
			NewServer(protocol.ProtocolOptions{Version: test.version}, nil).enableGetMeasures,
			"server, version %#x", test.version)
	}
}
