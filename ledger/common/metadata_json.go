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
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// ParseCardanoCLIMetadataJSONNoSchema parses cardano-cli no-schema metadata
// JSON into a metadata map. Top-level object keys are transaction metadata
// labels and must be decimal uint64 values. Lowercase strings with a "0x"
// prefix are decoded as bytes, integer-looking map keys are decoded as ints,
// and other strings are decoded as text.
//
// To attach the parsed metadata to a metadata-only transaction auxiliary data
// value, pass the result to NewShelleyAuxiliaryData and CBOR encode it.
func ParseCardanoCLIMetadataJSONNoSchema(
	data []byte,
) (TransactionMetadatum, error) {
	return parseCardanoCLIMetadataJSON(data, metadataFromJSONNoSchema)
}

// ParseCardanoCLIMetadataJSONDetailedSchema parses cardano-cli
// detailed-schema metadata JSON into a metadata map. Each metadatum object must
// contain exactly one of "int", "bytes", "string", "list", or "map". Detailed
// schema byte strings are hex encoded without a "0x" prefix. Map entries must
// contain exactly "k" and "v" fields, allowing arbitrary metadatum map keys.
//
// To attach the parsed metadata to a metadata-only transaction auxiliary data
// value, pass the result to NewShelleyAuxiliaryData and CBOR encode it.
func ParseCardanoCLIMetadataJSONDetailedSchema(
	data []byte,
) (TransactionMetadatum, error) {
	return parseCardanoCLIMetadataJSON(data, metadataFromJSONDetailedSchema)
}

// NewShelleyAuxiliaryData creates metadata-only Shelley auxiliary data.
// This is also the compact metadata-only auxiliary data representation used by
// later eras when no auxiliary scripts are present.
func NewShelleyAuxiliaryData(
	metadata TransactionMetadatum,
) *ShelleyAuxiliaryData {
	return &ShelleyAuxiliaryData{metadata: metadata}
}

type metadataJSONValueKind uint8

const (
	metadataJSONNull metadataJSONValueKind = iota
	metadataJSONBool
	metadataJSONNumber
	metadataJSONString
	metadataJSONArray
	metadataJSONObject
)

type metadataJSONValue struct {
	kind   metadataJSONValueKind
	number json.Number
	str    string
	array  []metadataJSONValue
	object []metadataJSONMember
}

type metadataJSONMember struct {
	key   string
	value metadataJSONValue
}

type metadataJSONConverter func(
	value metadataJSONValue,
	path string,
) (TransactionMetadatum, error)

func parseCardanoCLIMetadataJSON(
	data []byte,
	convert metadataJSONConverter,
) (TransactionMetadatum, error) {
	root, err := decodeMetadataJSON(data)
	if err != nil {
		return nil, err
	}
	if root.kind != metadataJSONObject {
		return nil, errors.New("metadata JSON top-level value must be an object")
	}
	pairs := make([]MetaPair, 0, len(root.object))
	seenLabels := make(map[uint64]struct{}, len(root.object))
	for _, member := range root.object {
		label, err := parseMetadataJSONLabel(member.key)
		if err != nil {
			return nil, err
		}
		if _, ok := seenLabels[label]; ok {
			return nil, fmt.Errorf("duplicate metadata label %d", label)
		}
		seenLabels[label] = struct{}{}
		value, err := convert(member.value, "$."+member.key)
		if err != nil {
			return nil, fmt.Errorf("metadata label %d: %w", label, err)
		}
		pairs = append(pairs, MetaPair{
			Key:   MetaInt{Value: new(big.Int).SetUint64(label)},
			Value: value,
		})
	}
	return newParsedMetaMap(pairs)
}

func decodeMetadataJSON(data []byte) (metadataJSONValue, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	value, err := readMetadataJSONValue(decoder)
	if err != nil {
		return metadataJSONValue{}, err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return metadataJSONValue{}, err
		}
		return metadataJSONValue{}, fmt.Errorf(
			"unexpected trailing JSON token %v",
			token,
		)
	}
	return value, nil
}

