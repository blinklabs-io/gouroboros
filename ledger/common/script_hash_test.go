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
	"testing"
)

func scriptBytes(size int, seed byte) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = seed + byte(i*7)
	}
	return b
}

func TestScriptHashMatchesBlake2b224OfTaggedBytes(t *testing.T) {
	for _, size := range []int{0, 1, scriptHashMemoMinSize - 1, scriptHashMemoMinSize, 20000} {
		raw := scriptBytes(size, byte(size))
		scripts := []struct {
			tag    byte
			script Script
		}{
			{ScriptRefTypePlutusV1, PlutusV1Script(raw)},
			{ScriptRefTypePlutusV2, PlutusV2Script(raw)},
			{ScriptRefTypePlutusV3, PlutusV3Script(raw)},
			{ScriptRefTypePlutusV4, PlutusV4Script(raw)},
		}
		seen := map[ScriptHash]byte{}
		for _, s := range scripts {
			want := ScriptHash(Blake2b224Hash(append([]byte{s.tag}, raw...)))
			// The second call reads the memo for scripts large enough to be
			// memoized.
			for range 2 {
				if got := s.script.Hash(); got != want {
					t.Fatalf("size %d tag %d: hash %x, want %x", size, s.tag, got, want)
				}
			}
			if other, ok := seen[want]; ok {
				t.Fatalf("size %d: tags %d and %d share a hash", size, other, s.tag)
			}
			seen[want] = s.tag
		}
	}
}

func TestScriptHashMemoIgnoresLaterChangesToTheBytes(t *testing.T) {
	raw := scriptBytes(4096, 3)
	original := bytes.Clone(raw)
	want := PlutusV3Script(original).Hash()
	if got := PlutusV3Script(raw).Hash(); got != want {
		t.Fatalf("hash %x, want %x", got, want)
	}
	// A caller reusing its buffer must not change what the memo holds.
	raw[0] ^= 0xff
	if got := PlutusV3Script(original).Hash(); got != want {
		t.Fatalf("hash after the caller changed its buffer %x, want %x", got, want)
	}
	if got := PlutusV3Script(raw).Hash(); got == want {
		t.Fatal("changed bytes must hash differently")
	}
}

func TestScriptHashMemoStaysBounded(t *testing.T) {
	size := 64 << 10
	for i := range 2*scriptHashMemoMaxBytes/size + 1 {
		raw := scriptBytes(size, 0)
		raw[0], raw[1], raw[2] = byte(i), byte(i>>8), 0xa5
		PlutusV3Script(raw).Hash()
		scriptHashMemo.RLock()
		held := scriptHashMemo.bytes
		scriptHashMemo.RUnlock()
		if held > scriptHashMemoMaxBytes {
			t.Fatalf("memo holds %d bytes, over its %d limit", held, scriptHashMemoMaxBytes)
		}
	}
}

func BenchmarkPlutusV3ScriptHash(b *testing.B) {
	raw := scriptBytes(12000, 9)
	b.Run("memoized", func(b *testing.B) {
		s := PlutusV3Script(raw)
		b.ReportAllocs()
		for b.Loop() {
			_ = s.Hash()
		}
	})
	b.Run("computed", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_ = computeScriptHash(ScriptRefTypePlutusV3, raw)
		}
	})
}
