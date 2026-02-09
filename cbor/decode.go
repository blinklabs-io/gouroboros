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
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"

	_cbor "github.com/fxamacker/cbor/v2"
	"github.com/jinzhu/copier"
)

var (
	cachedDecMode     _cbor.DecMode
	cachedDecModeErr  error
	cachedDecModeOnce sync.Once
)

// getDecMode returns a cached DecMode, initializing it on first use.
// Uses sync.Once for thread-safe lazy initialization.
// Returns the cached error if initialization failed.
func getDecMode() (_cbor.DecMode, error) {
	cachedDecModeOnce.Do(func() {
		decOptions := _cbor.DecOptions{
			ExtraReturnErrors: _cbor.ExtraDecErrorUnknownField,
			// This defaults to 32, but there are blocks in the wild using >64 nested levels
			MaxNestedLevels: 256,
		}
		cachedDecMode, cachedDecModeErr = decOptions.DecModeWithTags(customTagSet)
	})
	return cachedDecMode, cachedDecModeErr
}

func Decode(dataBytes []byte, dest any) (int, error) {
	data := bytes.NewReader(dataBytes)
	decMode, err := getDecMode()
	if err != nil {
		return 0, err
	}
	if decMode == nil {
		return 0, errors.New("CBOR decoder mode not initialized")
	}
	dec := decMode.NewDecoder(data)
	err = dec.Decode(dest)
	return dec.NumBytesRead(), err
}

