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
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/blinklabs-io/gouroboros/cbor"
)

func (m MetaInt) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	if m.Value == nil {
		return nil, errors.New("metadata int has nil value")
	}
	return cbor.Encode(m.Value)
}

func (m MetaBytes) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode(m.Value)
}

func (m MetaText) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode(m.Value)
}

func (m MetaList) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	return cbor.Encode(m.Items)
}

func (m MetaMap) MarshalCBOR() ([]byte, error) {
	if raw := m.Cbor(); len(raw) > 0 {
		return raw, nil
	}
	pairs, err := encodeMetaMapPairs(m.Pairs)
	if err != nil {
		return nil, err
	}
	ret := appendCBORMapHeader(nil, uint64(len(pairs)))
	for _, pair := range pairs {
		ret = append(ret, pair.key...)
		ret = append(ret, pair.value...)
	}
	return ret, nil
}

type encodedMetaPair struct {
	key   []byte
	value []byte
}

func metaMapKeyLess(left, right []byte) bool {
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return bytes.Compare(left, right) < 0
}

func encodeMetaMapPairs(pairs []MetaPair) ([]encodedMetaPair, error) {
	ret := make([]encodedMetaPair, 0, len(pairs))
	seen := make(map[string]struct{}, len(pairs))
	for idx, pair := range pairs {
		keyCbor, err := cbor.Encode(pair.Key)
		if err != nil {
			return nil, fmt.Errorf("encode metadata map key %d: %w", idx, err)
		}
		key := string(keyCbor)
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("duplicate metadata map key at index %d", idx)
		}
		seen[key] = struct{}{}

		valueCbor, err := cbor.Encode(pair.Value)
		if err != nil {
			return nil, fmt.Errorf(
				"encode metadata map value %d: %w",
				idx,
				err,
			)
		}
		ret = append(ret, encodedMetaPair{
			key:   keyCbor,
			value: valueCbor,
		})
	}
	sort.Slice(ret, func(i, j int) bool {
		return metaMapKeyLess(ret[i].key, ret[j].key)
	})
	return ret, nil
}

func appendCBORMapHeader(dst []byte, length uint64) []byte {
	const majorMap = 0xa0
	switch {
	case length <= 23:
		return append(dst, majorMap|byte(length))
	case length <= 0xff:
		return append(dst, majorMap|24, byte(length))
	case length <= 0xffff:
		dst = append(dst, majorMap|25, 0, 0)
		binary.BigEndian.PutUint16(dst[len(dst)-2:], uint16(length))
		return dst
	case length <= 0xffffffff:
		dst = append(dst, majorMap|26, 0, 0, 0, 0)
		binary.BigEndian.PutUint32(dst[len(dst)-4:], uint32(length))
		return dst
	default:
		dst = append(dst, majorMap|27, 0, 0, 0, 0, 0, 0, 0, 0)
		binary.BigEndian.PutUint64(dst[len(dst)-8:], length)
		return dst
	}
}
