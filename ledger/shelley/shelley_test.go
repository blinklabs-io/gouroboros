package shelley_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShelleyTransactionInputSetCoalescesUntaggedDuplicates(t *testing.T) {
	input1 := shelley.NewShelleyTransactionInput(
		"0101010101010101010101010101010101010101010101010101010101010101",
		0,
	)
	input2 := shelley.NewShelleyTransactionInput(
		"0202020202020202020202020202020202020202020202020202020202020202",
		1,
	)
	wire, err := cbor.Encode([]shelley.ShelleyTransactionInput{
		input2,
		input1,
		input2,
	})
	require.NoError(t, err)

	var set shelley.ShelleyTransactionInputSet
	_, err = cbor.Decode(wire, &set)
	require.NoError(t, err)
	assert.Equal(t, []shelley.ShelleyTransactionInput{input2, input1}, set.Items())
}

func TestShelleyTransactionOutputString(t *testing.T) {
	addrStr := "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"
	addr, _ := common.NewAddress(addrStr)
	out := shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	s := out.String()
	expected := fmt.Sprintf(
		"(ShelleyTransactionOutput address=%s amount=456)",
		addrStr,
	)
	if s != expected {
		t.Fatalf("unexpected string: %s", s)
	}
}

func TestShelleyOutputTooSmallErrorFormatting(t *testing.T) {
	addrStr := "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"
	addr, _ := common.NewAddress(addrStr)
	out := &shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	errStr := shelley.OutputTooSmallUtxoError{
		Outputs: []common.TransactionOutput{out},
	}.Error()
	expected := fmt.Sprintf(
		"output too small: (ShelleyTransactionOutput address=%s amount=456)",
		addrStr,
	)
	if errStr != expected {
		t.Fatalf("unexpected error: %s", errStr)
	}
}
