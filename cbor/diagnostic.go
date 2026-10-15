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
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DiagnosticNode represents a CBOR element with metadata for display.

type DiagnosticNode struct {
	Type  DiagnosticType
	Value any
	// RawBytes is a read-only view into the parser's owned input buffer.
	// Its capacity ends at the node boundary; copy it before modifying it.
	RawBytes   []byte
	Offset     int
	Length     int
	Children   []DiagnosticNode
	Tag        *uint64
	Indefinite bool
	// Comment is an optional human-readable annotation attached by
	// AnnotateAddresses and similar helpers. Empty by default.
	Comment string
}

type DiagnosticType uint8

const (
	DiagTypeUint DiagnosticType = iota
	DiagTypeNint
	DiagTypeBytes
	DiagTypeText
	DiagTypeArray
	DiagTypeMap
	DiagTypeTag
	DiagTypeSimple
	DiagTypeFloat
)

// DiagnosticOptions controls rendering and optional parsing budgets.
type DiagnosticOptions struct {
	ShowOffsets   bool
	ShowHex       bool
	IndentString  string
	MaxDepth      int
	MaxArrayItems int
	MaxByteLength int
	CardanoAware  bool
	// MaxOutputBytes caps the rendered content of one render. A render that
	// reaches it stops and ends with a short truncation marker, so the
	// returned string can exceed the cap by that marker. A byte or text string
	// that would not fit is not built at all. Zero or negative selects a
	// default proportional to the encoded size of the rendered node, which
	// leaves renders of ordinary inputs untouched but stops a small, deeply
	// nested input from producing output quadratic in its depth.
	MaxOutputBytes int
	ParseLimits    DiagnosticParseLimits
}

// DiagnosticParseLimits bounds tree construction independently of rendering.
// Zero fields select the default diagnostic budget; negative fields are invalid.
// These are inspection budgets, not Cardano ledger or query-response limits.
type DiagnosticParseLimits struct {
	// MaxNodes counts every node, including tags and indefinite chunks.
	MaxNodes int
	// MaxCollectionItems counts array entries, map pairs and string chunks
	// cumulatively across the complete diagnostic operation.
	MaxCollectionItems int
	// MaxRetainedBytes bounds conservative allocation reservations, including
	// owned input, decoder scratch, tree storage and string payload copies.
	MaxRetainedBytes int
	// MaxWorkBytes counts input visits, node visits and payload copy work.
	MaxWorkBytes int
}

func (o *DiagnosticOptions) normalize() {
	if o.IndentString == "" {
		o.IndentString = "  "
	}
}

// ParseDiagnostic parses one complete CBOR item with offset tracking and the
// default diagnostic construction budgets. Use ParseDiagnosticWithLimits to
// configure a larger inspection budget for trusted archival data.
func ParseDiagnostic(data []byte) (*DiagnosticNode, error) {
	return ParseDiagnosticWithLimits(data, DiagnosticParseLimits{})
}

// ParseDiagnosticWithLimits parses one complete item with explicit construction
// budgets. The parser owns one input copy shared by the nodes' raw spans.
func ParseDiagnosticWithLimits(
	data []byte,
	limits DiagnosticParseLimits,
) (*DiagnosticNode, error) {
	node, _, err := parseDiagnosticWithBudget(data, limits, nil)
	return node, err
}

func parseDiagnosticWithBudget(
	data []byte,
	limits DiagnosticParseLimits,
	budget *diagnosticBudget,
) (*DiagnosticNode, *diagnosticBudget, error) {
	dec, err := NewStreamDecoder(data)
	if err != nil {
		return nil, nil, err
	}
	if budget == nil {
		if err := dec.startDiagnostic(limits); err != nil {
			return nil, nil, err
		}
	} else {
		// A tag-24 payload is already owned and charged by the outer tree.
		if err := budget.process(len(data)); err != nil {
			return nil, nil, err
		}
		dec.diagnostic = budget
	}
	node, err := parseDiagnosticNode(dec, 0)
	if err != nil {
		return nil, nil, err
	}
	if !dec.EOF() {
		return nil, nil, errors.New("trailing CBOR data after first item")
	}
	return node, dec.diagnostic, nil
}

