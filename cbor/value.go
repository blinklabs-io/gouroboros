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
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"math"
	"math/big"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	_cbor "github.com/fxamacker/cbor/v2"
)

// MaxMapKeyItems is the most data items a single composite map key may hold,
// counting every item nested anywhere inside it. A composite key is retained
// whole as the Go map key and carries an identity digest for each of its
// entries while it decodes, so without this bound one key could hold decoded
// memory far beyond any Cardano structure: keys inside a maximum-size
// transaction stay well below it. It is a var so an application decoding
// trusted data with larger keys can raise it.
var MaxMapKeyItems = 65536

// ErrMapKeyItemBudget reports a composite map key holding more than
// MaxMapKeyItems data items.
var ErrMapKeyItemBudget = errors.New("composite map key exceeds item budget")

// Helpful wrapper for parsing arbitrary CBOR data which may contain types that
// cannot be easily represented in Go (such as maps with bytestring keys)
type Value struct {
	value any
	// We store this as a string so that the type is still hashable for use as map keys
	cborData string
}

func (v *Value) MarshalCBOR() ([]byte, error) {
	// Return stored CBOR
	// This is only a stopgap, since it doesn't allow us to build values from scratch
	return []byte(v.cborData), nil
}

func (v *Value) UnmarshalCBOR(data []byte) error {
	if len(data) == 0 {
		return errors.New("empty CBOR data")
	}
	decMode, err := getDecMode()
	if err != nil {
		return err
	}
	if decMode == nil {
		return errors.New("CBOR decoder mode not initialized")
	}
	_, err = v.unmarshalCBOR(
		data,
		true,
		nil,
		nil,
		0,
		rejectDuplicateMapKeys,
		decMode,
	)
	return err
}

