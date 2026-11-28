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

package messagesubmission

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

// Message Submission V2 runs on DMQ node-to-node version 2 and on Cardano
// node-to-node versions from MessageSubmissionV2MinVersion. A protocol
// built without a negotiated version (0) runs V1, and so does DMQ version 1.
func TestMessageSubmissionV2ByVersion(t *testing.T) {
	for _, test := range []struct {
		version uint16
		want    bool
	}{
		{0, false},
		{protocol.ProtocolVersionDMQNtN1, false},
		{protocol.ProtocolVersionDMQNtN2, true},
		{MessageSubmissionV2MinVersion - 1, false},
		{MessageSubmissionV2MinVersion, true},
		{16, true},
	} {
		require.Equal(t, test.want, isMessageSubmissionV2(test.version), "version %d", test.version)
		wantRole := protocol.ProtocolRoleClient
		if test.want {
			wantRole = protocol.ProtocolRoleServer
		}
		require.Equal(t, wantRole, roleFor(test.version, protocol.ProtocolRoleClient), "version %d", test.version)
	}
}
