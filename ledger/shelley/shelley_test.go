// Copyright 2024 Blink Labs Software
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

package shelley

import (
	"testing"
)

func TestNonceUnmarshalCBOR(t *testing.T) {
	testCases := []struct {
		name        string
		data        []byte
		expectedErr string
	}{
		{
			name: "NonceType0",
			data: []byte{0x81, 0x00},
		},
		{
			name: "NonceType1",
			data: []byte{0x82, 0x01, 0x42, 0x01, 0x02},
		},
		{
			name:        "UnsupportedNonceType",
			data:        []byte{0x82, 0x02},
			expectedErr: "unsupported nonce type 2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			n := &Nonce{}
			err := n.UnmarshalCBOR(tc.data)
			if err != nil {
				if tc.expectedErr == "" || err.Error() != tc.expectedErr {
					t.Errorf("unexpected error: %v", err)
				}
			} else if tc.expectedErr != "" {
				t.Errorf("expected error: %v, got nil", tc.expectedErr)
			}
		})
	}
}
