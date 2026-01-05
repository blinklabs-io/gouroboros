package shelley_test

import (
"math/big"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestShelleyTransactionOutputString(t *testing.T) {
	addrStr := "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"
	addr, _ := common.NewAddress(addrStr)
	out := shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount: new(big.Int).SetUint64(456),
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
		OutputAmount: new(big.Int).SetUint64(456),
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