func (v *Value) unmarshalCBOR(
	data []byte,
	retainCbor bool,
	mapKeyDigestOut *[sha256.Size]byte,
	keyItems *int,
	depth int,
	duplicateKeyPolicy duplicateMapKeyPolicy,
	decMode _cbor.DecMode,
) (int, error) {
	if len(data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	if depth > MaxNestedLevels {
		return 0, fmt.Errorf(
			"exceeded maximum CBOR nesting depth: %d",
			MaxNestedLevels,
		)
	}
	if retainCbor {
		// The custom walker must inherit the caller's limits before allocating.
		// Decode consumes one item, so bytes after that item remain allowed.
		if err := decMode.Wellformed(data); err != nil {
			var extra *_cbor.ExtraneousDataError
			if !errors.As(err, &extra) {
				return 0, err
			}
			dec := decMode.NewDecoder(bytes.NewReader(data))
			if err := dec.Skip(); err != nil {
				return 0, err
			}
			data = data[:dec.NumBytesRead()]
		}
		// Save the original CBOR
		v.cborData = string(data)
	} else {
		v.cborData = ""
	}
	cborType := data[0] & CborTypeMask
	switch cborType {
	case CborTypeMap:
		return v.processMap(
			data,
			mapKeyDigestOut,
			keyItems,
			depth,
			duplicateKeyPolicy,
			decMode,
		)
	case CborTypeArray:
		return v.processArray(
			data,
			mapKeyDigestOut,
			keyItems,
			depth,
			duplicateKeyPolicy,
			decMode,
		)
	case CborTypeTextString:
		var tmpValue string
		decodedLength, err := decodeWithMode(
			data,
			&tmpValue,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		setMapKeyDigest(mapKeyDigestOut, v.value)
		return decodedLength, nil
	case CborTypeByteString:
		// Use our custom type which stores the bytestring in a way that allows it to be used as a map key
		var tmpValue ByteString
		decodedLength, err := decodeWithMode(
			data,
			&tmpValue,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		setMapKeyDigest(mapKeyDigestOut, v.value)
		return decodedLength, nil
	case CborTypeTag:
		if keyItems != nil {
			// Tag content is decoded by the library rather than by this
			// walker, so its items are counted before anything is decoded.
			// The tag itself was already charged by its parent.
			items, err := countItems(data, *keyItems+1)
			if err != nil {
				if errors.Is(err, ErrItemBudget) {
					return 0, fmt.Errorf(
						"%w of %d",
						ErrMapKeyItemBudget,
						MaxMapKeyItems,
					)
				}
				return 0, err
			}
			*keyItems -= items - 1
		}
		// Parse as a raw tag to get number and nested CBOR data
		tmpTag := RawTag{}
		decodedLength, err := decodeWithMode(
			data,
			&tmpTag,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		if IsAlternativeTag(tmpTag.Number) {
			// Constructors/alternatives
			var tmpConstr ConstructorDecoder
			if _, err := decodeWithMode(
				data,
				&tmpConstr,
				decMode,
				duplicateKeyPolicy,
			); err != nil {
				return 0, err
			}
			v.value = tmpConstr
		} else {
			// Fall back to standard CBOR tag parsing for our supported types
			var tmpTagDecode any
			if _, err := decodeWithMode(
				data,
				&tmpTagDecode,
				decMode,
				duplicateKeyPolicy,
			); err != nil {
				return 0, err
			}
			v.value = tmpTagDecode
		}
		setMapKeyDigest(mapKeyDigestOut, v.value)
		return decodedLength, nil
	default:
		var tmpValue any
		decodedLength, err := decodeWithMode(
			data,
			&tmpValue,
			decMode,
			duplicateKeyPolicy,
		)
		if err != nil {
			return 0, err
		}
		v.value = tmpValue
		setMapKeyDigest(mapKeyDigestOut, v.value)
		return decodedLength, nil
	}
}

// spendMapKeyItem charges one data item to a composite map key's budget. A
// nil budget means the item is not inside a map key.
func spendMapKeyItem(keyItems *int) error {
	if keyItems == nil {
		return nil
	}
	if *keyItems <= 0 {
		return fmt.Errorf("%w of %d", ErrMapKeyItemBudget, MaxMapKeyItems)
	}
	*keyItems--
	return nil
}

func setMapKeyDigest(out *[sha256.Size]byte, value any) {
	if out != nil {
		*out = mapKeyDigest(value)
	}
}

func (v *Value) Cbor() []byte {
	return []byte(v.cborData)
}

func (v *Value) Value() any {
	return v.value
}

func (v Value) MarshalJSON() ([]byte, error) {
	cborHex := hex.EncodeToString([]byte(v.cborData))
	buf := make([]byte, 0, len(cborHex)+64)
	buf = append(buf, `{"cbor":"`...)
	buf = append(buf, cborHex...)
	buf = append(buf, '"')
	if v.value != nil {
		buf = append(buf, `,"json":`...)
		var err error
		if buf, err = appendAstJson(buf, v.value); err != nil {
			return nil, err
		}
	}
	return append(buf, '}'), nil
}

func (v *Value) processMap(
	data []byte,
	mapKeyDigestOut *[sha256.Size]byte,
	keyItems *int,
	depth int,
	duplicateKeyPolicy duplicateMapKeyPolicy,
	decMode _cbor.DecMode,
) (decodedLength int, err error) {
	// There are certain types that cannot be used as map keys in Go but are valid in CBOR. Trying to
	// parse CBOR containing a map with keys of one of those types will cause a panic. We setup this
	// deferred function to recover from a possible panic and return an error
	defer func() {
		if r := recover(); r != nil {
			if !isUnhashableMapKeyPanic(r) {
				panic(r)
			}
			err = fmt.Errorf(
				"decode failure, probably due to type unsupported by Go: %v",
				r,
			)
		}
	}()
	itemCount, headerLength, indefinite := MapInfo(data)
	if itemCount < 0 {
		return 0, errors.New("invalid CBOR map header")
	}
	newValue := map[any]any{}
	seenKeys := make(map[string]any)
	var digestEntries map[string][sha256.Size * 2]byte
	if mapKeyDigestOut != nil {
		digestEntries = make(map[string][sha256.Size * 2]byte)
	}
	position := int(headerLength)
	for itemIndex := 0; indefinite || itemIndex < itemCount; itemIndex++ {
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		if indefinite && data[position] == 0xff {
			position++
			v.value = newValue
			if mapKeyDigestOut != nil {
				*mapKeyDigestOut = mapKeyMapDigest('m', digestEntries)
			}
			return position, nil
		}

		if err := spendMapKeyItem(keyItems); err != nil {
			return 0, err
		}
		var (
			key       Value
			keyDigest [sha256.Size]byte
		)
		// A key nested in a composite key spends the enclosing key's budget.
		childKeyItems := keyItems
		if childKeyItems == nil {
			remaining := MaxMapKeyItems
			childKeyItems = &remaining
		}
		keyLength, keyErr := key.unmarshalCBOR(
			data[position:],
			false,
			&keyDigest,
			childKeyItems,
			depth+1,
			duplicateKeyPolicy,
			decMode,
		)
		if keyErr != nil {
			return 0, keyErr
		}
		position += keyLength
		keyValue := key.Value()
		keyComparable := isReflexiveMapKey(keyValue)
		keyIdentity := string(keyDigest[:])
		storageKey, duplicate := seenKeys[keyIdentity]
		if duplicate && duplicateKeyPolicy == rejectDuplicateMapKeys {
			return 0, &_cbor.DupMapKeyError{
				Key:   keyValue,
				Index: itemIndex,
			}
		}
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}

		var (
			value          Value
			valueDigest    [sha256.Size]byte
			valueDigestOut *[sha256.Size]byte
		)
		if mapKeyDigestOut != nil {
			valueDigestOut = &valueDigest
		}
		if err := spendMapKeyItem(keyItems); err != nil {
			return 0, err
		}
		valueLength, valueErr := value.unmarshalCBOR(
			data[position:],
			false,
			valueDigestOut,
			keyItems,
			depth+1,
			duplicateKeyPolicy,
			decMode,
		)
		if valueErr != nil {
			return 0, valueErr
		}
		position += valueLength
		if digestEntries != nil {
			var entry [sha256.Size * 2]byte
			copy(entry[:sha256.Size], keyDigest[:])
			copy(entry[sha256.Size:], valueDigest[:])
			digestEntries[keyIdentity] = entry
		}

		if duplicate {
			newValue[storageKey] = value.Value()
			continue
		}

		newStorageKey := keyValue
		if !keyComparable {
			// Use a pointer for unhashable keys and values, such as NaN, that do
			// not compare equal to themselves.
			newStorageKey = &keyValue
		}
		seenKeys[keyIdentity] = newStorageKey
		newValue[newStorageKey] = value.Value()
	}
	v.value = newValue
	if mapKeyDigestOut != nil {
		*mapKeyDigestOut = mapKeyMapDigest('m', digestEntries)
	}
	return position, nil
}

func (v *Value) processArray(
	data []byte,
	mapKeyDigestOut *[sha256.Size]byte,
	keyItems *int,
	depth int,
	duplicateKeyPolicy duplicateMapKeyPolicy,
	decMode _cbor.DecMode,
) (int, error) {
	itemCount, headerLength, indefinite := ArrayInfo(data)
	if itemCount < 0 {
		return 0, errors.New("invalid CBOR array header")
	}
	newValue := []any{}
	// Grown by append: the declared count is untrusted until entries decode.
	var childDigests [][sha256.Size]byte
	position := int(headerLength)
	for itemIndex := 0; indefinite || itemIndex < itemCount; itemIndex++ {
		if position >= len(data) {
			return 0, io.ErrUnexpectedEOF
		}
		if indefinite && data[position] == 0xff {
			position++
			v.value = newValue
			if mapKeyDigestOut != nil {
				*mapKeyDigestOut = mapKeySequenceDigest('a', childDigests)
			}
			return position, nil
		}

		var (
			value          Value
			valueDigest    [sha256.Size]byte
			valueDigestOut *[sha256.Size]byte
		)
		if mapKeyDigestOut != nil {
			valueDigestOut = &valueDigest
		}
		if err := spendMapKeyItem(keyItems); err != nil {
			return 0, err
		}
		valueLength, err := value.unmarshalCBOR(
			data[position:],
			false,
			valueDigestOut,
			keyItems,
			depth+1,
			duplicateKeyPolicy,
			decMode,
		)
		if err != nil {
			return 0, err
		}
		position += valueLength
		newValue = append(newValue, value.Value())
		if mapKeyDigestOut != nil {
			childDigests = append(childDigests, valueDigest)
		}
	}
	v.value = newValue
	if mapKeyDigestOut != nil {
		*mapKeyDigestOut = mapKeySequenceDigest('a', childDigests)
	}
	return position, nil
}

func isReflexiveMapKey(key any) bool {
	if key == nil {
		return true
	}
	return reflect.ValueOf(key).Comparable() && key == key
}

func mapKeyDigest(key any) [sha256.Size]byte {
	digest := sha256.New()
	appendMapKeyDigest(digest, key)
	var ret [sha256.Size]byte
	copy(ret[:], digest.Sum(nil))
	return ret
}

func mapKeySequenceDigest(
	marker byte,
	children [][sha256.Size]byte,
) [sha256.Size]byte {
	digest := sha256.New()
	writeMapKeyDigestUint(digest, marker, uint64(len(children)))
	for _, child := range children {
		_, _ = digest.Write(child[:])
	}
	return sumMapKeyDigest(digest)
}

func mapKeyMapDigest(
	marker byte,
	entriesByKey map[string][sha256.Size * 2]byte,
) [sha256.Size]byte {
	entries := make([][sha256.Size * 2]byte, 0, len(entriesByKey))
	for _, entry := range entriesByKey {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i][:], entries[j][:]) < 0
	})
	digest := sha256.New()
	writeMapKeyDigestMap(digest, marker, entries)
	return sumMapKeyDigest(digest)
}