// Extract the first item from a CBOR list. This will return the first item from the
// provided list if it's numeric and an error otherwise
func DecodeIdFromList(cborData []byte) (int, error) {
	// If the list length is <= the max simple uint and the first list value
	// is <= the max simple uint, then we can extract the value straight from
	// the byte slice
	listLen, err := ListLength(cborData)
	if err != nil {
		return 0, err
	}
	if listLen == 0 {
		return 0, errors.New("cannot return first item from empty list")
	}
	if listLen < int(CborMaxUintSimple) {
		if cborData[1] <= CborMaxUintSimple {
			return int(cborData[1]), nil
		}
	}
	// If we couldn't use the shortcut above, actually decode the list
	var tmp Value
	if _, err := Decode(cborData, &tmp); err != nil {
		return 0, err
	}
	// Make sure that the value is actually a slice
	val := tmp.Value()
	list, ok := val.([]any)
	if !ok {
		return 0, fmt.Errorf("decoded value was not a list, found: %T", val)
	}
	if len(list) == 0 {
		return 0, errors.New("cannot return first item from empty list")
	}
	// Make sure that the first item is actually numeric
	switch v := list[0].(type) {
	// The upstream CBOR library uses uint64 by default for numeric values
	case uint64:
		if v > uint64(math.MaxInt) {
			return 0, errors.New("decoded numeric value too large: uint64 > int")
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("first list item was not numeric, found: %v", v)
	}
}

// Determine the length of a CBOR list
func ListLength(cborData []byte) (int, error) {
	// If the list length is <= the max simple uint, then we can extract the length
	// value straight from the byte slice (with a little math)
	if cborData[0] >= CborTypeArray &&
		cborData[0] <= (CborTypeArray+CborMaxUintSimple) {
		return int(cborData[0]) - int(CborTypeArray), nil
	}
	// If we couldn't use the shortcut above, actually decode the list
	var tmp []RawMessage
	if _, err := Decode(cborData, &tmp); err != nil {
		return 0, err
	}
	return len(tmp), nil
}

// Decode CBOR list data by the leading value of each list item. It expects CBOR data and
// a map of numbers to object pointers to decode into
func DecodeById(
	cborData []byte,
	idMap map[int]any,
) (any, error) {
	id, err := DecodeIdFromList(cborData)
	if err != nil {
		return nil, err
	}
	ret, ok := idMap[id]
	if !ok || ret == nil {
		return nil, fmt.Errorf("found unknown ID: %x", id)
	}
	if _, err := Decode(cborData, ret); err != nil {
		return nil, err
	}
	return ret, nil
}

var (
	decodeGenericTypeCache      = map[reflect.Type]reflect.Type{}
	decodeGenericTypeCacheMutex sync.RWMutex
)

// DecodeGeneric decodes the specified CBOR into the destination object without using the
// destination object's UnmarshalCBOR() function
func DecodeGeneric(cborData []byte, dest any) error {
	// Get destination type
	valueDest := reflect.ValueOf(dest)
	typeDest := valueDest.Elem().Type()
	// Check type cache
	decodeGenericTypeCacheMutex.RLock()
	tmpTypeDest, ok := decodeGenericTypeCache[typeDest]
	decodeGenericTypeCacheMutex.RUnlock()
	if !ok {
		// Create a duplicate(-ish) struct from the destination
		// We do this so that we can bypass any custom UnmarshalCBOR() function on the
		// destination object
		if valueDest.Kind() != reflect.Pointer ||
			valueDest.Elem().Kind() != reflect.Struct {
			return errors.New("destination must be a pointer to a struct")
		}
		destTypeFields := []reflect.StructField{}
		for i := range typeDest.NumField() {
			tmpField := typeDest.Field(i)
			if tmpField.IsExported() && tmpField.Name != "DecodeStoreCbor" {
				destTypeFields = append(destTypeFields, tmpField)
			}
		}
		tmpTypeDest = reflect.StructOf(destTypeFields)
		// Populate cache
		decodeGenericTypeCacheMutex.Lock()
		decodeGenericTypeCache[typeDest] = tmpTypeDest
		decodeGenericTypeCacheMutex.Unlock()
	}
	// Create temporary object with the type created above
	tmpDest := reflect.New(tmpTypeDest)
	// Decode CBOR into temporary object
	if _, err := Decode(cborData, tmpDest.Interface()); err != nil {
		return err
	}
	// Copy values from temporary object into destination object
	if err := copier.Copy(dest, tmpDest.Interface()); err != nil {
		return err
	}
	return nil
}

// StreamDecoder provides sequential CBOR decoding with position tracking.
// It wraps the underlying decoder to track byte offsets of each decoded item.
type StreamDecoder struct {
	dec      *_cbor.Decoder
	decMode  _cbor.DecMode // cached decode mode for reuse in Advance()
	data     []byte
	consumed int // bytes consumed by Advance() calls
}

// NewStreamDecoder creates a decoder for sequential CBOR item extraction with position tracking.
func NewStreamDecoder(data []byte) (*StreamDecoder, error) {
	decMode, err := getDecMode()
	if err != nil {
		return nil, err
	}
	if decMode == nil {
		return nil, errors.New("CBOR decoder mode not initialized")
	}
	return &StreamDecoder{
		dec:     decMode.NewDecoder(bytes.NewReader(data)),
		decMode: decMode,
		data:    data,
	}, nil
}

// Position returns the current byte position in the stream.
func (d *StreamDecoder) Position() int {
	return d.consumed + d.dec.NumBytesRead()
}

// Decode decodes the next CBOR item into dest and returns its byte range.
// Returns (startOffset, length, error).
func (d *StreamDecoder) Decode(dest any) (int, int, error) {
	start := d.consumed + d.dec.NumBytesRead()
	if err := d.dec.Decode(dest); err != nil {
		return 0, 0, err
	}
	end := d.consumed + d.dec.NumBytesRead()
	return start, end - start, nil
}

// Skip skips the next CBOR item and returns its byte range.
// Returns (startOffset, length, error).
func (d *StreamDecoder) Skip() (int, int, error) {
	start := d.consumed + d.dec.NumBytesRead()
	if err := d.dec.Skip(); err != nil {
		return 0, 0, err
	}
	end := d.consumed + d.dec.NumBytesRead()
	return start, end - start, nil
}

// DecodeRaw decodes the next CBOR item and returns both its value and raw bytes.
// Returns (startOffset, rawBytes, error).
func (d *StreamDecoder) DecodeRaw(dest any) (int, []byte, error) {
	absStart := d.consumed + d.dec.NumBytesRead()
	relStart := d.dec.NumBytesRead()
	if err := d.dec.Decode(dest); err != nil {
		return 0, nil, err
	}
	relEnd := d.dec.NumBytesRead()
	return absStart, d.data[d.consumed+relStart : d.consumed+relEnd], nil
}

// RawBytes returns the raw bytes for the given offset and length.
func (d *StreamDecoder) RawBytes(offset, length int) []byte {
	// Check for negative values and integer overflow
	if offset < 0 || length < 0 {
		return nil
	}
	end := offset + length
	// Check for integer overflow: if end < offset, overflow occurred
	if end < offset || end > len(d.data) {
		return nil
	}
	return d.data[offset:end]
}

// Data returns the underlying byte slice.
func (d *StreamDecoder) Data() []byte {
	return d.data
}

// EOF returns true if the decoder has reached the end of the data.
func (d *StreamDecoder) EOF() bool {
	return d.consumed+d.dec.NumBytesRead() >= len(d.data)
}

// Advance moves the decoder position forward by n bytes without decoding.
// This is useful for skipping past headers that were parsed manually.
// Returns an error if n would advance past the end of data.
func (d *StreamDecoder) Advance(n int) error {
	if n < 0 {
		return errors.New("cannot advance by negative amount")
	}
	newPos := d.consumed + d.dec.NumBytesRead() + n
	if newPos > len(d.data) {
		return errors.New("advance would exceed data bounds")
	}
	d.consumed = newPos
	// Reinitialize decoder with remaining data, reusing cached DecMode
	d.dec = d.decMode.NewDecoder(bytes.NewReader(d.data[d.consumed:]))
	return nil
}

// DecodeArrayHeader decodes a CBOR array header and returns the number of elements.
// This advances the position past the header only, not the array contents.
// Returns (arrayLength, headerOffset, headerLength, error).
func (d *StreamDecoder) DecodeArrayHeader() (int, int, int, error) {
	relStart := d.dec.NumBytesRead()
	absStart := d.consumed + relStart
	if absStart >= len(d.data) {
		return 0, 0, 0, errors.New("unexpected end of data")
	}

	// Parse array header manually to avoid consuming content
	firstByte := d.data[absStart]
	majorType := firstByte & CborTypeMask

	if majorType != CborTypeArray {
		return 0, 0, 0, fmt.Errorf("expected array (0x%x), got 0x%x", CborTypeArray, majorType)
	}

	additionalInfo := firstByte & 0x1f
	var length int
	var headerLen int

	switch {
	case additionalInfo < 24:
		// Length encoded in the first byte
		length = int(additionalInfo)
		headerLen = 1
	case additionalInfo == 24:
		// 1-byte length follows
		if absStart+2 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading array length")
		}
		length = int(d.data[absStart+1])
		headerLen = 2
	case additionalInfo == 25:
		// 2-byte length follows (big-endian)
		if absStart+3 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading array length")
		}
		length = int(d.data[absStart+1])<<8 | int(d.data[absStart+2])
		headerLen = 3
	case additionalInfo == 26:
		// 4-byte length follows (big-endian)
		if absStart+5 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading array length")
		}
		// Read as uint32 first to avoid sign issues on 32-bit systems
		len32 := uint32(d.data[absStart+1])<<24 | uint32(d.data[absStart+2])<<16 |
			uint32(d.data[absStart+3])<<8 | uint32(d.data[absStart+4])
		if len32 > uint32(math.MaxInt32) {
			return 0, 0, 0, errors.New("array length exceeds maximum int32 value")
		}
		length = int(len32)
		headerLen = 5
	case additionalInfo == 27:
		// 8-byte length follows (big-endian)
		if absStart+9 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading array length")
		}
		// Read as uint64 first, then check if it fits in int32
		// Use MaxInt32 for consistency with ArrayInfo and to prevent overflow
		// when values are later converted to uint32 for offset calculations
		len64 := uint64(d.data[absStart+1])<<56 | uint64(d.data[absStart+2])<<48 |
			uint64(d.data[absStart+3])<<40 | uint64(d.data[absStart+4])<<32 |
			uint64(d.data[absStart+5])<<24 | uint64(d.data[absStart+6])<<16 |
			uint64(d.data[absStart+7])<<8 | uint64(d.data[absStart+8])
		if len64 > uint64(math.MaxInt32) {
			return 0, 0, 0, errors.New("array length exceeds maximum int32 value")
		}
		length = int(len64)
		headerLen = 9
	case additionalInfo == 31:
		// Indefinite length array - need to count elements
		return 0, 0, 0, errors.New("indefinite length arrays not supported in header-only decode")
	default:
		return 0, 0, 0, fmt.Errorf("invalid array additional info: %d", additionalInfo)
	}

	// Advance the decoder position past the header
	if err := d.Advance(headerLen); err != nil {
		return 0, 0, 0, err
	}

	return length, absStart, headerLen, nil
}

