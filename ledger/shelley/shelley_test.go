package shelley_test

import (
	"regexp"
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
	re := regexp.MustCompile(`^\(ShelleyTransactionOutput address=addr1[0-9a-z]+ amount=456\)$`)
	if !re.MatchString(s) {
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
	if matched, _ := regexp.MatchString(`^output too small: \(ShelleyTransactionOutput address=addr1[0-9a-z]+ amount=456\)$`, errStr); !matched {
		t.Fatalf("unexpected error: %s", errStr)
	}
}
