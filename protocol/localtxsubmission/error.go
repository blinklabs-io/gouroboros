package localtxsubmission

import (
	"fmt"
)

type TransactionRejectedError struct {
	ReasonCbor []byte
}

func (e TransactionRejectedError) Error() string {
	return fmt.Sprintf("transaction rejected: CBOR reason hex: %x", e.ReasonCbor)
}