// DecodeMapHeader decodes a CBOR map header and returns the number of key-value pairs.
// This advances the position past the header only, not the map contents.
// Returns (mapLength, headerOffset, headerLength, error).
func (d *StreamDecoder) DecodeMapHeader() (int, int, int, error) {
	relStart := d.dec.NumBytesRead()
	absStart := d.consumed + relStart
	if absStart >= len(d.data) {
		return 0, 0, 0, errors.New("unexpected end of data")
	}

	firstByte := d.data[absStart]
	majorType := firstByte & CborTypeMask

	if majorType != CborTypeMap {
		return 0, 0, 0, fmt.Errorf("expected map (0x%x), got 0x%x", CborTypeMap, majorType)
	}

	additionalInfo := firstByte & 0x1f
	var length int
	var headerLen int

	switch {
	case additionalInfo < 24:
		length = int(additionalInfo)
		headerLen = 1
	case additionalInfo == 24:
		if absStart+2 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading map length")
		}
		length = int(d.data[absStart+1])
		headerLen = 2
	case additionalInfo == 25:
		if absStart+3 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading map length")
		}
		length = int(d.data[absStart+1])<<8 | int(d.data[absStart+2])
		headerLen = 3
	case additionalInfo == 26:
		// 4-byte length follows (big-endian)
		if absStart+5 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading map length")
		}
		// Read as uint32 first to avoid sign issues on 32-bit systems
		len32 := uint32(d.data[absStart+1])<<24 | uint32(d.data[absStart+2])<<16 |
			uint32(d.data[absStart+3])<<8 | uint32(d.data[absStart+4])
		if len32 > uint32(math.MaxInt32) {
			return 0, 0, 0, errors.New("map length exceeds maximum int32 value")
		}
		length = int(len32)
		headerLen = 5
	case additionalInfo == 27:
		// 8-byte length follows (big-endian)
		if absStart+9 > len(d.data) {
			return 0, 0, 0, errors.New("unexpected end of data reading map length")
		}
		// Read as uint64 first, then check if it fits in int32
		// Use MaxInt32 for consistency with MapInfo and to prevent overflow
		// when values are later converted to uint32 for offset calculations
		len64 := uint64(d.data[absStart+1])<<56 | uint64(d.data[absStart+2])<<48 |
			uint64(d.data[absStart+3])<<40 | uint64(d.data[absStart+4])<<32 |
			uint64(d.data[absStart+5])<<24 | uint64(d.data[absStart+6])<<16 |
			uint64(d.data[absStart+7])<<8 | uint64(d.data[absStart+8])
		if len64 > uint64(math.MaxInt32) {
			return 0, 0, 0, errors.New("map length exceeds maximum int32 value")
		}
		length = int(len64)
		headerLen = 9
	case additionalInfo == 31:
		return 0, 0, 0, errors.New("indefinite length maps not supported in header-only decode")
	default:
		return 0, 0, 0, fmt.Errorf("invalid map additional info: %d", additionalInfo)
	}

	// Advance the decoder position past the header
	if err := d.Advance(headerLen); err != nil {
		return 0, 0, 0, err
	}

	return length, absStart, headerLen, nil
}

