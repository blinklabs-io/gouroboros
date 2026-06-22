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
	"math"
	"strconv"
	"strings"
	"sync/atomic"
)

// AddressFormatter converts raw address bytes to a human-readable string
// (typically bech32 for Shelley-era and base58 for Byron). It must return
// false on the second value when the input is not recognised as an address.
type AddressFormatter func(addr []byte) (string, bool)

var addressFormatter atomic.Pointer[AddressFormatter]

// RegisterAddressFormatter installs a hook used by AnnotateAddresses and
// the Cardano-aware formatters to convert raw address byte strings into
// their bech32/base58 representation. The ledger/common package registers
// the production formatter in its init(), so consumers that import the
// ledger package get annotations for free; consumers using the cbor
// package in isolation must call this themselves. Passing nil clears the
// hook.
func RegisterAddressFormatter(fn AddressFormatter) {
	if fn == nil {
		addressFormatter.Store(nil)
		return
	}
	addressFormatter.Store(&fn)
}

func formatAddress(raw []byte) (string, bool) {
	ptr := addressFormatter.Load()
	if ptr == nil || *ptr == nil {
		return "", false
	}
	return (*ptr)(raw)
}

// FormatTransactionDiagnostic renders a Cardano transaction
// ([body, witness_set, is_valid?, auxiliary_data?]) using Cardano-aware
// labels for the body/witness keys and the surrounding array positions.
func FormatTransactionDiagnostic(
	txData []byte,
	opts DiagnosticOptions,
) (string, error) {
	node, err := ParseDiagnostic(txData)
	if err != nil {
		return "", err
	}
	if node.Type != DiagTypeArray {
		return "", fmt.Errorf(
			"transaction must be an array, got %s",
			diagTypeName(node.Type),
		)
	}
	opts.normalize()
	opts.CardanoAware = true
	var b strings.Builder
	w := &cardanoWriter{b: &b, opts: opts}
	w.writeTransaction(node, 0)
	return b.String(), nil
}

// FormatBlockDiagnostic renders a Cardano block. The outer CBOR shape is a
// tag-wrapped 2-element array [era, block]. Shelley through Conway blocks use
// [header, tx_bodies, tx_witnesses, auxiliary_data_set, invalid_transactions];
// Dijkstra appends nullable leios_cert and peras_cert slots. Either the wrapped
// form or the inner block array alone is accepted.
func FormatBlockDiagnostic(
	blockData []byte,
	opts DiagnosticOptions,
) (string, error) {
	node, err := ParseDiagnostic(blockData)
	if err != nil {
		return "", err
	}
	inner := node
	era := -1
	if inner.Type == DiagTypeTag {
		// Tag 24 wraps a byte string whose contents are an embedded CBOR
		// item — we have to decode that payload before we can walk it.
		// Other tag wrappers (rare for blocks) are unwrapped to their
		// single CBOR child directly.
		if inner.Tag != nil && *inner.Tag == CborTagCbor {
			if len(inner.Children) == 0 || inner.Children[0].Type != DiagTypeBytes {
				return "", errors.New("tag-24 block payload is not a byte string")
			}
			payload, ok := inner.Children[0].Value.([]byte)
			if !ok {
				return "", errors.New("tag-24 block payload missing bytes")
			}
			decoded, err := ParseDiagnostic(payload)
			if err != nil {
				return "", fmt.Errorf("decode tag-24 block payload: %w", err)
			}
			inner = decoded
		} else {
			if len(inner.Children) == 0 {
				return "", errors.New("empty tag-wrapped block")
			}
			inner = &inner.Children[0]
		}
	}
	if inner.Type == DiagTypeArray && len(inner.Children) == 2 {
		if eraNode := &inner.Children[0]; eraNode.Type == DiagTypeUint {
			if v, ok := eraNode.Value.(uint64); ok && v <= math.MaxInt32 {
				era = int(v) //nolint:gosec // bounded above
			}
		}
		inner = &inner.Children[1]
	}
	if inner.Type != DiagTypeArray {
		return "", fmt.Errorf(
			"block body must be an array, got %s",
			diagTypeName(inner.Type),
		)
	}
	opts.normalize()
	opts.CardanoAware = true
	var b strings.Builder
	w := &cardanoWriter{b: &b, opts: opts}
	w.writeBlock(inner, era, 0)
	return b.String(), nil
}

