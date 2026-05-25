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

package cbor

import (
	"errors"
	"fmt"
)

// maxKnownEra is the highest era id the validator considers normal. Cardano
// currently runs through Conway (era 7); the cap is loose so a future era
// roll-out doesn't immediately produce warnings, but pathological values
// (e.g. era 99) still get flagged.
const maxKnownEra = 10

// maxDiagnosticInputBytes caps the raw input size accepted by the public
// Diagnose entry points. Tag-24-wrapped Conway blocks top out near 90 KiB
// in practice; the 16 MiB ceiling here is a DoS guard for unattended
// callers (e.g. an HTTP handler running Diagnose on user-supplied bytes),
// not a protocol-level cap. Callers parsing trusted on-disk data that
// genuinely exceeds this should call ParseDiagnostic directly.
const maxDiagnosticInputBytes = 16 * 1024 * 1024

// maxCardanoTxSize is the Cardano mainnet protocol parameter for the
// largest accepted transaction (set on the network as max_tx_size, 16 KiB
// for Conway). DiagnoseTransaction does not reject larger inputs — the
// whole point of a diagnostic is to inspect bytes that might be wrong —
// but it surfaces an oversized transaction as a warning so the caller can
// see *why* a tx might have been refused by the network.
const maxCardanoTxSize = 16384

// DiagnosticResult bundles a parsed CBOR diagnostic tree with the
// statistics, warnings, and any non-fatal errors gathered during parsing.
// The Root is the tree produced by ParseDiagnostic — its offsets are
// relative to the original input — so callers can drive secondary
// formatters (FormatDiagnostic, FormatHexDump, FormatTransactionDiagnostic,
// etc.) directly off it.
type DiagnosticResult struct {
	Root       *DiagnosticNode
	Errors     []DecodeError
	Warnings   []string
	Statistics DiagnosticStats
}

// DiagnosticStats summarises the parsed tree. TotalBytes always equals
// len(input). ElementCount counts every node in the tree (root, array
// items, map keys and values, indefinite-string chunks, tag payloads).
// ByteStringSize and TextStringSize sum the *payload* lengths of byte and
// text strings, not their CBOR-encoded sizes. Indefinite strings are
// counted once via their chunk children, never via the parent's
// concatenated payload (which would otherwise double-count).
type DiagnosticStats struct {
	TotalBytes     int
	ElementCount   int
	MaxDepth       int
	ByteStringSize int
	TextStringSize int
}

// Diagnose performs a full diagnostic parse of the supplied CBOR data and
// returns the resulting tree alongside derived statistics. Parse errors
// are returned as the function's error; non-fatal warnings (shape oddities
// the caller may want to surface) are attached to the result.
//
// Two safety gates apply before tree construction: an input-size guard
// (maxDiagnosticInputBytes, 16 MiB) and the depth cap that
// ParseDiagnostic / parseDiagnosticNode enforce internally
// (maxDiagnosticNestedLevels, 256). Inputs that violate either are
// rejected without producing a partial tree.
//
// opts is part of the signature so callers and the Cardano-aware wrappers
// share a single surface — Diagnose itself does not consult it.
func Diagnose(data []byte, _ DiagnosticOptions) (*DiagnosticResult, error) {
	if len(data) == 0 {
		return nil, errors.New("cbor: empty input")
	}
	if len(data) > maxDiagnosticInputBytes {
		return nil, fmt.Errorf(
			"cbor: input size %d exceeds diagnostic limit of %d bytes",
			len(data),
			maxDiagnosticInputBytes,
		)
	}
	root, err := ParseDiagnostic(data)
	if err != nil {
		return nil, err
	}
	result := &DiagnosticResult{
		Root: root,
		Statistics: DiagnosticStats{
			TotalBytes: len(data),
		},
	}
	collectDiagnosticStats(root, 0, &result.Statistics)
	return result, nil
}

// DiagnoseTransaction parses txData as a Cardano transaction
// ([body, witness_set, is_valid?, auxiliary_data?]). The Root in the
// returned result is the parsed CBOR tree with offsets preserved; address
// byte strings are annotated in place when an AddressFormatter is
// registered. Shape oddities (unexpected element counts, a non-bool at
// the is_valid position) appear as warnings rather than errors so the
// caller can still inspect the bytes.
func DiagnoseTransaction(
	txData []byte,
	opts DiagnosticOptions,
) (*DiagnosticResult, error) {
	result, err := Diagnose(txData, opts)
	if err != nil {
		return nil, err
	}
	if len(txData) > maxCardanoTxSize {
		result.Warnings = append(
			result.Warnings,
			fmt.Sprintf(
				"transaction is %d bytes; exceeds Cardano protocol max_tx_size (%d)",
				len(txData),
				maxCardanoTxSize,
			),
		)
	}
	if result.Root.Type != DiagTypeArray {
		return nil, fmt.Errorf(
			"transaction must be an array, got %s",
			diagTypeName(result.Root.Type),
		)
	}
	// Valid Cardano transaction shapes:
	//   2 elements: [body, witness_set]                                 (rare/partial)
	//   3 elements: [body, witness_set, auxiliary_data]                 (pre-Alonzo)
	//   4 elements: [body, witness_set, is_valid, auxiliary_data]       (Alonzo+)
	// Anything outside 2..4 is a shape mismatch; 4-element shapes must
	// have a bool at index 2 (otherwise the is_valid label would be a lie).
	switch n := len(result.Root.Children); {
	case n < 2, n > 4:
		result.Warnings = append(
			result.Warnings,
			fmt.Sprintf("transaction array has %d elements; expected 2-4", n),
		)
	case n == 2, n == 3:
		// Valid pre-Alonzo / partial shape — nothing to warn about.
	case n == 4:
		isValid := &result.Root.Children[2]
		_, isBool := isValid.Value.(bool)
		if isValid.Type != DiagTypeSimple || !isBool {
			result.Warnings = append(
				result.Warnings,
				"transaction has 4 elements but element 2 is not a bool (is_valid)",
			)
		}
	}
	AnnotateAddresses(result.Root)
	return result, nil
}