func readMetadataJSONValue(
	decoder *json.Decoder,
) (metadataJSONValue, error) {
	token, err := decoder.Token()
	if err != nil {
		return metadataJSONValue{}, err
	}
	switch value := token.(type) {
	case nil:
		return metadataJSONValue{kind: metadataJSONNull}, nil
	case bool:
		return metadataJSONValue{kind: metadataJSONBool}, nil
	case json.Number:
		return metadataJSONValue{
			kind:   metadataJSONNumber,
			number: value,
		}, nil
	case string:
		return metadataJSONValue{
			kind: metadataJSONString,
			str:  value,
		}, nil
	case json.Delim:
		switch value {
		case '{':
			return readMetadataJSONObject(decoder)
		case '[':
			return readMetadataJSONArray(decoder)
		default:
			return metadataJSONValue{}, fmt.Errorf(
				"unexpected JSON delimiter %q",
				value,
			)
		}
	default:
		return metadataJSONValue{}, fmt.Errorf(
			"unsupported JSON token %T",
			token,
		)
	}
}

func readMetadataJSONObject(
	decoder *json.Decoder,
) (metadataJSONValue, error) {
	members := []metadataJSONMember{}
	seen := map[string]struct{}{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return metadataJSONValue{}, err
		}
		key, ok := token.(string)
		if !ok {
			return metadataJSONValue{}, fmt.Errorf(
				"expected JSON object key, got %T",
				token,
			)
		}
		if _, ok := seen[key]; ok {
			return metadataJSONValue{}, fmt.Errorf(
				"duplicate JSON object key %q",
				key,
			)
		}
		seen[key] = struct{}{}
		value, err := readMetadataJSONValue(decoder)
		if err != nil {
			return metadataJSONValue{}, err
		}
		members = append(members, metadataJSONMember{
			key:   key,
			value: value,
		})
	}
	if err := readMetadataJSONEnd(decoder, '}'); err != nil {
		return metadataJSONValue{}, err
	}
	return metadataJSONValue{
		kind:   metadataJSONObject,
		object: members,
	}, nil
}

func readMetadataJSONArray(
	decoder *json.Decoder,
) (metadataJSONValue, error) {
	items := []metadataJSONValue{}
	for decoder.More() {
		value, err := readMetadataJSONValue(decoder)
		if err != nil {
			return metadataJSONValue{}, err
		}
		items = append(items, value)
	}
	if err := readMetadataJSONEnd(decoder, ']'); err != nil {
		return metadataJSONValue{}, err
	}
	return metadataJSONValue{
		kind:  metadataJSONArray,
		array: items,
	}, nil
}

func readMetadataJSONEnd(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != want {
		return fmt.Errorf("expected JSON delimiter %q, got %v", want, token)
	}
	return nil
}

func parseMetadataJSONLabel(label string) (uint64, error) {
	if label == "" {
		return 0, errors.New("metadata label cannot be empty")
	}
	if len(label) > 1 && label[0] == '0' {
		return 0, fmt.Errorf(
			"metadata label %q has redundant leading zeroes",
			label,
		)
	}
	for _, r := range label {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("metadata label %q is not a uint64", label)
		}
	}
	ret, err := strconv.ParseUint(label, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("metadata label %q is not a uint64", label)
	}
	return ret, nil
}