// FormatPlutusData renders Plutus data using compact constructor notation
// (e.g. Constr_0([...])) when the tag falls in the recognised ranges, and
// falling back to the underlying CBOR otherwise.
func FormatPlutusData(
	data []byte,
	opts DiagnosticOptions,
) (string, error) {
	node, err := ParseDiagnostic(data)
	if err != nil {
		return "", err
	}
	opts.normalize()
	opts.CardanoAware = true
	var b strings.Builder
	w := &cardanoWriter{b: &b, opts: opts}
	w.writePlutusData(node, 0)
	return b.String(), nil
}

// FormatNativeScript renders a native script array using its logic-operator
// name (script_pubkey, script_all, script_any, script_n_of_k,
// invalid_before, invalid_hereafter).
func FormatNativeScript(
	data []byte,
	opts DiagnosticOptions,
) (string, error) {
	node, err := ParseDiagnostic(data)
	if err != nil {
		return "", err
	}
	if node.Type != DiagTypeArray || len(node.Children) == 0 {
		return "", errors.New("native script must be a non-empty array")
	}
	opts.normalize()
	opts.CardanoAware = true
	var b strings.Builder
	w := &cardanoWriter{b: &b, opts: opts}
	w.writeNativeScript(node, 0)
	return b.String(), nil
}

// AnnotateAddresses walks a diagnostic tree and attaches a bech32/base58
// rendering to every byte-string node whose value is recognised as an
// address. The returned node is the same root with annotations appended to
// matching leaves; the original tree is mutated in place for convenience.
// If no address formatter is registered, the tree is returned unchanged.
func AnnotateAddresses(node *DiagnosticNode) *DiagnosticNode {
	if node == nil {
		return nil
	}
	if addressFormatter.Load() == nil {
		return node
	}
	annotateAddresses(node)
	return node
}

func annotateAddresses(n *DiagnosticNode) {
	if n == nil {
		return
	}
	if n.Type == DiagTypeBytes && !n.Indefinite {
		if raw, ok := n.Value.([]byte); ok && looksLikeAddress(raw) {
			if encoded, ok := formatAddress(raw); ok {
				n.Comment = encoded
			}
		}
	}
	for i := range n.Children {
		annotateAddresses(&n.Children[i])
	}
}

// looksLikeAddress is a cheap pre-filter for AnnotateAddresses so we don't
// hand every byte string to the formatter. Shelley-era addresses are 29..57
// bytes (1 header + 28..56 payload); Byron addresses encode to roughly
// 30..200 bytes after base58 decoding. We accept the union with a generous
// upper bound.
func looksLikeAddress(b []byte) bool {
	if len(b) < 29 || len(b) > 200 {
		return false
	}
	return true
}

// diagTypeName returns the string name of a DiagnosticType.
func diagTypeName(t DiagnosticType) string {
	switch t {
	case DiagTypeUint:
		return "uint"
	case DiagTypeNint:
		return "nint"
	case DiagTypeBytes:
		return "bytes"
	case DiagTypeText:
		return "text"
	case DiagTypeArray:
		return "array"
	case DiagTypeMap:
		return "map"
	case DiagTypeTag:
		return "tag"
	case DiagTypeSimple:
		return "simple"
	case DiagTypeFloat:
		return "float"
	}
	return "unknown"
}

// cardanoWriter renders DiagnosticNodes with Cardano-specific labels emitted
// as RFC 8949 / comment / markers on each line. Output is multi-line and
// indented; depth tracks nesting for the indent prefix.
type cardanoWriter struct {
	b    *strings.Builder
	opts DiagnosticOptions
}

func (w *cardanoWriter) indent(depth int) string {
	return strings.Repeat(w.opts.IndentString, depth)
}

func (w *cardanoWriter) writeTransaction(n *DiagnosticNode, depth int) {
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("transaction [")
	w.writeRange(n)
	w.b.WriteString("\n")
	labels := txArrayLabelsFor(n)
	for i := range n.Children {
		label := "extra"
		if i < len(labels) {
			label = labels[i]
		}
		w.b.WriteString(w.indent(depth + 1))
		w.b.WriteString(label)
		w.b.WriteString(": ")
		switch label {
		case "body":
			w.writeLabelledMap(&n.Children[i], CardanoTxBodyLabels, depth+1)
		case "witness_set":
			w.writeLabelledMap(&n.Children[i], CardanoWitnessLabels, depth+1)
		default:
			w.writeValue(&n.Children[i], depth+1)
		}
		if i < len(n.Children)-1 {
			w.b.WriteString(",")
		}
		w.b.WriteString("\n")
	}
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("]")
}

