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

package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestValidateMessageTTLAtAsReference checks the expiry bounds against
// dmq-node a267003c14e9 src/DMQ/Protocol/SigSubmission/Validate.hs 115-122:
// a message is expired when now > expiresAt, and too far ahead when
// expiresAt >= now + maxSigExpiresAtDelay (1800 s, src/DMQ/Policy.hs 60-61).
// The reference's now is a POSIXTime with sub-second precision, while
// expiresAt is whole seconds on the wire. dmq-node ships no vectors for this
// rule; the cases are derived from that code.
func TestValidateMessageTTLAtAsReference(t *testing.T) {
	const base = int64(1_000_000)
	half := 500 * time.Millisecond
	cases := []struct {
		name      string
		expiresAt int64
		nowOffset time.Duration
		expired   bool
		tooFar    bool
	}{
		{"expires now", base, 0, false, false},
		{"expired half a second ago", base, half, true, false},
		{"expired a second ago", base - 1, 0, true, false},
		{"expires in a second", base + 1, 0, false, false},
		{"last second inside the bound", base + 1799, 0, false, false},
		{"exactly the bound", base + 1800, 0, false, true},
		{"the bound, half a second later", base + 1800, half, false, false},
		{"past the bound", base + 1801, half, false, true},
	}
	validator := NewTTLValidator(30*time.Minute, nil)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := &DmqMessage{
				Payload: DmqMessagePayload{ExpiresAt: uint32(tc.expiresAt)},
			}
			now := time.Unix(base, 0).Add(tc.nowOffset)
			err := validator.ValidateMessageTTLAt(msg, now)
			switch {
			case tc.expired:
				require.ErrorIs(t, err, ErrMessageExpired)
			case tc.tooFar:
				require.Error(t, err)
				require.NotErrorIs(t, err, ErrMessageExpired)
			default:
				require.NoError(t, err)
			}
		})
	}
}