func sumMapKeyDigest(digest hash.Hash) [sha256.Size]byte {
	var ret [sha256.Size]byte
	copy(ret[:], digest.Sum(nil))
	return ret
}

func writeMapKeyDigestUint(digest hash.Hash, marker byte, value uint64) {
	_, _ = digest.Write([]byte{marker})
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = digest.Write(encoded[:])
}

func writeMapKeyDigestMap(
	digest hash.Hash,
	marker byte,
	entries [][sha256.Size * 2]byte,
) {
	writeMapKeyDigestUint(digest, marker, uint64(len(entries)))
	for _, entry := range entries {
		_, _ = digest.Write(entry[:])
	}
}

func appendMapKeyDigest(digest hash.Hash, key any) {
	writeBytes := func(marker byte, value []byte) {
		_, _ = digest.Write([]byte{marker})
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = digest.Write(length[:])
		_, _ = digest.Write(value)
	}
	writeUint := func(marker byte, value uint64) {
		_, _ = digest.Write([]byte{marker})
		var encoded [8]byte
		binary.BigEndian.PutUint64(encoded[:], value)
		_, _ = digest.Write(encoded[:])
	}
	writeSequence := func(marker byte, values []any) {
		writeUint(marker, uint64(len(values)))
		for _, value := range values {
			childDigest := mapKeyDigest(value)
			_, _ = digest.Write(childDigest[:])
		}
	}
	writeMap := func(marker byte, values map[any]any) {
		entries := make([][sha256.Size * 2]byte, 0, len(values))
		for key, value := range values {
			keyDigest := mapKeyDigest(key)
			valueDigest := mapKeyDigest(value)
			var entry [sha256.Size * 2]byte
			copy(entry[:sha256.Size], keyDigest[:])
			copy(entry[sha256.Size:], valueDigest[:])
			entries = append(entries, entry)
		}
		sort.Slice(entries, func(i, j int) bool {
			return bytes.Compare(entries[i][:], entries[j][:]) < 0
		})
		writeMapKeyDigestMap(digest, marker, entries)
	}

	switch key := key.(type) {
	case nil:
		_, _ = digest.Write([]byte{'n'})
	case bool:
		if key {
			_, _ = digest.Write([]byte{'b', 1})
		} else {
			_, _ = digest.Write([]byte{'b', 0})
		}
	case uint64:
		writeBytes('i', []byte(strconv.FormatUint(key, 10)))
	case int64:
		writeBytes('i', []byte(strconv.FormatInt(key, 10)))
	case float64:
		bits := math.Float64bits(key)
		if key == 0 {
			bits = 0
		}
		writeUint('f', bits)
	case string:
		writeBytes('s', []byte(key))
	case ByteString:
		writeBytes('x', key.Bytes())
	case []byte:
		writeBytes('x', key)
	case WrappedCbor:
		writeBytes('w', key)
	case big.Int:
		writeBytes('i', []byte(key.String()))
	case *big.Int:
		if key == nil {
			_, _ = digest.Write([]byte{'G', 0})
		} else {
			writeBytes('i', []byte(key.String()))
		}
	case []any:
		writeSequence('a', key)
	case Set:
		writeSequence('e', []any(key))
	case map[any]any:
		writeMap('m', key)
	case Map:
		writeMap('M', map[any]any(key))
	case *any:
		if key == nil {
			_, _ = digest.Write([]byte{'p', 0})
		} else {
			appendMapKeyDigest(digest, *key)
		}
	case ConstructorDecoder:
		writeUint('c', uint64(key.Tag()))
		writeBytes('C', key.Fields())
	case _cbor.Tag:
		writeUint('g', key.Number)
		appendMapKeyDigest(digest, key.Content)
	case RawTag:
		writeUint('t', key.Number)
		writeBytes('T', key.Content)
	case Rat:
		if key.Rat == nil {
			_, _ = digest.Write([]byte{'r', 0})
		} else {
			writeBytes('r', []byte(key.RatString()))
		}
	default:
		writeBytes('?', []byte(fmt.Sprintf("%T:%#v", key, key)))
	}
}

