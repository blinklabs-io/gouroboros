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
	"sync"

	"golang.org/x/crypto/blake2b"
)

// scriptHashMemoMinSize is the smallest script whose hash is memoized.
// Below it, hashing costs about as much as the lookup.
const scriptHashMemoMinSize = 1024

// scriptHashMemoMaxBytes bounds the script bytes the memo holds. Filling it
// empties it, so a stream of distinct scripts cannot grow it further.
const scriptHashMemoMaxBytes = 16 << 20

// scriptHashMemo maps each script language tag and script bytes to the
// script's hash. Validating one transaction hashes every witness and
// reference script once per ledger rule that collects them, a dozen times or
// more, and blake2b runs about twenty times slower than a map lookup on the
// same bytes, so the memo spares all but the first. A hash is a function of
// the bytes it is keyed on, so an entry is never stale.
var scriptHashMemo = struct {
	sync.RWMutex
	hashes map[byte]map[string]ScriptHash
	bytes  int
}{
	hashes: make(map[byte]map[string]ScriptHash),
}

// scriptHash returns blake2b-224 of the language tag followed by raw, the
// hash that identifies a script.
func scriptHash(tag byte, raw []byte) ScriptHash {
	if len(raw) < scriptHashMemoMinSize {
		return computeScriptHash(tag, raw)
	}
	scriptHashMemo.RLock()
	hash, ok := scriptHashMemo.hashes[tag][string(raw)]
	scriptHashMemo.RUnlock()
	if ok {
		return hash
	}
	hash = computeScriptHash(tag, raw)
	scriptHashMemo.Lock()
	defer scriptHashMemo.Unlock()
	if scriptHashMemo.bytes+len(raw) > scriptHashMemoMaxBytes {
		scriptHashMemo.hashes = make(map[byte]map[string]ScriptHash)
		scriptHashMemo.bytes = 0
	}
	byTag := scriptHashMemo.hashes[tag]
	if byTag == nil {
		byTag = make(map[string]ScriptHash)
		scriptHashMemo.hashes[tag] = byTag
	}
	if _, ok := byTag[string(raw)]; !ok {
		byTag[string(raw)] = hash
		scriptHashMemo.bytes += len(raw)
	}
	return hash
}

func computeScriptHash(tag byte, raw []byte) ScriptHash {
	h, err := blake2b.New(Blake2b224Size, nil)
	if err != nil {
		panic(fmt.Sprintf(
			"unexpected error generating empty blake2b hash: %s",
			err,
		))
	}
	h.Write([]byte{tag})
	h.Write(raw)
	var ret ScriptHash
	h.Sum(ret[:0])
	return ret
}
