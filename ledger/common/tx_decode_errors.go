// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package common

import (
	"fmt"
	"strings"
)

// TxComponent identifies which part of a transaction a decode error came from.
type TxComponent uint8

const (
	TxComponentBody TxComponent = iota
	TxComponentWitness
	TxComponentMetadata
)

func (c TxComponent) String() string {
	switch c {
	case TxComponentBody:
		return "body"
	case TxComponentWitness:
		return "witness"
	case TxComponentMetadata:
		return "metadata"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(c))
	}
}

// txBodyFieldNames maps Cardano transaction body map keys to their canonical
// names. Used to enrich TxDecodeError messages when a specific field fails to
// decode.
var txBodyFieldNames = map[int]string{
	0:  "inputs",
	1:  "outputs",
	2:  "fee",
	3:  "ttl",
	4:  "certificates",
	5:  "withdrawals",
	6:  "update",
	7:  "auxiliary_data_hash",
	8:  "validity_interval_start",
	9:  "mint",
	11: "script_data_hash",
	13: "collateral",
	14: "required_signers",
	15: "network_id",
	16: "collateral_return",
	17: "total_collateral",
	18: "reference_inputs",
	19: "voting_procedures",
	20: "proposal_procedures",
	21: "current_treasury_value",
	22: "donation",
}

// TxFieldName returns the canonical name for a transaction body map key, or
// an empty string if the key is unknown. The lookup is body-specific; callers
// dealing with witness or metadata maps should not rely on it.
func TxFieldName(key int) string {
	return txBodyFieldNames[key]
}

// TxDecodeError wraps a decode error with transaction-specific context.
type TxDecodeError struct {
	Inner      error
	TxIndex    int
	Component  TxComponent
	FieldKey   int
	Offset     int
	Diagnostic string
}

// NewTxDecodeError constructs a TxDecodeError. txIndex < 0 means the index is
// unknown; fieldKey < 0 means the failing field is unknown.
func NewTxDecodeError(
	inner error,
	txIndex int,
	component TxComponent,
	fieldKey, offset int,
	diagnostic string,
) *TxDecodeError {
	return &TxDecodeError{
		Inner:      inner,
		TxIndex:    txIndex,
		Component:  component,
		FieldKey:   fieldKey,
		Offset:     offset,
		Diagnostic: diagnostic,
	}
}

func (e *TxDecodeError) Error() string {
	if e == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("tx decode error")
	if e.TxIndex >= 0 {
		fmt.Fprintf(&b, " [tx %d]", e.TxIndex)
	}
	fmt.Fprintf(&b, " in %s", e.Component)
	if e.FieldKey >= 0 {
		if name := TxFieldName(e.FieldKey); name != "" && e.Component == TxComponentBody {
			fmt.Fprintf(&b, " field %d (%s)", e.FieldKey, name)
		} else {
			fmt.Fprintf(&b, " field %d", e.FieldKey)
		}
	}
	if e.Offset > 0 {
		fmt.Fprintf(&b, " at offset %d", e.Offset)
	}
	if e.Inner != nil {
		fmt.Fprintf(&b, ": %v", e.Inner)
	}
	return b.String()
}

func (e *TxDecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// WithDiagnostic returns Error() followed by the stored Diagnostic snippet on
// new lines. When Diagnostic is empty, it returns Error() unchanged.
func (e *TxDecodeError) WithDiagnostic() string {
	if e == nil {
		return ""
	}
	if e.Diagnostic == "" {
		return e.Error()
	}
	var b strings.Builder
	b.WriteString(e.Error())
	b.WriteString("\n\nDiagnostic:\n")
	b.WriteString(e.Diagnostic)
	return b.String()
}
