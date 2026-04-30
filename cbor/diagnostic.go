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

const maxDiagnosticNestedLevels = 256

// DiagnosticNode represents a CBOR element with metadata for display.
type DiagnosticNode struct {
	Type       DiagnosticType
	Value      any
	RawBytes   []byte
	Offset     int
	Length     int
	Children   []DiagnosticNode
	Tag        *uint64
	Indefinite bool
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

// DiagnosticOptions controls output formatting.
type DiagnosticOptions struct {
	ShowOffsets   bool
	ShowHex       bool
	IndentString  string
	MaxDepth      int
	MaxArrayItems int
	MaxByteLength int
	CardanoAware  bool
}

func (o *DiagnosticOptions) normalize() {
	if o.IndentString == "" {
		o.IndentString = "  "
	}
}

// ParseDiagnostic parses CBOR data into a DiagnosticNode tree with offset
// tracking.
func ParseDiagnostic(data []byte) (*DiagnosticNode, error) {
	dec, err := NewStreamDecoder(data)
	if err != nil {
		return nil, err
	}
	node, err := parseDiagnosticNode(dec, 0)
	if err != nil {
		return nil, err
	}
	if !dec.EOF() {
		return nil, errors.New("trailing CBOR data after first item")
	}
	return node, nil
}

func parseDiagnosticNode(dec *StreamDecoder, depth int) (*DiagnosticNode, error) {
	if depth > maxDiagnosticNestedLevels {
		return nil, fmt.Errorf(
			"CBOR nesting exceeds max depth of %d",
			maxDiagnosticNestedLevels,
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
		RawBytes: append([]byte(nil), raw...),
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
			child, err := parseDiagnosticNode(dec, depth+1)
			if err != nil {
				return nil, err
			}
			children = append(children, *child)
		}
	} else {
		for idx := 0; idx < length; idx++ {
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
		RawBytes:   append([]byte(nil), data[start:end]...),
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
		for idx := 0; idx < length; idx++ {
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
		RawBytes:   append([]byte(nil), data[start:end]...),
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
		RawBytes: append([]byte(nil), data[start:end]...),
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
		child, err := parseDiagnosticNode(dec, depth+1)
		if err != nil {
			return nil, err
		}
		if child.Indefinite {
			return nil, errors.New("indefinite string chunk cannot be indefinite")
		}
		switch majorType {
		case CborTypeByteString:
			if child.Type != DiagTypeBytes {
				return nil, errors.New("indefinite byte string chunk was not bytes")
			}
			chunk, _ := child.Value.([]byte)
			bytesValue = append(bytesValue, chunk...)
		case CborTypeTextString:
			if child.Type != DiagTypeText {
				return nil, errors.New("indefinite text string chunk was not text")
			}
			chunk, _ := child.Value.(string)
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
		RawBytes:   append([]byte(nil), data[start:end]...),
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
		RawBytes: append([]byte(nil), raw...),
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
		value := int(data[offset+1])<<24 |
			int(data[offset+2])<<16 |
			int(data[offset+3])<<8 |
			int(data[offset+4])
		return value, 5, false, nil
	case additional == 27:
		if offset+8 >= len(data) {
			return 0, 0, false, errors.New("truncated collection length")
		}
		var value uint64
		for i := 1; i <= 8; i++ {
			value = (value << 8) | uint64(data[offset+i])
		}
		if value > uint64(math.MaxInt) {
			return 0, 0, false, errors.New("collection length exceeds int range")
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

// FormatDiagnostic returns RFC 8949 diagnostic notation string.
func (n *DiagnosticNode) FormatDiagnostic(opts DiagnosticOptions) string {
	opts.normalize()
	out := n.formatCompact(opts, 0)
	if opts.ShowOffsets {
		out = fmt.Sprintf(
			"%s  / @%d-%d /",
			out,
			n.Offset,
			n.Offset+n.Length,
		)
	}
	if opts.ShowHex {
		out = fmt.Sprintf("%s  / %x /", out, n.RawBytes)
	}
	return out
}

// FormatDiagnosticPretty returns multi-line indented diagnostic notation.
func (n *DiagnosticNode) FormatDiagnosticPretty(opts DiagnosticOptions) string {
	opts.normalize()
	out := n.formatPretty(opts, 0)
	if opts.ShowHex {
		out = fmt.Sprintf("%s  / %x /", out, n.RawBytes)
	}
	return out
}

func (n *DiagnosticNode) formatCompact(
	opts DiagnosticOptions,
	depth int,
) string {
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return "..."
	}
	switch n.Type {
	case DiagTypeUint, DiagTypeNint:
		return fmt.Sprintf("%v", n.Value)
	case DiagTypeBytes:
		if n.Indefinite {
			items := make([]string, 0)
			for i := range n.Children {
				if opts.MaxArrayItems > 0 && i >= opts.MaxArrayItems {
					items = append(items, "...")
					break
				}
				items = append(items, n.Children[i].formatCompact(opts, depth+1))
			}
			return "(_ " + strings.Join(items, ", ") + ")"
		}
		b, _ := n.Value.([]byte)
		if opts.MaxByteLength > 0 && len(b) > opts.MaxByteLength {
			return fmt.Sprintf("h'%s...'", hex.EncodeToString(b[:opts.MaxByteLength]))
		}
		return fmt.Sprintf("h'%s'", hex.EncodeToString(b))
	case DiagTypeText:
		if n.Indefinite {
			items := make([]string, 0)
			for i := range n.Children {
				if opts.MaxArrayItems > 0 && i >= opts.MaxArrayItems {
					items = append(items, "...")
					break
				}
				items = append(items, n.Children[i].formatCompact(opts, depth+1))
			}
			return "(_ " + strings.Join(items, ", ") + ")"
		}
		s, _ := n.Value.(string)
		return strconv.Quote(s)
	case DiagTypeArray:
		items := make([]string, 0)
		for i := range n.Children {
			if opts.MaxArrayItems > 0 && i >= opts.MaxArrayItems {
				items = append(items, "...")
				break
			}
			items = append(items, n.Children[i].formatCompact(opts, depth+1))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case DiagTypeMap:
		pairs := make([]string, 0)
		for i := 0; i+1 < len(n.Children); i += 2 {
			if opts.MaxArrayItems > 0 && (i/2) >= opts.MaxArrayItems {
				pairs = append(pairs, "...")
				break
			}
			keyStr := n.Children[i].formatCompact(opts, depth+1)
			valStr := n.Children[i+1].formatCompact(opts, depth+1)
			pairs = append(pairs, fmt.Sprintf("%s: %s", keyStr, valStr))
		}
		return "{" + strings.Join(pairs, ", ") + "}"
	case DiagTypeTag:
		tagNum := uint64(0)
		if n.Tag != nil {
			tagNum = *n.Tag
		}
		content := "null"
		if len(n.Children) > 0 {
			content = n.Children[0].formatCompact(opts, depth+1)
		}
		if opts.CardanoAware {
			switch tagNum {
			case CborTagSet:
				return fmt.Sprintf("set(%s)", content)
			case CborTagMap:
				return fmt.Sprintf("map(%s)", content)
			case CborTagCbor:
				return fmt.Sprintf("cbor(%s)", content)
			case CborTagRational:
				return fmt.Sprintf("rational(%s)", content)
			}
		}
		return fmt.Sprintf("%d(%s)", tagNum, content)
	case DiagTypeFloat:
		switch v := n.Value.(type) {
		case float64:
			return strconv.FormatFloat(v, 'g', -1, 64)
		case float32:
			return strconv.FormatFloat(float64(v), 'g', -1, 32)
		default:
			return fmt.Sprintf("%v", n.Value)
		}
	case DiagTypeSimple:
		switch v := n.Value.(type) {
		case nil:
			return "null"
		case bool:
			if v {
				return "true"
			}
			return "false"
		case uint8:
			// CBOR simple values that are not mapped to bool/null/undefined.
			return fmt.Sprintf("simple(%d)", v)
		default:
			return fmt.Sprintf("%v", v)
		}
	default:
		return fmt.Sprintf("%v", n.Value)
	}
}

func (n *DiagnosticNode) formatPretty(
	opts DiagnosticOptions,
	depth int,
) string {
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return "..."
	}
	prefix := strings.Repeat(opts.IndentString, depth)
	if n.Type != DiagTypeArray && n.Type != DiagTypeMap {
		line := n.formatCompact(opts, depth)
		if opts.ShowOffsets {
			return fmt.Sprintf(
				"%s%s  / @%d /",
				prefix,
				line,
				n.Offset,
			)
		}
		return prefix + line
	}
	if n.Type == DiagTypeArray {
		header := prefix + "["
		if opts.ShowOffsets {
			header = fmt.Sprintf("%s  / @%d /", header, n.Offset)
		}
		lines := []string{header}
		limit := len(n.Children)
		if opts.MaxArrayItems > 0 && limit > opts.MaxArrayItems {
			limit = opts.MaxArrayItems
		}
		for i := 0; i < limit; i++ {
			itemLine := n.Children[i].formatPretty(opts, depth+1)
			if i < limit-1 {
				itemLine += ","
			}
			lines = append(lines, itemLine)
		}
		if limit < len(n.Children) {
			lines = append(lines, strings.Repeat(opts.IndentString, depth+1)+"...")
		}
		footer := prefix + "]"
		if opts.ShowOffsets {
			footer = fmt.Sprintf("%s  / end @%d /", footer, n.Offset+n.Length)
		}
		lines = append(lines, footer)
		return strings.Join(lines, "\n")
	}
	header := prefix + "{"
	if opts.ShowOffsets {
		header = fmt.Sprintf("%s  / @%d /", header, n.Offset)
	}
	lines := []string{header}
	pairCount := len(n.Children) / 2
	limit := pairCount
	if opts.MaxArrayItems > 0 && limit > opts.MaxArrayItems {
		limit = opts.MaxArrayItems
	}
	for i := 0; i < limit; i++ {
		keyLine := n.Children[i*2].formatCompact(opts, depth+1)
		valNode := n.Children[i*2+1]
		entryPrefix := strings.Repeat(opts.IndentString, depth+1)
		hasMoreEntries := i < pairCount-1 || limit < pairCount
		if valNode.Type == DiagTypeArray || valNode.Type == DiagTypeMap {
			lines = append(lines, fmt.Sprintf("%s%s:", entryPrefix, keyLine))
			valPretty := valNode.formatPretty(opts, depth+2)
			if hasMoreEntries {
				valPretty += ","
			}
			lines = append(lines, valPretty)
			continue
		}
		valLine := valNode.formatCompact(opts, depth+1)
		entry := fmt.Sprintf("%s%s: %s", entryPrefix, keyLine, valLine)
		if hasMoreEntries {
			entry += ","
		}
		lines = append(lines, entry)
	}
	if limit < pairCount {
		lines = append(lines, strings.Repeat(opts.IndentString, depth+1)+"...")
	}
	footer := prefix + "}"
	if opts.ShowOffsets {
		footer = fmt.Sprintf("%s  / end @%d /", footer, n.Offset+n.Length)
	}
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}
