package common

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/sha3"
)

func TestComputeByronAddressRootHashesChainCodeOfAnyLength(t *testing.T) {
	t.Parallel()
	pubkey := make([]byte, 32)
	attrs := []byte{0xa0}
	for _, n := range []int{0, 31, 32, 33} {
		chainCode := make([]byte, n)
		for i := range chainCode {
			chainCode[i] = byte(i + 1)
		}
		got, err := computeByronAddressRoot(pubkey, chainCode, attrs)
		require.NoError(t, err, "chain code length %d", n)

		// The reference keeps the 0x5840 byte-string header fixed and
		// hashes whatever chain code bytes were supplied.
		buf := []byte{0x83, 0x00, 0x82, 0x00, 0x58, 0x40}
		buf = append(buf, pubkey...)
		buf = append(buf, chainCode...)
		buf = append(buf, attrs...)
		sum := sha3.Sum256(buf)
		require.Equal(t, Blake2b224Hash(sum[:]), got, "chain code length %d", n)
	}
}
