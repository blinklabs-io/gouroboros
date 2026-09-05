// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

func TestResolveInputUtxoRejectsTypedNilOutput(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"0000000000000000000000000000000000000000000000000000000000000000", 0,
	)
	var output *shelley.ShelleyTransactionOutput
	state := mockledger.NewLedgerStateBuilder().WithUtxoById(
		func(common.TransactionInput) (common.Utxo, error) {
			return common.Utxo{Output: output}, nil
		},
	).Build()

	_, err := common.ResolveInputUtxo(state, input)
	require.ErrorIs(t, err, common.ErrInputResolution)
}