func parseDiagnosticNode(
	dec *StreamDecoder,
	depth int,
) (*DiagnosticNode, error) {
	if err := dec.admitDiagnosticNode(); err != nil {
		return nil, err
	}
	if depth > MaxNestedLevels {
		return nil, fmt.Errorf(
			"CBOR nesting exceeds max depth of %d",
			MaxNestedLevels,
		)
	}
	start := dec.Position()
	if start >= len(dec.Data()) {
		return nil, errors.New("unexpected end of CBOR data")
	}
	first := dec.Data()[start]
	majorType := first & CborTypeMask
	additional := first & 0x1f

	switch majorType {
	case 0x00, 0x20:
		return parsePrimitiveDiagnosticNode(dec, majorType)
	case CborTypeByteString, CborTypeTextString:
		if additional == 31 {
			return parseIndefiniteStringDiagnosticNode(dec, majorType, depth)
		}
		return parsePrimitiveDiagnosticNode(dec, majorType)
	case CborTypeArray:
		return parseArrayDiagnosticNode(dec, depth)
	case CborTypeMap:
		return parseMapDiagnosticNode(dec, depth)
	case CborTypeTag:
		return parseTaggedDiagnosticNode(dec, depth)
	case 0xe0:
		return parseSpecialDiagnosticNode(dec, additional)
	default:
		return nil, fmt.Errorf("unsupported CBOR major type: 0x%x", majorType)
	}
}

func parsePrimitiveDiagnosticNode(
	dec *StreamDecoder,
	majorType uint8,
) (*DiagnosticNode, error) {
	start := dec.Position()
	var val any
	_, raw, err := dec.DecodeRaw(&val)
	if err != nil {
		return nil, err
	}
	nodeType := DiagTypeSimple
	switch majorType {
	case 0x00:
		nodeType = DiagTypeUint
	case 0x20:
		nodeType = DiagTypeNint
	case CborTypeByteString:
		nodeType = DiagTypeBytes
	case CborTypeTextString:
		nodeType = DiagTypeText
	}
	return &DiagnosticNode{
		Type:     nodeType,
		Value:    val,
		RawBytes: raw[:len(raw):len(raw)],
		Offset:   start,
		Length:   len(raw),
	}, nil
}

func parseArrayDiagnosticNode(
	dec *StreamDecoder,
	depth int,
) (*DiagnosticNode, error) {
	start := dec.Position()
	data := dec.Data()
	length, headerLen, indefinite, err := parseCollectionHeader(data, start)
	if err != nil {
		return nil, err
	}
	if err := dec.diagnostic.collection(length); err != nil {
		return nil, err
	}
	if err := dec.Advance(headerLen); err != nil {
		return nil, err
	}
	children := []DiagnosticNode{}
	if indefinite {
		for {
			pos := dec.Position()
			if pos >= len(data) {
				return nil, errors.New("unterminated indefinite array")
			}
			if data[pos] == 0xff {
				if err := dec.Advance(1); err != nil {
					return nil, err
				}
				break
			}
			if err := dec.diagnostic.item(); err != nil {
				return nil, err
			}
			child, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			children = append(children, *child)
		}
	} else {
		for range length {
			if err := dec.diagnostic.item(); err != nil {
				return nil, err
			}
			child, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			children = append(children, *child)
		}
	}
	end := dec.Position()
	values := make([]any, len(children))
	for i := range children {
		values[i] = children[i].Value
	}
	return &DiagnosticNode{
		Type:       DiagTypeArray,
		Value:      values,
		RawBytes:   data[start:end:end],
		Offset:     start,
		Length:     end - start,
		Children:   children,
		Indefinite: indefinite,
	}, nil
}

func parseMapDiagnosticNode(
	dec *StreamDecoder,
	depth int,
) (*DiagnosticNode, error) {
	start := dec.Position()
	data := dec.Data()
	length, headerLen, indefinite, err := parseCollectionHeader(data, start)
	if err != nil {
		return nil, err
	}
	if err := dec.diagnostic.collection(length); err != nil {
		return nil, err
	}
	if err := dec.Advance(headerLen); err != nil {
		return nil, err
	}
	children := []DiagnosticNode{}
	pairs := make([][2]any, 0)
	if indefinite {
		for {
			pos := dec.Position()
			if pos >= len(data) {
				return nil, errors.New("unterminated indefinite map")
			}
			if data[pos] == 0xff {
				if err := dec.Advance(1); err != nil {
					return nil, err
				}
				break
			}
			if err := dec.diagnostic.item(); err != nil {
				return nil, err
			}
			keyNode, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			valNode, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			children = append(children, *keyNode, *valNode)
			pairs = append(pairs, [2]any{keyNode.Value, valNode.Value})
		}
	} else {
		for range length {
			if err := dec.diagnostic.item(); err != nil {
				return nil, err
			}
			keyNode, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			valNode, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			children = append(children, *keyNode, *valNode)
			pairs = append(pairs, [2]any{keyNode.Value, valNode.Value})
		}
	}
	end := dec.Position()
	return &DiagnosticNode{
		Type:       DiagTypeMap,
		Value:      pairs,
		RawBytes:   data[start:end:end],
		Offset:     start,
		Length:     end - start,
		Children:   children,
		Indefinite: indefinite,
	}, nil
}