// txArrayLabelsFor returns the field-label slice appropriate for the
// transaction array shape. Alonzo introduced the is_valid bool at index 2,
// shifting auxiliary_data from index 2 to index 3; pre-Alonzo transactions
// are 3-element [body, witness_set, auxiliary_data]. We distinguish on
// length, with a defensive type check on element 2 so a future shape change
// (or a malformed payload) doesn't silently misname fields.
func txArrayLabelsFor(n *DiagnosticNode) []string {
	if len(n.Children) <= 3 {
		return []string{"body", "witness_set", "auxiliary_data"}
	}
	// 4 or more elements: trust the Alonzo+ ordering only if element 2
	// looks like a bool. Otherwise fall back to numeric-suffix labels so
	// an unexpected layout shows up plainly in the output.
	if n.Children[2].Type == DiagTypeSimple {
		if _, ok := n.Children[2].Value.(bool); ok {
			return CardanoTxArrayLabels
		}
	}
	return []string{"body", "witness_set", "field_2", "auxiliary_data"}
}

func (w *cardanoWriter) writeBlock(n *DiagnosticNode, era int, depth int) {
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("block [")
	if era >= 0 {
		fmt.Fprintf(w.b, "  / era %d /", era)
	}
	w.writeRange(n)
	w.b.WriteString("\n")
	for i := range n.Children {
		label := fmt.Sprintf("field_%d", i)
		if i < len(CardanoBlockLabels) {
			label = CardanoBlockLabels[i]
		}
		w.b.WriteString(w.indent(depth + 1))
		w.b.WriteString(label)
		w.b.WriteString(": ")
		w.writeValue(&n.Children[i], depth+1)
		if i < len(n.Children)-1 {
			w.b.WriteString(",")
		}
		w.b.WriteString("\n")
	}
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("]")
}

func (w *cardanoWriter) writeLabelledMap(
	n *DiagnosticNode,
	labels map[int]string,
	depth int,
) {
	if n.Type != DiagTypeMap {
		w.writeValue(n, depth)
		return
	}
	w.b.WriteString("{")
	w.writeRange(n)
	w.b.WriteString("\n")
	pairs := len(n.Children) / 2
	limit := pairs
	if w.opts.MaxArrayItems > 0 && limit > w.opts.MaxArrayItems {
		limit = w.opts.MaxArrayItems
	}
	for i := 0; i < limit; i++ {
		keyNode := &n.Children[i*2]
		valNode := &n.Children[i*2+1]
		w.b.WriteString(w.indent(depth + 1))
		keyStr := keyNode.formatCompact(w.opts, depth+1)
		w.b.WriteString(keyStr)
		w.b.WriteString(": ")
		w.writeValue(valNode, depth+1)
		if label, ok := mapLabelForKey(keyNode, labels); ok {
			fmt.Fprintf(w.b, "  / %s /", label)
		}
		if i < pairs-1 {
			w.b.WriteString(",")
		}
		w.b.WriteString("\n")
	}
	if limit < pairs {
		w.b.WriteString(w.indent(depth + 1))
		w.b.WriteString("...\n")
	}
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("}")
}

// mapLabelForKey returns the Cardano label string for a numeric map key, if
// the key is a uint that maps into the provided label table.
func mapLabelForKey(key *DiagnosticNode, labels map[int]string) (string, bool) {
	if key.Type != DiagTypeUint {
		return "", false
	}
	idx, ok := key.Value.(uint64)
	if !ok || idx > math.MaxInt32 {
		return "", false
	}
	label, ok := labels[int(idx)] //nolint:gosec // bounded above
	return label, ok
}

// writeValue dispatches on node type. Bytes recognised as addresses (or
// carrying a Comment from AnnotateAddresses) get an annotated rendering.
func (w *cardanoWriter) writeValue(n *DiagnosticNode, depth int) {
	switch n.Type {
	case DiagTypeArray:
		w.writeArray(n, depth)
	case DiagTypeMap:
		w.writeMap(n, depth)
	case DiagTypeTag:
		w.writeTag(n, depth)
	case DiagTypeBytes:
		w.writeBytes(n)
	case DiagTypeUint,
		DiagTypeNint,
		DiagTypeText,
		DiagTypeSimple,
		DiagTypeFloat:
		w.b.WriteString(n.formatCompact(w.opts, depth))
	default:
		w.b.WriteString(n.formatCompact(w.opts, depth))
	}
}

func (w *cardanoWriter) writeBytes(n *DiagnosticNode) {
	raw, _ := n.Value.([]byte)
	w.b.WriteString(n.formatCompact(w.opts, 0))
	if comment := w.byteComment(n, raw); comment != "" {
		fmt.Fprintf(w.b, "  / %s /", comment)
	}
}

