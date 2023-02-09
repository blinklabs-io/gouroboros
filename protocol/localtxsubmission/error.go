package localtxsubmission

import (
	"fmt"
)

// TransactionRejectedError represents an explicit transaction rejection
type TransactionRejectedError struct {
	ReasonCbor []byte
	Reason     error
}

func (e TransactionRejectedError) Error() string {
	if e.Reason != nil {
		return e.Reason.Error()
	} else {
		return fmt.Sprintf("transaction rejected: CBOR reason hex: %x", e.ReasonCbor)
	}
}