func isUnhashableMapKeyPanic(r any) bool {
	runtimeErr, ok := r.(runtime.Error)
	if !ok {
		return false
	}
	return strings.Contains(runtimeErr.Error(), "hash of unhashable type")
}

// appendAstJson appends the Cardano-style AST JSON rendering of obj to buf.
// Children are written straight into the caller's buffer so total work is
// proportional to the output size rather than to depth times output size.
func appendAstJson(buf []byte, obj any) ([]byte, error) {
	switch v := obj.(type) {
	case []byte:
		return appendAstJsonBytes(buf, v), nil
	case ByteString:
		return appendAstJsonBytes(buf, v.Bytes()), nil
	case WrappedCbor:
		return appendAstJsonBytes(buf, v.Bytes()), nil
	case []any:
		return appendAstJsonList(buf, v)
	case Set:
		return appendAstJsonList(buf, v)
	case map[any]any:
		return appendAstJsonMap(buf, v)
	case Map:
		return appendAstJsonMap(buf, v)
	case ConstructorDecoder:
		return v.appendAstJson(buf)
	case big.Int:
		buf = append(buf, `{"int":`...)
		buf = v.Append(buf, 10)
		return append(buf, '}'), nil
	case *big.Int:
		if v == nil {
			return append(buf, `{"int":0}`...), nil
		}
		buf = append(buf, `{"int":`...)
		buf = v.Append(buf, 10)
		return append(buf, '}'), nil
	case Rat:
		return appendAstJsonList(buf, []any{v.Num(), v.Denom()})
	case int:
		return appendAstJsonInt(buf, int64(v)), nil
	case int64:
		return appendAstJsonInt(buf, v), nil
	case uint:
		return appendAstJsonUint(buf, uint64(v)), nil
	case uint64:
		return appendAstJsonUint(buf, v), nil
	case bool:
		buf = append(buf, `{"bool":`...)
		buf = strconv.AppendBool(buf, v)
		return append(buf, '}'), nil
	case string:
		// json.Marshal supplies the string escaping, including the
		// HTML-safe escapes, that the rendered output has always carried.
		quoted, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		buf = append(buf, `{"string":`...)
		buf = append(buf, quoted...)
		return append(buf, '}'), nil
	default:
		return nil, fmt.Errorf(
			"unknown data type (%T) for value: %#v",
			obj,
			obj,
		)
	}
}