func parseTaggedDiagnosticNode(
	dec *StreamDecoder,
	depth int,
) (*DiagnosticNode, error) {
	start := dec.Position()
	data := dec.Data()
	tagNum, headerLen, err := parseTagHeader(data, start)
	if err != nil {
		return nil, err
	}
	if err := dec.Advance(headerLen); err != nil {
		return nil, err
	}
	child, err := parseDiagnosticNode(dec, depth+1)
	if err != nil {
		return nil, err
	}
	end := dec.Position()
	tagCopy := tagNum
	return &DiagnosticNode{
		Type:     DiagTypeTag,
		Value:    child.Value,
		RawBytes: data[start:end:end],
		Offset:   start,
		Length:   end - start,
		Children: []DiagnosticNode{*child},
		Tag:      &tagCopy,
	}, nil
}

func parseIndefiniteStringDiagnosticNode(
	dec *StreamDecoder,
	majorType uint8,
	depth int,
) (*DiagnosticNode, error) {
	start := dec.Position()
	data := dec.Data()
	if start >= len(data) {
		return nil, errors.New("unexpected end of CBOR data")
	}
	if err := dec.Advance(1); err != nil {
		return nil, err
	}
	children := []DiagnosticNode{}
	var bytesValue []byte
	var textBuilder strings.Builder
	for {
		pos := dec.Position()
		if pos >= len(data) {
			return nil, errors.New("unterminated indefinite string")
		}
		if data[pos] == 0xff {
			if err := dec.Advance(1); err != nil {
				return nil, err
			}
			break
		}
		if data[pos]&31 == 31 {
			return nil, errors.New(
				"indefinite string chunk cannot be indefinite",
			)
		}
		if data[pos]&CborTypeMask != majorType {
			if majorType == CborTypeByteString {
				return nil, errors.New(
					"indefinite byte string chunk was not bytes",
				)
			}
			return nil, errors.New("indefinite text string chunk was not text")
		}
		if err := dec.diagnostic.item(); err != nil {
			return nil, err
		}
		child, err := parseDiagnosticNode(dec, depth+1)
		if err != nil {
			return nil, err
		}
		if child.Indefinite {
			return nil, errors.New(
				"indefinite string chunk cannot be indefinite",
			)
		}
		switch majorType {
		case CborTypeByteString:
			if child.Type != DiagTypeBytes {
				return nil, errors.New(
					"indefinite byte string chunk was not bytes",
				)
			}
			chunk, _ := child.Value.([]byte)
			if err := dec.diagnostic.payload(len(chunk), 2); err != nil {
				return nil, err
			}
			bytesValue = append(bytesValue, chunk...)
		case CborTypeTextString:
			if child.Type != DiagTypeText {
				return nil, errors.New(
					"indefinite text string chunk was not text",
				)
			}
			chunk, _ := child.Value.(string)
			if err := dec.diagnostic.payload(len(chunk), 2); err != nil {
				return nil, err
			}
			textBuilder.WriteString(chunk)
		default:
			return nil, errors.New("invalid indefinite string major type")
		}
		children = append(children, *child)
	}
	end := dec.Position()
	nodeType := DiagTypeBytes
	var value any = bytesValue
	if majorType == CborTypeTextString {
		nodeType = DiagTypeText
		value = textBuilder.String()
	}
	return &DiagnosticNode{
		Type:       nodeType,
		Value:      value,
		RawBytes:   data[start:end:end],
		Offset:     start,
		Length:     end - start,
		Children:   children,
		Indefinite: true,
	}, nil
}

