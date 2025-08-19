// Copyright 2025 Blink Labs Software
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

package script

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	lcommon "github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/plutigo/data"
)

func formatPlutusData(pd data.PlutusData) string {
	var builder strings.Builder
	pdStr := pd.String()
	indentLevel := 0
	needsNewline := false
	for _, c := range pdStr {
		if c == ' ' && needsNewline {
			builder.WriteString("\n" + strings.Repeat(` `, indentLevel*4))
			continue
		}
		needsNewline = false
		if c == '{' || c == '[' {
			indentLevel++
			needsNewline = true
		}
		if c == '}' || c == ']' {
			indentLevel--
			builder.WriteString("\n" + strings.Repeat(` `, indentLevel*4))
			needsNewline = true
		}
		if c == ')' {
			needsNewline = true
		}
		if c == ',' {
			needsNewline = true
		}
		builder.WriteRune(c)
		if needsNewline {
			builder.WriteString("\n" + strings.Repeat(` `, indentLevel*4))
		}
	}
	ret := builder.String()
	// Strip out empty lines
	tmpRe := regexp.MustCompile(`\n +\n`)
	ret = tmpRe.ReplaceAllString(ret, "\n")
	return ret
}

func buildTxInfoV3(
	txHex string,
	inputsHex string,
	inputOutputsHex string,
) (TxInfo, error) {
	// Transaction
	txBytes, err := hex.DecodeString(txHex)
	if err != nil {
		return nil, fmt.Errorf("decode transaction hex: %w", err)
	}
	tx, err := conway.NewConwayTransactionFromCbor(txBytes)
	if err != nil {
		return nil, err
	}
	// Inputs
	inputsBytes, err := hex.DecodeString(inputsHex)
	if err != nil {
		return nil, fmt.Errorf("decode inputs hex: %w", err)
	}
	var tmpInputs []shelley.ShelleyTransactionInput
	if _, err := cbor.Decode(inputsBytes, &tmpInputs); err != nil {
		return nil, fmt.Errorf("decode inputs: %w", err)
	}
	// Input outputs
	inputOutputsBytes, err := hex.DecodeString(inputOutputsHex)
	if err != nil {
		return nil, fmt.Errorf("decode input outputs hex: %w", err)
	}
	var tmpOutputs []babbage.BabbageTransactionOutput
	if _, err := cbor.Decode(inputOutputsBytes, &tmpOutputs); err != nil {
		return nil, fmt.Errorf("decode input outputs: %w", err)
	}
	// Build resolved inputs
	var resolvedInputs []lcommon.Utxo
	if len(tmpInputs) != len(tmpOutputs) {
		return nil, errors.New("input and output length don't match")
	}
	for i := range tmpInputs {
		resolvedInputs = append(
			resolvedInputs,
			lcommon.Utxo{
				Id:     tmpInputs[i],
				Output: tmpOutputs[i],
			},
		)
	}
	// Build TxInfo
	txInfo := NewTxInfoV3FromTransaction(tx, resolvedInputs)
	return txInfo, nil
}

func TestScriptContextV3SimpleSend(t *testing.T) {
	// NOTE: these values come from the Aiken tests
	// https://github.com/aiken-lang/aiken/blob/af4e04b91e54dbba3340de03fc9e65a90f24a93b/crates/uplc/src/tx/script_context.rs#L1189-L1225
	txInfo, err := buildTxInfoV3(
		`84a70081825820000000000000000000000000000000000000000000000000000000000000000000018182581d60111111111111111111111111111111111111111111111111111111111a3b9aca0002182a0b5820ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff0d818258200000000000000000000000000000000000000000000000000000000000000000001082581d60000000000000000000000000000000000000000000000000000000001a3b9aca001101a20581840000d87980821a000f42401a05f5e100078152510101003222253330044a229309b2b2b9a1f5f6`,
		`81825820000000000000000000000000000000000000000000000000000000000000000000`,
		`81a300581d7039f47fd3b388ef53c48f08de24766d3e55dade6cae908cc24e0f4f3e011a3b9aca00028201d81843d87980`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	// Extract purpose and redeemer from first redeemer in TxInfo
	redeemerPair := txInfo.(TxInfoV3).Redeemers[0]
	purpose := redeemerPair.Key
	redeemer := redeemerPair.Value
	// Build script context
	sc := NewScriptContextV3(txInfo, redeemer, purpose)
	// Read expected structure from file
	expectedBytes, err := os.ReadFile(
		`testdata/simple_send_expected_structure.txt`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	expected := strings.TrimSpace(string(expectedBytes))
	scPd := strings.TrimSpace(formatPlutusData(sc.ToPlutusData()))
	if scPd != expected {
		t.Fatalf(
			"did not get expected structure\n\n     got:\n\n%s\n\n  wanted:\n\n%s",
			scPd,
			expected,
		)
	}
}