func appendAstJsonBytes(buf []byte, v []byte) []byte {
	buf = append(buf, `{"bytes":"`...)
	buf = hex.AppendEncode(buf, v)
	return append(buf, `"}`...)
}

func appendAstJsonInt(buf []byte, v int64) []byte {
	buf = append(buf, `{"int":`...)
	buf = strconv.AppendInt(buf, v, 10)
	return append(buf, '}')
}

func appendAstJsonUint(buf []byte, v uint64) []byte {
	buf = append(buf, `{"int":`...)
	buf = strconv.AppendUint(buf, v, 10)
	return append(buf, '}')
}

func appendAstJsonList[T []any | Set](buf []byte, v T) ([]byte, error) {
	buf = append(buf, `{"list":[`...)
	for idx, val := range v {
		if idx > 0 {
			buf = append(buf, ',')
		}
		var err error
		if buf, err = appendAstJson(buf, val); err != nil {
			return nil, err
		}
	}
	return append(buf, `]}`...), nil
}

type astJsonMapEntry struct {
	key, val   any
	keyStart   int
	keyEnd     int
	valueStart int
	valueEnd   int
}

func appendAstJsonMap[T map[any]any | Map](buf []byte, v T) ([]byte, error) {
	// Entries are ordered by their rendered key alone. Keys are complete
	// JSON objects and none is a strict prefix of another, so for distinct
	// keys this is the same order as sorting whole `{"k":K,"v":V}` entries.
	// Only keys are rendered into scratch (and copied once); values are
	// rendered straight into buf in final order and never moved. Keys are
	// bounded when decoded, so the key copy does not grow with nesting.
	entries := make([]astJsonMapEntry, 0, len(v))
	var keyScratch []byte
	for key, val := range v {
		start := len(keyScratch)
		var err error
		if keyScratch, err = appendAstJson(keyScratch, key); err != nil {
			return nil, err
		}
		entries = append(
			entries,
			astJsonMapEntry{
				key:      key,
				val:      val,
				keyStart: start,
				keyEnd:   len(keyScratch),
			},
		)
	}
	keyBytes := func(e astJsonMapEntry) []byte {
		return keyScratch[e.keyStart:e.keyEnd]
	}
	slices.SortFunc(entries, func(a, b astJsonMapEntry) int {
		return bytes.Compare(keyBytes(a), keyBytes(b))
	})
	buf = append(buf, `{"map":[`...)
	for i := 0; i < len(entries); {
		// Distinct Go keys can render identically (int 1 and uint64 1).
		// Value decoding rejects such duplicate keys, so a tie group only
		// arises from a hand-built Map; its values are rendered aside and
		// ordered by their bytes, as the whole-entry order would.
		j := i + 1
		for j < len(entries) &&
			bytes.Equal(keyBytes(entries[i]), keyBytes(entries[j])) {
			j++
		}
		var err error
		if j-i == 1 {
			buf = append(buf, `{"k":`...)
			buf = append(buf, keyBytes(entries[i])...)
			buf = append(buf, `,"v":`...)
			if buf, err = appendAstJson(buf, entries[i].val); err != nil {
				return nil, err
			}
			buf = append(buf, `},`...)
			i = j
			continue
		}
		var valScratch []byte
		for k := i; k < j; k++ {
			entries[k].valueStart = len(valScratch)
			valScratch, err = appendAstJson(valScratch, entries[k].val)
			if err != nil {
				return nil, err
			}
			entries[k].valueEnd = len(valScratch)
		}
		group := entries[i:j]
		slices.SortFunc(group, func(a, b astJsonMapEntry) int {
			return bytes.Compare(
				valScratch[a.valueStart:a.valueEnd],
				valScratch[b.valueStart:b.valueEnd],
			)
		})
		for _, e := range group {
			buf = append(buf, `{"k":`...)
			buf = append(buf, keyBytes(e)...)
			buf = append(buf, `,"v":`...)
			buf = append(buf, valScratch[e.valueStart:e.valueEnd]...)
			buf = append(buf, `},`...)
		}
		i = j
	}
	if len(entries) > 0 {
		// Drop the comma after the last entry.
		buf = buf[:len(buf)-1]
	}
	return append(buf, `]}`...), nil
}