func parseSpecialDiagnosticNode(
	dec *StreamDecoder,
	additional uint8,
) (*DiagnosticNode, error) {
	start := dec.Position()
	var val any
	_, raw, err := dec.DecodeRaw(&val)
	if err != nil {
		return nil, err
	}
	nodeType := DiagTypeSimple
	if additional == 25 || additional == 26 || additional == 27 {
		nodeType = DiagTypeFloat
	}
	return &DiagnosticNode{
		Type:     nodeType,
		Value:    val,
		RawBytes: raw[:len(raw):len(raw)],
		Offset:   start,
		Length:   len(raw),
	}, nil
}

func parseCollectionHeader(
	data []byte,
	offset int,
) (int, int, bool, error) {
	if offset >= len(data) {
		return 0, 0, false, errors.New("unexpected end of CBOR data")
	}
	first := data[offset]
	additional := first & 0x1f
	switch {
	case additional < 24:
		return int(additional), 1, false, nil
	case additional == 24:
		if offset+1 >= len(data) {
			return 0, 0, false, errors.New("truncated collection length")
		}
		return int(data[offset+1]), 2, false, nil
	case additional == 25:
		if offset+2 >= len(data) {
			return 0, 0, false, errors.New("truncated collection length")
		}
		value := int(data[offset+1])<<8 | int(data[offset+2])
		return value, 3, false, nil
	case additional == 26:
		if offset+4 >= len(data) {
			return 0, 0, false, errors.New("truncated collection length")
		}
		value := uint32(data[offset+1])<<24 |
			uint32(data[offset+2])<<16 |
			uint32(data[offset+3])<<8 |
			uint32(data[offset+4])
		if value > uint32(math.MaxInt32) {
			return 0, 0, false, errors.New(
				"collection length exceeds int32 range",
			)
		}
		return int(value), 5, false, nil
	case additional == 27:
		if offset+8 >= len(data) {
			return 0, 0, false, errors.New("truncated collection length")
		}
		var value uint64
		for i := 1; i <= 8; i++ {
			value = (value << 8) | uint64(data[offset+i])
		}
		if value > uint64(math.MaxInt32) {
			return 0, 0, false, errors.New(
				"collection length exceeds int32 range",
			)
		}
		return int(value), 9, false, nil
	case additional == 31:
		return 0, 1, true, nil
	default:
		return 0, 0, false, fmt.Errorf(
			"invalid collection additional info: %d",
			additional,
		)
	}
}

func parseTagHeader(data []byte, offset int) (uint64, int, error) {
	if offset >= len(data) {
		return 0, 0, errors.New("unexpected end of CBOR data")
	}
	first := data[offset]
	if first&CborTypeMask != CborTypeTag {
		return 0, 0, errors.New("not a tag header")
	}
	additional := first & 0x1f
	switch {
	case additional < 24:
		return uint64(additional), 1, nil
	case additional == 24:
		if offset+1 >= len(data) {
			return 0, 0, errors.New("truncated tag number")
		}
		return uint64(data[offset+1]), 2, nil
	case additional == 25:
		if offset+2 >= len(data) {
			return 0, 0, errors.New("truncated tag number")
		}
		value := uint64(data[offset+1])<<8 | uint64(data[offset+2])
		return value, 3, nil
	case additional == 26:
		if offset+4 >= len(data) {
			return 0, 0, errors.New("truncated tag number")
		}
		value := uint64(data[offset+1])<<24 |
			uint64(data[offset+2])<<16 |
			uint64(data[offset+3])<<8 |
			uint64(data[offset+4])
		return value, 5, nil
	case additional == 27:
		if offset+8 >= len(data) {
			return 0, 0, errors.New("truncated tag number")
		}
		var value uint64
		for i := 1; i <= 8; i++ {
			value = (value << 8) | uint64(data[offset+i])
		}
		return value, 9, nil
	default:
		return 0, 0, fmt.Errorf("invalid tag additional info: %d", additional)
	}
}

// GetNodeAtOffset returns the deepest node whose byte range covers offset.
// Returns nil if offset falls outside this node's range.
func (n *DiagnosticNode) GetNodeAtOffset(offset int) *DiagnosticNode {
	if n == nil {
		return nil
	}
	if offset < n.Offset || offset >= n.Offset+n.Length {
		return nil
	}
	for i := range n.Children {
		if child := n.Children[i].GetNodeAtOffset(offset); child != nil {
			return child
		}
	}
	return n
}

// GetPathToOffset returns a slice describing the path from this node to the
// deepest descendant containing offset. Map entries contribute their key as
// a path segment; array entries contribute "[i]". Returns nil if offset is
// outside this node's range.
func (n *DiagnosticNode) GetPathToOffset(offset int) []string {
	if n == nil {
		return nil
	}
	if offset < n.Offset || offset >= n.Offset+n.Length {
		return nil
	}
	return n.collectPath(offset, []string{})
}