// SkipN skips n CBOR items and returns the total byte range skipped.
// Returns (startOffset, totalLength, error).
func (d *StreamDecoder) SkipN(n int) (int, int, error) {
	if n <= 0 {
		pos := d.consumed + d.dec.NumBytesRead()
		return pos, 0, nil
	}

	start := d.consumed + d.dec.NumBytesRead()
	for i := 0; i < n; i++ {
		if err := d.dec.Skip(); err != nil {
			return 0, 0, fmt.Errorf("skip item %d: %w", i, err)
		}
	}
	end := d.consumed + d.dec.NumBytesRead()
	return start, end - start, nil
}

// DecodeArrayItems decodes all items in an array, calling the callback for each.
// The callback receives the item index, start offset, and length.
// Returns the total array byte range (including header).
func (d *StreamDecoder) DecodeArrayItems(
	callback func(index int, offset int, length int, data []byte) error,
) (int, int, error) {
	arrayStart := d.consumed + d.dec.NumBytesRead()

	// Decode as array of RawMessage to get each item's bytes
	var items []RawMessage
	if _, _, err := d.Decode(&items); err != nil {
		return 0, 0, fmt.Errorf("decode array: %w", err)
	}

	arrayEnd := d.consumed + d.dec.NumBytesRead()

	// Calculate actual header size from the raw bytes to handle non-canonical encodings
	headerLen, err := cborArrayHeaderSizeFromBytes(d.data, arrayStart)
	if err != nil {
		return 0, 0, fmt.Errorf("parse array header: %w", err)
	}

	// Calculate positions for each item
	// The array header is followed by consecutive items
	pos := arrayStart + headerLen
	for i, item := range items {
		itemLen := len(item)
		if callback != nil {
			if err := callback(i, pos, itemLen, []byte(item)); err != nil {
				return 0, 0, err
			}
		}
		pos += itemLen
	}

	return arrayStart, arrayEnd - arrayStart, nil
}