func (w *cardanoWriter) byteComment(n *DiagnosticNode, raw []byte) string {
	if n.Comment != "" {
		return n.Comment
	}
	if !looksLikeAddress(raw) {
		return ""
	}
	if encoded, ok := formatAddress(raw); ok {
		return encoded
	}
	return ""
}

func (w *cardanoWriter) writeArray(n *DiagnosticNode, depth int) {
	if len(n.Children) == 0 {
		w.b.WriteString("[]")
		return
	}
	w.b.WriteString("[")
	w.writeRange(n)
	w.b.WriteString("\n")
	limit := len(n.Children)
	if w.opts.MaxArrayItems > 0 && limit > w.opts.MaxArrayItems {
		limit = w.opts.MaxArrayItems
	}
	for i := 0; i < limit; i++ {
		w.b.WriteString(w.indent(depth + 1))
		w.writeValue(&n.Children[i], depth+1)
		if i < len(n.Children)-1 {
			w.b.WriteString(",")
		}
		w.b.WriteString("\n")
	}
	if limit < len(n.Children) {
		w.b.WriteString(w.indent(depth + 1))
		w.b.WriteString("...\n")
	}
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("]")
}

func (w *cardanoWriter) writeMap(n *DiagnosticNode, depth int) {
	if len(n.Children) == 0 {
		w.b.WriteString("{}")
		return
	}
	w.b.WriteString("{")
	w.writeRange(n)
	w.b.WriteString("\n")
	pairs := len(n.Children) / 2
	limit := pairs
	if w.opts.MaxArrayItems > 0 && limit > w.opts.MaxArrayItems {
		limit = w.opts.MaxArrayItems
	}
	for i := 0; i < limit; i++ {
		w.b.WriteString(w.indent(depth + 1))
		w.writeValue(&n.Children[i*2], depth+1)
		w.b.WriteString(": ")
		w.writeValue(&n.Children[i*2+1], depth+1)
		if i < pairs-1 {
			w.b.WriteString(",")
		}
		w.b.WriteString("\n")
	}
	if limit < pairs {
		w.b.WriteString(w.indent(depth + 1))
		w.b.WriteString("...\n")
	}
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("}")
}

func (w *cardanoWriter) writeTag(n *DiagnosticNode, depth int) {
	tagNum := uint64(0)
	if n.Tag != nil {
		tagNum = *n.Tag
	}
	if idx, ok := PlutusConstrFromTag(tagNum); ok {
		fmt.Fprintf(w.b, "Constr_%d(", idx)
		if len(n.Children) > 0 {
			w.writeValue(&n.Children[0], depth)
		}
		w.b.WriteString(")")
		return
	}
	w.b.WriteString(n.formatCompact(w.opts, depth))
}

// writeRange appends an "@start-end" comment when ShowOffsets is enabled.
func (w *cardanoWriter) writeRange(n *DiagnosticNode) {
	if !w.opts.ShowOffsets {
		return
	}
	fmt.Fprintf(w.b, "  / @%d-%d /", n.Offset, n.Offset+n.Length)
}