// collectPath descends exactly one child per level and never backtracks, so
// it extends the one path slice in place. Copying the path at each level
// would cost time and memory quadratic in the nesting depth.
func (n *DiagnosticNode) collectPath(offset int, path []string) []string {
	for {
		var next *DiagnosticNode
		switch n.Type {
		case DiagTypeArray:
			for i := range n.Children {
				child := &n.Children[i]
				if offset >= child.Offset &&
					offset < child.Offset+child.Length {
					path = append(path, fmt.Sprintf("[%d]", i))
					next = child
					break
				}
			}
		case DiagTypeMap:
			for i := 0; i+1 < len(n.Children); i += 2 {
				keyChild := &n.Children[i]
				valChild := &n.Children[i+1]
				if offset >= keyChild.Offset &&
					offset < keyChild.Offset+keyChild.Length {
					path = append(path, mapKeyPathSegment(keyChild))
					next = keyChild
					break
				}
				if offset >= valChild.Offset &&
					offset < valChild.Offset+valChild.Length {
					path = append(path, mapKeyPathSegment(keyChild))
					next = valChild
					break
				}
			}
		case DiagTypeTag:
			if len(n.Children) > 0 {
				child := &n.Children[0]
				if offset >= child.Offset &&
					offset < child.Offset+child.Length {
					next = child
				}
			}
		case DiagTypeUint,
			DiagTypeNint,
			DiagTypeBytes,
			DiagTypeText,
			DiagTypeSimple,
			DiagTypeFloat:
			// Leaf types contribute no further path segments.
		}
		if next == nil {
			return path
		}
		n = next
	}
}

func mapKeyPathSegment(n *DiagnosticNode) string {
	switch n.Type {
	case DiagTypeText:
		s, _ := n.Value.(string)
		return s
	case DiagTypeUint, DiagTypeNint:
		return fmt.Sprintf("%v", n.Value)
	case DiagTypeBytes:
		b, _ := n.Value.([]byte)
		return "h'" + hex.EncodeToString(b) + "'"
	case DiagTypeArray,
		DiagTypeMap,
		DiagTypeTag,
		DiagTypeSimple,
		DiagTypeFloat:
		return n.formatCompact(DiagnosticOptions{}, 0)
	}
	return n.formatCompact(DiagnosticOptions{}, 0)
}

// FormatHexDump returns a hex dump with CBOR structure annotations.
// Each row pairs an offset and the relevant bytes with the structural meaning
// of those bytes, indented to reflect nesting. Output past
// opts.MaxOutputBytes is replaced by a truncation marker.
func (n *DiagnosticNode) FormatHexDump(opts DiagnosticOptions) string {
	opts.normalize()
	w := newDiagnosticWriter(opts, n)
	w.printf("%-8s%-42s%s\n", "Offset", "Hex", "Structure")
	w.printf(
		"%-8s%-42s%s\n",
		"------",
		strings.Repeat("-", 40),
		"------------------",
	)
	n.writeHexDump(w, opts, 0, "")
	out := w.result("")
	if w.truncated {
		out += "\n"
	}
	return out
}