func metadataFromJSONNoSchema(
	value metadataJSONValue,
	path string,
) (TransactionMetadatum, error) {
	switch value.kind {
	case metadataJSONNull:
		return nil, fmt.Errorf("%s: null is not allowed in metadata JSON", path)
	case metadataJSONBool:
		return nil, fmt.Errorf("%s: bool is not allowed in metadata JSON", path)
	case metadataJSONNumber:
		n, err := parseMetadataJSONInteger(value.number)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		return MetaInt{Value: n}, nil
	case metadataJSONString:
		if bytesValue, ok := parseNoSchemaHexBytes(value.str); ok {
			return MetaBytes{Value: bytesValue}, nil
		}
		return MetaText{Value: value.str}, nil
	case metadataJSONArray:
		items := make([]TransactionMetadatum, 0, len(value.array))
		for idx, item := range value.array {
			md, err := metadataFromJSONNoSchema(
				item,
				fmt.Sprintf("%s[%d]", path, idx),
			)
			if err != nil {
				return nil, err
			}
			items = append(items, md)
		}
		return MetaList{Items: items}, nil
	case metadataJSONObject:
		pairs := make([]MetaPair, 0, len(value.object))
		for _, member := range value.object {
			md, err := metadataFromJSONNoSchema(
				member.value,
				fmt.Sprintf("%s.%s", path, member.key),
			)
			if err != nil {
				return nil, err
			}
			pairs = append(pairs, MetaPair{
				Key:   parseNoSchemaMapKey(member.key),
				Value: md,
			})
		}
		return newParsedMetaMap(pairs)
	default:
		return nil, fmt.Errorf("%s: unsupported JSON value", path)
	}
}

func metadataFromJSONDetailedSchema(
	value metadataJSONValue,
	path string,
) (TransactionMetadatum, error) {
	if value.kind != metadataJSONObject {
		return nil, fmt.Errorf("%s: detailed metadatum must be an object", path)
	}
	if len(value.object) != 1 {
		return nil, fmt.Errorf(
			"%s: detailed metadatum must have exactly one field",
			path,
		)
	}
	member := value.object[0]
	switch member.key {
	case "int":
		if member.value.kind != metadataJSONNumber {
			return nil, fmt.Errorf("%s.int: expected JSON number", path)
		}
		n, err := parseMetadataJSONInteger(member.value.number)
		if err != nil {
			return nil, fmt.Errorf("%s.int: %w", path, err)
		}
		return MetaInt{Value: n}, nil
	case "bytes":
		if member.value.kind != metadataJSONString {
			return nil, fmt.Errorf("%s.bytes: expected JSON string", path)
		}
		bytesValue, err := hex.DecodeString(member.value.str)
		if err != nil {
			return nil, fmt.Errorf("%s.bytes: invalid hex: %w", path, err)
		}
		return MetaBytes{Value: bytesValue}, nil
	case "string":
		if member.value.kind != metadataJSONString {
			return nil, fmt.Errorf("%s.string: expected JSON string", path)
		}
		return MetaText{Value: member.value.str}, nil
	case "list":
		if member.value.kind != metadataJSONArray {
			return nil, fmt.Errorf("%s.list: expected JSON array", path)
		}
		items := make([]TransactionMetadatum, 0, len(member.value.array))
		for idx, item := range member.value.array {
			md, err := metadataFromJSONDetailedSchema(
				item,
				fmt.Sprintf("%s.list[%d]", path, idx),
			)
			if err != nil {
				return nil, err
			}
			items = append(items, md)
		}
		return MetaList{Items: items}, nil
	case "map":
		if member.value.kind != metadataJSONArray {
			return nil, fmt.Errorf("%s.map: expected JSON array", path)
		}
		return metadataMapFromJSONDetailedSchema(member.value.array, path)
	default:
		return nil, fmt.Errorf(
			"%s: unknown detailed metadatum field %q",
			path,
			member.key,
		)
	}
}