// writePlutusData formats a parsed CBOR node as Plutus data: constructor
// tags render as Constr_N(...) and integer maps/arrays/bytestrings get
// their natural RFC 8949 form, indented for nesting.
func (w *cardanoWriter) writePlutusData(n *DiagnosticNode, depth int) {
	if n.Type == DiagTypeTag && n.Tag != nil {
		if idx, ok := PlutusConstrFromTag(*n.Tag); ok {
			fmt.Fprintf(w.b, "Constr_%d(", idx)
			if len(n.Children) > 0 {
				w.writePlutusData(&n.Children[0], depth)
			}
			w.b.WriteString(")")
			return
		}
		if *n.Tag == PlutusConstrGeneral && len(n.Children) > 0 {
			// Tag 102: [index, fields]
			payload := &n.Children[0]
			if payload.Type == DiagTypeArray && len(payload.Children) == 2 {
				if idxNode := &payload.Children[0]; idxNode.Type == DiagTypeUint {
					if idx, ok := idxNode.Value.(uint64); ok {
						fmt.Fprintf(w.b, "Constr_%d(", idx)
						w.writePlutusData(&payload.Children[1], depth)
						w.b.WriteString(")")
						return
					}
				}
			}
		}
	}
	switch n.Type {
	case DiagTypeArray:
		// Recurse through writePlutusData for each element so nested
		// constructors (including tag 102) keep their Constr_N(...) form.
		if len(n.Children) == 0 {
			w.b.WriteString("[]")
			return
		}
		w.b.WriteString("[")
		w.writeRange(n)
		w.b.WriteString("\n")
		limit := len(n.Children)
		if w.opts.MaxArrayItems > 0 && limit > w.opts.MaxArrayItems {
			limit = w.opts.MaxArrayItems
		}
		for i := 0; i < limit; i++ {
			w.b.WriteString(w.indent(depth + 1))
			w.writePlutusData(&n.Children[i], depth+1)
			if i < len(n.Children)-1 {
				w.b.WriteString(",")
			}
			w.b.WriteString("\n")
		}
		if limit < len(n.Children) {
			w.b.WriteString(w.indent(depth + 1))
			w.b.WriteString("...\n")
		}
		w.b.WriteString(w.indent(depth))
		w.b.WriteString("]")
	case DiagTypeMap:
		// Plutus maps are written as {k: v, ...} but values may themselves
		// be Plutus data: recurse with writePlutusData.
		if len(n.Children) == 0 {
			w.b.WriteString("{}")
			return
		}
		w.b.WriteString("{")
		w.writeRange(n)
		w.b.WriteString("\n")
		pairs := len(n.Children) / 2
		for i := range pairs {
			w.b.WriteString(w.indent(depth + 1))
			w.writePlutusData(&n.Children[i*2], depth+1)
			w.b.WriteString(": ")
			w.writePlutusData(&n.Children[i*2+1], depth+1)
			if i < pairs-1 {
				w.b.WriteString(",")
			}
			w.b.WriteString("\n")
		}
		w.b.WriteString(w.indent(depth))
		w.b.WriteString("}")
	case DiagTypeUint,
		DiagTypeNint,
		DiagTypeBytes,
		DiagTypeText,
		DiagTypeTag,
		DiagTypeSimple,
		DiagTypeFloat:
		w.b.WriteString(n.formatCompact(w.opts, depth))
	default:
		w.b.WriteString(n.formatCompact(w.opts, depth))
	}
}

func (w *cardanoWriter) writeNativeScript(n *DiagnosticNode, depth int) {
	typeID := -1
	if len(n.Children) > 0 && n.Children[0].Type == DiagTypeUint {
		if v, ok := n.Children[0].Value.(uint64); ok && v <= math.MaxInt32 {
			typeID = int(v) //nolint:gosec // bounded above
		}
	}
	label, ok := CardanoNativeScriptLabels[typeID]
	if !ok {
		label = fmt.Sprintf("native_script_unknown_%d", typeID)
	}
	w.b.WriteString(label)
	w.b.WriteString("(")
	switch typeID {
	case 0:
		// [type, key_hash]
		if len(n.Children) >= 2 {
			w.b.WriteString("key_hash: ")
			w.writeValue(&n.Children[1], depth+1)
		}
	case 1, 2:
		// [type, scripts]
		if len(n.Children) >= 2 {
			w.b.WriteString("scripts: ")
			w.writeScriptArray(&n.Children[1], depth+1)
		}
	case 3:
		// [type, n, scripts]
		if len(n.Children) >= 3 {
			fmt.Fprintf(w.b, "n: %s, scripts: ", strconv.FormatUint(uintFromNode(&n.Children[1]), 10))
			w.writeScriptArray(&n.Children[2], depth+1)
		}
	case 4, 5:
		// [type, slot]
		if len(n.Children) >= 2 {
			fmt.Fprintf(w.b, "slot: %s", strconv.FormatUint(uintFromNode(&n.Children[1]), 10))
		}
	default:
		// Unknown — fall back to raw rendering of trailing fields.
		for i := 1; i < len(n.Children); i++ {
			if i > 1 {
				w.b.WriteString(", ")
			}
			w.writeValue(&n.Children[i], depth+1)
		}
	}
	w.b.WriteString(")")
}

func (w *cardanoWriter) writeScriptArray(n *DiagnosticNode, depth int) {
	if n.Type != DiagTypeArray {
		w.writeValue(n, depth)
		return
	}
	if len(n.Children) == 0 {
		w.b.WriteString("[]")
		return
	}
	w.b.WriteString("[\n")
	for i := range n.Children {
		w.b.WriteString(w.indent(depth + 1))
		w.writeNativeScript(&n.Children[i], depth+1)
		if i < len(n.Children)-1 {
			w.b.WriteString(",")
		}
		w.b.WriteString("\n")
	}
	w.b.WriteString(w.indent(depth))
	w.b.WriteString("]")
}

func uintFromNode(n *DiagnosticNode) uint64 {
	if n == nil {
		return 0
	}
	if v, ok := n.Value.(uint64); ok {
		return v
	}
	return 0
}
