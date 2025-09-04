package shelley_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestShelleyTransactionOutputString(t *testing.T) {
	addr, _ := common.NewAddress("addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd")
	out := shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	s := out.String()
	expected := fmt.Sprintf("(ShelleyTransactionOutput address=%s amount=456)", addr.String())
	if s != expected {
		t.Fatalf("unexpected string: %s", s)
	}
}

func TestShelleyOutputTooSmallErrorFormatting(t *testing.T) {
	addr, _ := common.NewAddress("addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd")
	out := &shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	errStr := shelley.OutputTooSmallUtxoError{Outputs: []common.TransactionOutput{out}}.Error()
	expected := fmt.Sprintf("output too small: (ShelleyTransactionOutput address=%s amount=456)", addr.String())
	if errStr != expected {
		t.Fatalf("unexpected error: %s", errStr)
	}
}