func (n *DiagnosticNode) writeHexDump(
	w *diagnosticWriter,
	opts DiagnosticOptions,
	depth int,
	role string,
) {
	if w.truncated {
		return
	}
	indent := strings.Repeat("| ", depth)
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		w.printf(
			"%-8s%-42s%s\n",
			fmt.Sprintf("%04x", n.Offset),
			"...",
			indent+"...",
		)
		return
	}

	headerLen := min(n.cborHeaderLen(), len(n.RawBytes))
	headerBytes := n.RawBytes[:headerLen]

	label := n.structureLabel()
	if role != "" {
		label = role + ": " + label
	}
	w.printf(
		"%-8s%-42s%s\n",
		fmt.Sprintf("%04x", n.Offset),
		formatHexBytes(headerBytes, 0),
		indent+label,
	)

	if !n.Indefinite &&
		(n.Type == DiagTypeBytes || n.Type == DiagTypeText) &&
		len(n.RawBytes) > headerLen {
		contentBytes := n.RawBytes[headerLen:]
		contentOffset := n.Offset + headerLen
		maxShow := opts.MaxByteLength
		if maxShow <= 0 {
			maxShow = 4
		}
		w.printf(
			"%-8s%-42s%s\n",
			fmt.Sprintf("%04x", contentOffset),
			formatHexBytes(contentBytes, maxShow),
			indent,
		)
	}

	switch n.Type {
	case DiagTypeArray:
		limit := len(n.Children)
		if opts.MaxArrayItems > 0 && limit > opts.MaxArrayItems {
			limit = opts.MaxArrayItems
		}
		for i := 0; i < limit; i++ {
			n.Children[i].writeHexDump(w, opts, depth+1, "")
		}
		if limit < len(n.Children) {
			w.printf(
				"%-8s%-42s%s\n",
				"",
				"...",
				strings.Repeat("| ", depth+1)+"...",
			)
		}
	case DiagTypeMap:
		pairCount := len(n.Children) / 2
		limit := pairCount
		if opts.MaxArrayItems > 0 && limit > opts.MaxArrayItems {
			limit = opts.MaxArrayItems
		}
		for i := 0; i < limit; i++ {
			n.Children[i*2].writeHexDump(w, opts, depth+1, "key")
			n.Children[i*2+1].writeHexDump(w, opts, depth+1, "value")
		}
		if limit < pairCount {
			w.printf(
				"%-8s%-42s%s\n",
				"",
				"...",
				strings.Repeat("| ", depth+1)+"...",
			)
		}
	case DiagTypeTag:
		if len(n.Children) > 0 {
			n.Children[0].writeHexDump(w, opts, depth+1, "")
		}
	case DiagTypeBytes, DiagTypeText:
		if n.Indefinite {
			for i := range n.Children {
				n.Children[i].writeHexDump(w, opts, depth+1, "")
			}
		}
	case DiagTypeUint, DiagTypeNint, DiagTypeSimple, DiagTypeFloat:
		// Primitive leaves; the header line already covered the value.
	}
}

// cborHeaderLen returns the length of the CBOR header (initial byte plus any
// length/value bytes encoded in the additional info) for this node. For
// primitive items this equals the entire encoded length; for collections,
// strings, and tags it covers only the header portion.
func (n *DiagnosticNode) cborHeaderLen() int {
	if len(n.RawBytes) == 0 {
		return 0
	}
	additional := n.RawBytes[0] & 0x1f
	switch {
	case additional < 24:
		return 1
	case additional == 24:
		return 2
	case additional == 25:
		return 3
	case additional == 26:
		return 5
	case additional == 27:
		return 9
	default:
		return 1
	}
}

func (n *DiagnosticNode) structureLabel() string {
	switch n.Type {
	case DiagTypeUint, DiagTypeNint:
		return fmt.Sprintf("%v", n.Value)
	case DiagTypeBytes:
		if n.Indefinite {
			return "bytes(_)"
		}
		b, _ := n.Value.([]byte)
		return fmt.Sprintf("bytes(%d)", len(b))
	case DiagTypeText:
		if n.Indefinite {
			return "text(_)"
		}
		s, _ := n.Value.(string)
		if len(s) > 0 && len(s) < 32 {
			return strconv.Quote(s)
		}
		return fmt.Sprintf("text(%d)", len(s))
	case DiagTypeArray:
		if n.Indefinite {
			return "array(_)"
		}
		return fmt.Sprintf("array(%d)", len(n.Children))
	case DiagTypeMap:
		if n.Indefinite {
			return "map(_)"
		}
		return fmt.Sprintf("map(%d)", len(n.Children)/2)
	case DiagTypeTag:
		tagNum := uint64(0)
		if n.Tag != nil {
			tagNum = *n.Tag
		}
		return fmt.Sprintf("tag(%d)", tagNum)
	case DiagTypeFloat:
		return fmt.Sprintf("%v", n.Value)
	case DiagTypeSimple:
		switch v := n.Value.(type) {
		case nil:
			return "null"
		case bool:
			if v {
				return "true"
			}
			return "false"
		default:
			return fmt.Sprintf("simple(%v)", v)
		}
	}
	return ""
}

func formatHexBytes(data []byte, maxBytes int) string {
	if maxBytes <= 0 || len(data) <= maxBytes {
		parts := make([]string, len(data))
		for i, b := range data {
			parts[i] = fmt.Sprintf("%02x", b)
		}
		return strings.Join(parts, " ")
	}
	parts := make([]string, maxBytes)
	for i := range maxBytes {
		parts[i] = fmt.Sprintf("%02x", data[i])
	}
	return strings.Join(parts, " ") + fmt.Sprintf(" ... (%d bytes)", len(data))
}

