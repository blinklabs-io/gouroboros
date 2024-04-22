package ledger

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
