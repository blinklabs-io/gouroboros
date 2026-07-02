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

package cbor_test

import (
	"encoding/hex"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeGenericTypeCacheConcurrent(t *testing.T) {
	src := &encodeGenericTestStruct{Foo: 5, Bar: "ba"}
	expected, err := cbor.EncodeGeneric(src)
	require.NoError(t, err)

	const goroutines = 32
	const iterations = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	results := make(chan []byte, goroutines*iterations)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				result, err := cbor.EncodeGeneric(src)
				if err != nil {
					errs <- err
					return
				}
				results <- result
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		require.NoError(t, err)
	}
	for result := range results {
		assert.Equal(t, expected, result)
	}
}

func TestDecodeGenericTypeCacheConcurrent(t *testing.T) {
	cborData, err := hex.DecodeString("a26342617262626163466f6f05")
	require.NoError(t, err)
	expected := &decodeGenericTestStruct{Foo: 5, Bar: "ba"}

	const goroutines = 32
	const iterations = 100

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	results := make(chan *decodeGenericTestStruct, goroutines*iterations)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range iterations {
				dest := &decodeGenericTestStruct{}
				if err := cbor.DecodeGeneric(cborData, dest); err != nil {
					errs <- err
					return
				}
				results <- dest
			}
		}()
	}
	wg.Wait()
	close(errs)
	close(results)

	for err := range errs {
		require.NoError(t, err)
	}
	for result := range results {
		assert.Equal(t, expected, result)
	}
}

func BenchmarkEncodeGenericCached(b *testing.B) {
	src := &encodeGenericTestStruct{Foo: 5, Bar: "ba"}
	if _, err := cbor.EncodeGeneric(src); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := cbor.EncodeGeneric(src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeGenericCachedParallel(b *testing.B) {
	src := &encodeGenericTestStruct{Foo: 5, Bar: "ba"}
	if _, err := cbor.EncodeGeneric(src); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := cbor.EncodeGeneric(src); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDecodeGenericCached(b *testing.B) {
	cborData, err := hex.DecodeString("a26342617262626163466f6f05")
	if err != nil {
		b.Fatal(err)
	}
	if err := cbor.DecodeGeneric(cborData, &decodeGenericTestStruct{}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := cbor.DecodeGeneric(cborData, &decodeGenericTestStruct{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecodeGenericCachedParallel(b *testing.B) {
	cborData, err := hex.DecodeString("a26342617262626163466f6f05")
	if err != nil {
		b.Fatal(err)
	}
	if err := cbor.DecodeGeneric(cborData, &decodeGenericTestStruct{}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if err := cbor.DecodeGeneric(cborData, &decodeGenericTestStruct{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}