// FormatDiagnostic returns RFC 8949 diagnostic notation string. Output past
// opts.MaxOutputBytes is replaced by a truncation marker.
func (n *DiagnosticNode) FormatDiagnostic(opts DiagnosticOptions) string {
	opts.normalize()
	w := newDiagnosticWriter(opts, n)
	n.writeCompact(w, opts, 0)
	if opts.ShowOffsets {
		w.printf("  / @%d-%d /", n.Offset, n.Offset+n.Length)
	}
	if opts.ShowHex {
		w.writeHexSuffix(n.RawBytes)
	}
	return w.result("")
}

// FormatDiagnosticPretty returns multi-line indented diagnostic notation.
// Output past opts.MaxOutputBytes is replaced by a truncation marker.
func (n *DiagnosticNode) FormatDiagnosticPretty(opts DiagnosticOptions) string {
	opts.normalize()
	w := newDiagnosticWriter(opts, n)
	n.writePretty(w, opts, 0)
	if opts.ShowHex {
		w.writeHexSuffix(n.RawBytes)
	}
	return w.result("\n")
}

// formatCompact renders n on one line within the default output cap.
func (n *DiagnosticNode) formatCompact(
	opts DiagnosticOptions,
	depth int,
) string {
	w := newDiagnosticWriter(opts, n)
	n.writeCompact(w, opts, depth)
	return w.result("")
}

// writeCompact streams n's one-line form into w and reports whether rendering
// may continue. Streaming matters: returning a string per node recopies every
// nested rendering at each enclosing level, quadratic in the nesting depth.
func (n *DiagnosticNode) writeCompact(
	w *diagnosticWriter,
	opts DiagnosticOptions,
	depth int,
) bool {
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return w.write("...")
	}
	switch n.Type {
	case DiagTypeUint, DiagTypeNint:
		return w.printf("%v", n.Value)
	case DiagTypeBytes:
		if n.Indefinite {
			return n.writeCompactChunks(w, opts, depth)
		}
		b, _ := n.Value.([]byte)
		if opts.MaxByteLength > 0 && len(b) > opts.MaxByteLength {
			if !w.fits(hexSize(opts.MaxByteLength, len("h'...'"))) {
				return false
			}
			return w.printf(
				"h'%s...'",
				hex.EncodeToString(b[:opts.MaxByteLength]),
			)
		}
		if !w.fits(hexSize(len(b), len("h''"))) {
			return false
		}
		return w.printf("h'%s'", hex.EncodeToString(b))
	case DiagTypeText:
		if n.Indefinite {
			return n.writeCompactChunks(w, opts, depth)
		}
		s, _ := n.Value.(string)
		// Escape expansion can make the quoted form much larger than the
		// source. Size it before strconv.Quote allocates the result so the
		// output cap also bounds transient rendering memory.
		if !w.fits(quotedStringSize(s)) {
			return false
		}
		return w.write(strconv.Quote(s))
	case DiagTypeArray:
		return w.write("[") &&
			n.writeCompactItems(w, opts, depth) &&
			w.write("]")
	case DiagTypeMap:
		if !w.write("{") {
			return false
		}
		for i := 0; i+1 < len(n.Children); i += 2 {
			if i > 0 && !w.write(", ") {
				return false
			}
			if opts.MaxArrayItems > 0 && (i/2) >= opts.MaxArrayItems {
				if !w.write("...") {
					return false
				}
				break
			}
			if !n.Children[i].writeCompact(w, opts, depth+1) ||
				!w.write(": ") ||
				!n.Children[i+1].writeCompact(w, opts, depth+1) {
				return false
			}
		}
		return w.write("}")
	case DiagTypeTag:
		tagNum := uint64(0)
		if n.Tag != nil {
			tagNum = *n.Tag
		}
		open := fmt.Sprintf("%d(", tagNum)
		if opts.CardanoAware {
			switch tagNum {
			case CborTagSet:
				open = "set("
			case CborTagMap:
				open = "map("
			case CborTagCbor:
				open = "cbor("
			case CborTagRational:
				open = "rational("
			}
		}
		if !w.write(open) {
			return false
		}
		if len(n.Children) > 0 {
			if !n.Children[0].writeCompact(w, opts, depth+1) {
				return false
			}
		} else if !w.write("null") {
			return false
		}
		return w.write(")")
	case DiagTypeFloat:
		switch v := n.Value.(type) {
		case float64:
			return w.write(strconv.FormatFloat(v, 'g', -1, 64))
		case float32:
			return w.write(strconv.FormatFloat(float64(v), 'g', -1, 32))
		default:
			return w.printf("%v", n.Value)
		}
	case DiagTypeSimple:
		switch v := n.Value.(type) {
		case nil:
			return w.write("null")
		case bool:
			if v {
				return w.write("true")
			}
			return w.write("false")
		case uint8:
			// CBOR simple values that are not mapped to bool/null/undefined.
			return w.printf("simple(%d)", v)
		default:
			return w.printf("%v", v)
		}
	default:
		return w.printf("%v", n.Value)
	}
}