type LazyValue struct {
	value *Value
}

func (l *LazyValue) MarshalCBOR() ([]byte, error) {
	if l.value == nil {
		l.value = &Value{}
	}
	// Return stored CBOR
	// This is only a stopgap, since it doesn't allow us to build values from scratch
	return []byte(l.value.cborData), nil
}

func (l *LazyValue) UnmarshalCBOR(data []byte) error {
	if l.value == nil {
		l.value = &Value{}
	}
	l.value.cborData = string(data[:])
	return nil
}

func (l *LazyValue) MarshalJSON() ([]byte, error) {
	if l.value == nil {
		l.value = &Value{}
	}
	if l.Value() == nil && len(l.value.cborData) > 0 {
		// Try to decode if we can, but don't blow up if we can't
		if _, err := l.Decode(); err != nil {
			tmpJsonObj := map[string]any{
				"cbor":  hex.EncodeToString([]byte(l.value.cborData)),
				"json":  nil,
				"error": err.Error(),
			}
			return json.Marshal(tmpJsonObj)
		}
	}
	return l.value.MarshalJSON()
}

func (l *LazyValue) Decode() (any, error) {
	if l.value == nil {
		l.value = &Value{}
	}
	err := l.value.UnmarshalCBOR([]byte(l.value.cborData))
	return l.Value(), err
}

func (l *LazyValue) Value() any {
	if l.value == nil {
		return nil
	}
	return l.value.Value()
}

func (l *LazyValue) Cbor() []byte {
	if l.value == nil {
		return nil
	}
	return l.value.Cbor()
}
