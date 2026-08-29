package localstatequery

import (
	"net"
	"testing"

	"github.com/blinklabs-io/gouroboros/connection"
	"github.com/blinklabs-io/gouroboros/ledger"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/protocol"
	"github.com/stretchr/testify/require"
)

func TestValidateLocalStateQuerySetRejectsOversizedInput(t *testing.T) {
	items := make([]int, maxLocalStateQuerySetItems+1)
	require.Error(t, validateLocalStateQuerySet(items, "items"))
}

func TestValidateLocalStateQuerySetRejectsDuplicateInput(t *testing.T) {
	require.Error(t, validateLocalStateQuerySet([]int{1, 2, 1}, "items"))
}

func TestValidateLocalStateQuerySetPreservesDistinctInput(t *testing.T) {
	require.NoError(t, validateLocalStateQuerySet([]int{1, 2, 3}, "items"))
}

func TestSetQueryInputsAreBoundedBeforeProtocolUse(t *testing.T) {
	client := NewClient(protocol.ProtocolOptions{
		ConnectionId: connection.ConnectionId{
			LocalAddr:  &net.TCPAddr{},
			RemoteAddr: &net.TCPAddr{},
		},
	}, nil)
	overflowCredentials := make([]lcommon.Credential, maxLocalStateQuerySetItems+1)
	overflowDreps := make([]lcommon.Drep, maxLocalStateQuerySetItems+1)
	overflowPoolIDs := make([]ledger.PoolId, maxLocalStateQuerySetItems+1)
	overflowStatuses := make([]int, maxLocalStateQuerySetItems+1)

	tests := []struct {
		name  string
		query func() error
	}{
		{
			name: "pool distribution v2",
			query: func() error {
				_, err := client.GetPoolDistr2(overflowPoolIDs)
				return err
			},
		},
		{
			name: "DRep state",
			query: func() error {
				_, err := client.GetDRepState(overflowCredentials)
				return err
			},
		},
		{
			name: "DRep stake distribution",
			query: func() error {
				_, err := client.GetDRepStakeDistr(overflowDreps)
				return err
			},
		},
		{
			name: "committee cold credentials",
			query: func() error {
				_, err := client.GetCommitteeMembersState(
					overflowCredentials,
					nil,
					nil,
				)
				return err
			},
		},
		{
			name: "committee hot credentials",
			query: func() error {
				_, err := client.GetCommitteeMembersState(
					nil,
					overflowCredentials,
					nil,
				)
				return err
			},
		},
		{
			name: "committee statuses",
			query: func() error {
				_, err := client.GetCommitteeMembersState(
					nil,
					nil,
					overflowStatuses,
				)
				return err
			},
		},
		{
			name: "filtered vote delegatees",
			query: func() error {
				_, err := client.GetFilteredVoteDelegatees(overflowCredentials)
				return err
			},
		},
		{
			name: "SPO stake distribution",
			query: func() error {
				_, err := client.GetSPOStakeDistr(overflowPoolIDs)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Error(t, test.query())
		})
	}
}