func metadataMapFromJSONDetailedSchema(
	entries []metadataJSONValue,
	path string,
) (TransactionMetadatum, error) {
	pairs := make([]MetaPair, 0, len(entries))
	for idx, entry := range entries {
		pairPath := fmt.Sprintf("%s.map[%d]", path, idx)
		if entry.kind != metadataJSONObject {
			return nil, fmt.Errorf("%s: map entry must be an object", pairPath)
		}
		if len(entry.object) != 2 {
			return nil, fmt.Errorf(
				"%s: map entry must contain exactly k and v fields",
				pairPath,
			)
		}
		var (
			keyJSON   *metadataJSONValue
			valueJSON *metadataJSONValue
		)
		for memberIdx := range entry.object {
			member := &entry.object[memberIdx]
			switch member.key {
			case "k":
				keyJSON = &member.value
			case "v":
				valueJSON = &member.value
			default:
				return nil, fmt.Errorf(
					"%s: unexpected map entry field %q",
					pairPath,
					member.key,
				)
			}
		}
		if keyJSON == nil || valueJSON == nil {
			return nil, fmt.Errorf(
				"%s: map entry must contain k and v fields",
				pairPath,
			)
		}
		key, err := metadataFromJSONDetailedSchema(*keyJSON, pairPath+".k")
		if err != nil {
			return nil, err
		}
		value, err := metadataFromJSONDetailedSchema(*valueJSON, pairPath+".v")
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, MetaPair{Key: key, Value: value})
	}
	return newParsedMetaMap(pairs)
}

func parseMetadataJSONInteger(number json.Number) (*big.Int, error) {
	text := number.String()
	if strings.ContainsAny(text, ".eE") {
		return nil, fmt.Errorf("metadata JSON number %q is not an integer", text)
	}
	ret, ok := new(big.Int).SetString(text, 10)
	if !ok {
		return nil, fmt.Errorf("metadata JSON number %q is not an integer", text)
	}
	return ret, nil
}

func parseNoSchemaMapKey(key string) TransactionMetadatum {
	if n, ok := parseNoSchemaMapKeyInteger(key); ok {
		return MetaInt{Value: n}
	}
	if bytesValue, ok := parseNoSchemaHexBytes(key); ok {
		return MetaBytes{Value: bytesValue}
	}
	return MetaText{Value: key}
}

func parseNoSchemaMapKeyInteger(key string) (*big.Int, bool) {
	if key == "" {
		return nil, false
	}
	unsigned := key
	if key[0] == '-' {
		unsigned = key[1:]
		if unsigned == "" {
			return nil, false
		}
	}
	if len(unsigned) > 1 && unsigned[0] == '0' {
		return nil, false
	}
	for _, r := range unsigned {
		if r < '0' || r > '9' {
			return nil, false
		}
	}
	ret, ok := new(big.Int).SetString(key, 10)
	return ret, ok
}

func parseNoSchemaHexBytes(value string) ([]byte, bool) {
	if !strings.HasPrefix(value, "0x") {
		return nil, false
	}
	hexText := value[2:]
	for _, r := range hexText {
		if r >= 'A' && r <= 'F' {
			return nil, false
		}
	}
	ret, err := hex.DecodeString(hexText)
	if err != nil {
		return nil, false
	}
	return ret, true
}

func newParsedMetaMap(pairs []MetaPair) (MetaMap, error) {
	pairs, err := sortMetaMapPairs(pairs)
	if err != nil {
		return MetaMap{}, err
	}
	return MetaMap{Pairs: pairs}, nil
}

func sortMetaMapPairs(pairs []MetaPair) ([]MetaPair, error) {
	type keyedPair struct {
		pair MetaPair
		key  []byte
	}
	ret := make([]keyedPair, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	for idx, pair := range pairs {
		keyCbor, err := cbor.Encode(pair.Key)
		if err != nil {
			return nil, fmt.Errorf("encode metadata map key %d: %w", idx, err)
		}
		key := string(keyCbor)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf(
				"duplicate metadata map key %q at index %d",
				key,
				idx,
			)
		}
		seen[key] = struct{}{}
		ret = append(ret, keyedPair{pair: pair, key: keyCbor})
	}
	sort.Slice(ret, func(i, j int) bool {
		return metaMapKeyLess(ret[i].key, ret[j].key)
	})

	sortedPairs := make([]MetaPair, 0, len(ret))
	for _, pair := range ret {
		sortedPairs = append(sortedPairs, pair.pair)
	}
	return sortedPairs, nil
}