// writeCompactChunks renders an indefinite-length string as "(_ chunk, ...)".
func (n *DiagnosticNode) writeCompactChunks(
	w *diagnosticWriter,
	opts DiagnosticOptions,
	depth int,
) bool {
	return w.write("(_ ") &&
		n.writeCompactItems(w, opts, depth) &&
		w.write(")")
}

// writeCompactItems writes n's children separated by ", ", eliding those
// past MaxArrayItems.
func (n *DiagnosticNode) writeCompactItems(
	w *diagnosticWriter,
	opts DiagnosticOptions,
	depth int,
) bool {
	for i := range n.Children {
		if i > 0 && !w.write(", ") {
			return false
		}
		if opts.MaxArrayItems > 0 && i >= opts.MaxArrayItems {
			return w.write("...")
		}
		if !n.Children[i].writeCompact(w, opts, depth+1) {
			return false
		}
	}
	return true
}

// writePretty streams n's multi-line form into w and reports whether
// rendering may continue.
func (n *DiagnosticNode) writePretty(
	w *diagnosticWriter,
	opts DiagnosticOptions,
	depth int,
) bool {
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return w.write("...")
	}
	if !w.writeRepeat(opts.IndentString, depth) {
		return false
	}
	if n.Type != DiagTypeArray && n.Type != DiagTypeMap {
		return n.writeCompact(w, opts, depth) &&
			(!opts.ShowOffsets || w.printf("  / @%d /", n.Offset))
	}
	open, closing, count := "[", "]", len(n.Children)
	if n.Type == DiagTypeMap {
		open, closing, count = "{", "}", len(n.Children)/2
	}
	limit := count
	if opts.MaxArrayItems > 0 && limit > opts.MaxArrayItems {
		limit = opts.MaxArrayItems
	}
	if !w.write(open) ||
		(opts.ShowOffsets && !w.printf("  / @%d /", n.Offset)) {
		return false
	}
	for i := 0; i < limit; i++ {
		if !w.write("\n") ||
			!n.writePrettyEntry(w, opts, depth, i, limit, count) {
			return false
		}
	}
	if limit < count {
		if !w.write("\n") ||
			!w.writeRepeat(opts.IndentString, depth+1) ||
			!w.write("...") {
			return false
		}
	}
	return w.write("\n") &&
		w.writeRepeat(opts.IndentString, depth) &&
		w.write(closing) &&
		(!opts.ShowOffsets || w.printf("  / end @%d /", n.Offset+n.Length))
}

// writePrettyEntry writes entry i of the array or map n, which has count
// entries of which limit are shown. An array item is followed by a comma
// unless it is the last one shown, so an elision marker never follows a comma;
// a map entry is followed by one unless it is the last in the map, so the
// marker does.
func (n *DiagnosticNode) writePrettyEntry(
	w *diagnosticWriter,
	opts DiagnosticOptions,
	depth int,
	i int,
	limit int,
	count int,
) bool {
	if n.Type == DiagTypeArray {
		return n.Children[i].writePretty(w, opts, depth+1) &&
			(i >= limit-1 || w.write(","))
	}
	key, val := &n.Children[i*2], &n.Children[i*2+1]
	if !w.writeRepeat(opts.IndentString, depth+1) ||
		!key.writeCompact(w, opts, depth+1) {
		return false
	}
	if val.Type == DiagTypeArray || val.Type == DiagTypeMap {
		if !w.write(":\n") || !val.writePretty(w, opts, depth+2) {
			return false
		}
	} else if !w.write(": ") || !val.writeCompact(w, opts, depth+1) {
		return false
	}
	return i >= count-1 || w.write(",")
}