// cborArrayHeaderSizeFromBytes returns the actual size of a CBOR array header by
// reading the bytes at the given offset. This handles non-canonical encodings correctly.
// Returns an error for indefinite-length arrays.
func cborArrayHeaderSizeFromBytes(data []byte, offset int) (int, error) {
	if offset >= len(data) {
		return 0, errors.New("unexpected end of data reading array header")
	}

	firstByte := data[offset]
	majorType := firstByte & CborTypeMask

	if majorType != CborTypeArray {
		return 0, fmt.Errorf("expected array (0x%x), got 0x%x", CborTypeArray, majorType)
	}

	additionalInfo := firstByte & 0x1f

	switch {
	case additionalInfo < 24:
		return 1, nil
	case additionalInfo == 24:
		return 2, nil
	case additionalInfo == 25:
		return 3, nil
	case additionalInfo == 26:
		return 5, nil
	case additionalInfo == 27:
		return 9, nil
	case additionalInfo == 31:
		return 0, errors.New("indefinite length arrays not supported")
	default:
		return 0, fmt.Errorf("invalid array additional info: %d", additionalInfo)
	}
}

// ArrayInfo extracts array item count and header size from CBOR array data.
// Returns (count, headerSize, isIndefinite). Count is -1 for invalid headers.
func ArrayInfo(data []byte) (int, uint32, bool) {
	if len(data) == 0 {
		return -1, 0, false
	}
	firstByte := data[0]

	// Check it's an array (major type 4)
	if firstByte&0xe0 != CborTypeArray {
		return -1, 0, false
	}

	additional := firstByte & 0x1f
	switch {
	case additional <= 23:
		return int(additional), 1, false
	case additional == 24 && len(data) >= 2:
		return int(data[1]), 2, false
	case additional == 25 && len(data) >= 3:
		return int(uint16(data[1])<<8 | uint16(data[2])), 3, false
	case additional == 26 && len(data) >= 5:
		// 4-byte length - check for overflow before converting to int
		len32 := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
		if len32 > uint32(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len32), 5, false
	case additional == 27 && len(data) >= 9:
		// 8-byte length (unlikely for typical data but handle for completeness)
		len64 := uint64(data[1])<<56 | uint64(data[2])<<48 |
			uint64(data[3])<<40 | uint64(data[4])<<32 |
			uint64(data[5])<<24 | uint64(data[6])<<16 |
			uint64(data[7])<<8 | uint64(data[8])
		if len64 > uint64(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len64), 9, false
	case additional == 31:
		return 0, 1, true // Indefinite length
	default:
		return -1, 0, false
	}
}

// MapInfo extracts map item count and header size from CBOR map data.
// Returns (count, headerSize, isIndefinite). Count is -1 for invalid headers.
func MapInfo(data []byte) (int, uint32, bool) {
	if len(data) == 0 {
		return -1, 0, false
	}
	firstByte := data[0]

	// Check it's a map (major type 5)
	if firstByte&0xe0 != CborTypeMap {
		return -1, 0, false
	}

	additional := firstByte & 0x1f
	switch {
	case additional <= 23:
		return int(additional), 1, false
	case additional == 24 && len(data) >= 2:
		return int(data[1]), 2, false
	case additional == 25 && len(data) >= 3:
		return int(uint16(data[1])<<8 | uint16(data[2])), 3, false
	case additional == 26 && len(data) >= 5:
		// 4-byte length - check for overflow before converting to int
		len32 := uint32(data[1])<<24 | uint32(data[2])<<16 | uint32(data[3])<<8 | uint32(data[4])
		if len32 > uint32(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len32), 5, false
	case additional == 27 && len(data) >= 9:
		// 8-byte length (unlikely for typical data but handle for completeness)
		len64 := uint64(data[1])<<56 | uint64(data[2])<<48 |
			uint64(data[3])<<40 | uint64(data[4])<<32 |
			uint64(data[5])<<24 | uint64(data[6])<<16 |
			uint64(data[7])<<8 | uint64(data[8])
		if len64 > uint64(math.MaxInt32) {
			return -1, 0, false // Too large to handle
		}
		return int(len64), 9, false
	case additional == 31:
		return 0, 1, true // Indefinite length
	default:
		return -1, 0, false
	}
}

// ArrayHeaderSize returns the CBOR header size in bytes for an array of given length.
func ArrayHeaderSize(length int) uint32 {
	if length < 24 {
		return 1 // 0x80 + length
	} else if length < 256 {
		return 2 // 0x98 + 1-byte length
	} else if length < 65536 {
		return 3 // 0x99 + 2-byte length
	}
	return 5 // 0x9a + 4-byte length (covers up to 2^32 elements)
}
