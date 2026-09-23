package conway_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func testAccountAddress(t *testing.T) common.Address {
	t.Helper()
	addr, err := common.NewAddress("stake_test1uqehkck0lajq8gr28t9uxnuvgcqrc6070x3k9r8048z8y5gssrtvn")
	require.NoError(t, err)
	return addr
}
