package byron

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeByronWitnessRequiresConstructorKeySize(t *testing.T) {
	for _, test := range []struct {
		name        string
		constructor uint64
		keySize     int
		valid       bool
	}{
		{name: "payment extended key", constructor: 0, keySize: 64, valid: true},
		{name: "payment short key", constructor: 0, keySize: 32},
		{name: "redeem key", constructor: 2, keySize: 32, valid: true},
		{name: "redeem extended key", constructor: 2, keySize: 64},
	} {
		t.Run(test.name, func(t *testing.T) {
			vkey, _, ok := decodeByronWitnessFromConstructor(
				test.constructor,
				[]any{make([]byte, test.keySize), make([]byte, 64)},
			)
			require.Equal(t, test.valid, ok)
			if test.valid {
				require.NotNil(t, vkey)
			}
		})
	}
}
