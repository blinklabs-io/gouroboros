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
	txId := make([]byte, common.Blake2b256Size)
	txId[0] = 1
	// The first index is the non-minimal encoding 0x1800. The second copy is
	// canonical, so normalization must not be used for wire round-tripping.
	wire := []byte{0x82, 0x82, 0x58, 0x20}
	wire = append(wire, txId...)
	wire = append(wire, 0x18, 0x00, 0x82, 0x58, 0x20)
	wire = append(wire, txId...)
	wire = append(wire, 0x00)

	var set shelley.ShelleyTransactionInputSet
	_, err := cbor.Decode(wire, &set)
	require.NoError(t, err)
	expected := shelley.ShelleyTransactionInput{
		TxId:        common.Blake2b256(txId),
		OutputIndex: 0,
	}
	assert.Equal(t, []shelley.ShelleyTransactionInput{expected}, set.Items())

	encoded, err := cbor.Encode(&set)
	require.NoError(t, err)
	assert.Equal(t, wire, encoded)
}

func TestShelleyTransactionInputSetOnlyCoalescesOnDecode(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"0101010101010101010101010101010101010101010101010101010101010101",
		0,
	)
	set := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input, input},
	)

	assert.Equal(t, []shelley.ShelleyTransactionInput{input, input}, set.Items())
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
