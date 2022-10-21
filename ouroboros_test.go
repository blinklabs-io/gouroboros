package ouroboros_test

import (
	ouroboros "github.com/cloudstruct/go-ouroboros-network"
	"testing"
)

// Ensure that we don't panic when closing the Ouroboros object after a failed Dial() call
func TestDialFailClose(t *testing.T) {
	oConn, err := ouroboros.New()
	if err != nil {
		t.Fatalf("unexpected error when creating Ouroboros object: %s", err)
	}
	err = oConn.Dial("unix", "/path/does/not/exist")
	if err == nil {
		t.Fatalf("did not get expected failure on Dial()")
	}
	// Close Ouroboros connection
	oConn.Close()
}