// DiagnoseBlock parses blockData as a Cardano block. The outer CBOR shape
// is either a tag-24-wrapped byte string holding the embedded block, a
// 2-element [era, inner] array, or the bare 5-element inner array. The
// Root in the returned result reflects the input as-parsed (so offsets
// stay valid); the inner block's shape is validated to populate warnings
// without rejecting the input outright.
//
// Note: when blockData is tag-24-wrapped, the embedded block is parsed
// into a separate tree that is not attached to result.Root, so address
// annotation only reaches byte strings that live in the outer wrapper.
// Callers that need annotations inside an embedded block should re-invoke
// DiagnoseBlock on the unwrapped payload.
func DiagnoseBlock(
	blockData []byte,
	opts DiagnosticOptions,
) (*DiagnosticResult, error) {
	result, err := Diagnose(blockData, opts)
	if err != nil {
		return nil, err
	}
	inner, era, eraPresent, unwrapErr := unwrapBlockInner(result.Root)
	if unwrapErr != nil {
		return nil, unwrapErr
	}
	if eraPresent && era > maxKnownEra {
		result.Warnings = append(
			result.Warnings,
			fmt.Sprintf("unexpected era id %d", era),
		)
	}
	if inner.Type != DiagTypeArray {
		return nil, fmt.Errorf(
			"block body must be an array, got %s",
			diagTypeName(inner.Type),
		)
	}
	if got := len(inner.Children); got != len(CardanoBlockLabels) {
		result.Warnings = append(
			result.Warnings,
			fmt.Sprintf(
				"block body has %d fields; expected %d",
				got,
				len(CardanoBlockLabels),
			),
		)
	}
	AnnotateAddresses(result.Root)
	return result, nil
}

// unwrapBlockInner peels back the optional tag-24 wrapper and the
// optional [era, inner] outer array. It returns the inner block body,
// the era id as a uint64, an eraPresent flag (false when no era element
// was found), and any fatal error. Only tag 24 (encoded CBOR) is
// accepted as an outer tag; any other tag is rejected so a malformed
// envelope (e.g. tag 99) cannot silently masquerade as a valid block.
// When the tag-24 payload is decoded, the returned inner is from a
// *separate* tree whose offsets are relative to the embedded payload,
// not the original input.
//
// The era is unwrapped whenever the [uint, x] shape is recognised,
// regardless of magnitude — otherwise an overflowing era value would
// leave validation running on the 2-element wrapper and emit a bogus
// "block body has 2 fields" warning.
func unwrapBlockInner(root *DiagnosticNode) (*DiagnosticNode, uint64, bool, error) {
	inner := root
	var era uint64
	eraPresent := false
	if inner.Type == DiagTypeTag {
		if inner.Tag == nil {
			return nil, 0, false, errors.New("tag-wrapped block has no tag number")
		}
		if *inner.Tag != CborTagCbor {
			return nil, 0, false, fmt.Errorf(
				"unsupported block outer tag %d; only tag %d (encoded CBOR) is allowed",
				*inner.Tag,
				CborTagCbor,
			)
		}
		if len(inner.Children) == 0 || inner.Children[0].Type != DiagTypeBytes {
			return nil, 0, false, errors.New("tag-24 block payload is not a byte string")
		}
		payload, ok := inner.Children[0].Value.([]byte)
		if !ok {
			return nil, 0, false, errors.New("tag-24 block payload missing bytes")
		}
		decoded, err := ParseDiagnostic(payload)
		if err != nil {
			return nil, 0, false, fmt.Errorf("decode tag-24 block payload: %w", err)
		}
		inner = decoded
	}
	if inner.Type == DiagTypeArray && len(inner.Children) == 2 {
		if eraNode := &inner.Children[0]; eraNode.Type == DiagTypeUint {
			if v, ok := eraNode.Value.(uint64); ok {
				era = v
				eraPresent = true
				inner = &inner.Children[1]
			}
		}
	}
	return inner, era, eraPresent, nil
}

// collectDiagnosticStats walks the tree and accumulates element counts,
// depth, and aggregate byte/text string sizes into stats.
//
// Indefinite-length strings are tricky: the parent node carries the
// concatenated payload on its Value, while the chunks live as Children
// each with their own slice of that payload. We count via the chunks so
// the totals match the encoded payload bytes exactly once. Definite-
// length strings have no children, so they're counted on the parent.
func collectDiagnosticStats(n *DiagnosticNode, depth int, stats *DiagnosticStats) {
	if n == nil {
		return
	}
	stats.ElementCount++
	if depth > stats.MaxDepth {
		stats.MaxDepth = depth
	}
	switch n.Type {
	case DiagTypeBytes:
		if !n.Indefinite {
			if b, ok := n.Value.([]byte); ok {
				stats.ByteStringSize += len(b)
			}
		}
	case DiagTypeText:
		if !n.Indefinite {
			if s, ok := n.Value.(string); ok {
				stats.TextStringSize += len(s)
			}
		}
	case DiagTypeUint,
		DiagTypeNint,
		DiagTypeArray,
		DiagTypeMap,
		DiagTypeTag,
		DiagTypeSimple,
		DiagTypeFloat:
		// No string payload to accumulate; child walk below handles
		// nested element counts and depth.
	}
	for i := range n.Children {
		collectDiagnosticStats(&n.Children[i], depth+1, stats)
	}
}
