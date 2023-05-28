package test

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// DecodeHexString is a helper function for tests that decodes hex strings. It doesn't return
// an error value, which makes it usable inline.
func DecodeHexString(hexData string) []byte {
	// Strip off any leading/trailing whitespace in hex string
	hexData = strings.TrimSpace(hexData)
	decoded, err := hex.DecodeString(hexData)
	if err != nil {
		panic(fmt.Sprintf("error decoding hex: %s", err))
	}
	return decoded
}
