package cbor_test

import (
	"encoding/hex"
	"testing"

	"github.com/cloudstruct/go-ouroboros-network/cbor"
)

type encodeTestDefinition struct {
	CborHex string
	Object  interface{}
}

var encodeTests = []encodeTestDefinition{
	// Simple list of numbers
	{
		CborHex: "83010203",
		Object:  []interface{}{1, 2, 3},
	},
}

func TestEncode(t *testing.T) {
	for _, test := range encodeTests {
		cborData, err := cbor.Encode(test.Object)
		if err != nil {
			t.Fatalf("failed to encode object to CBOR: %s", err)
		}
		cborHex := hex.EncodeToString(cborData)
		if cborHex != test.CborHex {
			t.Fatalf("object did not encode to expected CBOR\n  got: %s\n  wanted: %s", cborHex, test.CborHex)
		}
	}
}
