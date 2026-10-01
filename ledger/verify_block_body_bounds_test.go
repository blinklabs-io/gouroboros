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

package ledger_test

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"runtime"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

// allocatedBytes reports the heap bytes fn allocates. Tests using it must
// not run in parallel, since the counter is process-wide.
func allocatedBytes(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

func definiteArrayHeader(count int) []byte {
	header := make([]byte, 5)
	header[0] = 0x9a
	// #nosec G115 -- test counts are small and non-negative.
	binary.BigEndian.PutUint32(header[1:], uint32(count))
	return header
}

func blockBodyHex(count int, entry []byte) string {
	var body bytes.Buffer
	body.Write(definiteArrayHeader(count))
	for range count {
		body.Write(entry)
	}
	return hex.EncodeToString(body.Bytes())
}

// Each entry is a well-formed one-byte empty array, so the body decodes as
// CBOR while declaring four times more transactions than a body of its size
// can hold, though fewer than MaxVerifyBlockBodyTxs.
func TestVerifyBlockBodyRejectsExcessiveTransactionCount(t *testing.T) {
	const count = 40000
	data := blockBodyHex(count, []byte{0x80})
	var err error
	allocated := allocatedBytes(func() {
		_, err = ledger.VerifyBlockBody(
			data,
			ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
			nil,
		)
	})
	require.Less(t, allocated, uint64(len(data)))
	require.ErrorContains(t, err, "declares")
}

// A four-field entry is as large as a valid one, so the declared count passes
// the size check and only the entry shape is wrong.
func TestVerifyBlockBodyRejectsMalformedTransactionBeforeReserving(
	t *testing.T,
) {
	const count = 40000
	data := blockBodyHex(count, []byte{0x84, 0x60, 0x60, 0x60, 0x60})
	var err error
	allocated := allocatedBytes(func() {
		_, err = ledger.VerifyBlockBody(
			data,
			ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
			nil,
		)
	})
	require.Less(t, allocated, uint64(len(data)))
	require.ErrorContains(t, err, "expected 3 fields")
}

// Minimal entries keep the declared count within what the body's bytes can
// hold, so only the transaction ceiling rejects it. A definite-length body is
// rejected from its header.
func TestVerifyBlockBodyRejectsDeclaredTransactionsOverCeiling(
	t *testing.T,
) {
	data := blockBodyHex(
		ledger.MaxVerifyBlockBodyTxs+1,
		[]byte{0x83, 0x60, 0x60, 0x60},
	)
	var err error
	allocated := allocatedBytes(func() {
		_, err = ledger.VerifyBlockBody(
			data,
			ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
			nil,
		)
	})
	require.Less(t, allocated, uint64(len(data)))
	require.ErrorContains(t, err, "maximum")
}

// An indefinite-length body declares no count, so its entries are decoded up
// to the ceiling, the same work as a valid body of that many transactions.
func TestVerifyBlockBodyRejectsIndefiniteTransactionsOverCeiling(
	t *testing.T,
) {
	data := "9f" + strings.Repeat(
		"83606060",
		ledger.MaxVerifyBlockBodyTxs+1,
	) + "ff"
	_, err := ledger.VerifyBlockBody(
		data,
		ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
		nil,
	)
	require.ErrorContains(t, err, "maximum")
}

// An indefinite-length entry has its items counted before it is decoded.
func TestVerifyBlockBodyRejectsIndefiniteEntryWithExtraFields(t *testing.T) {
	entry := "9f" + strings.Repeat("60", 4) + "ff"
	_, err := ledger.VerifyBlockBody(
		"81"+entry,
		ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
		nil,
	)
	require.ErrorContains(t, err, "expected 3 fields, got 4")
}

func TestCalculateBlockBodyHashRejectsMalformedTransactionBeforeReserving(
	t *testing.T,
) {
	const count = 1 << 18
	txs := make([][]string, count)
	for i := range txs {
		txs[i] = []string{"", "", "", ""}
	}
	var err error
	allocated := allocatedBytes(func() {
		_, err = ledger.CalculateBlockBodyHash(txs, nil)
	})
	require.Less(t, allocated, uint64(64*1024))
	require.ErrorContains(t, err, "tx len error")
}

func TestVerifyBlockBodyRejectsOversizedInput(t *testing.T) {
	data := strings.Repeat("00", ledger.MaxVerifyBlockBodyBytes+1)
	var err error
	allocated := allocatedBytes(func() {
		_, err = ledger.VerifyBlockBody(
			data,
			ledger.BLOCK_BODY_HASH_ZERO_TX_HEX,
			nil,
		)
	})
	require.Less(t, allocated, uint64(64*1024))
	require.ErrorContains(t, err, "exceeds maximum")
}

func verifyBlockBodyOf(t *testing.T, data string, txs [][]string) {
	t.Helper()
	serialized, err := ledger.CalculateBlockBodyHash(txs, nil)
	require.NoError(t, err)
	hash := blake2b.Sum256(serialized)
	ok, err := ledger.VerifyBlockBody(data, hex.EncodeToString(hash[:]), nil)
	require.NoError(t, err)
	require.True(t, ok)
}

// The largest body the input limit admits: one transaction whose body field,
// the hex of one CBOR byte string, fills MaxVerifyBlockBodyBytes after the
// CBOR headers around it.
func TestVerifyBlockBodyAcceptsBodyAtSizeLimit(t *testing.T) {
	// A two-byte outer array header (so the total can be even), the entry
	// array header, the body field's 5-byte text header, and two empty
	// fields.
	const overhead = 2 + 1 + 5 + 1 + 1
	// The byte string's own header is 5 bytes, encoded as 10 hex digits.
	payloadLen := (ledger.MaxVerifyBlockBodyBytes-overhead)/2 - 5
	field := make([]byte, 5, 5+payloadLen)
	field[0] = 0x5a
	// #nosec G115 -- bounded by MaxVerifyBlockBodyBytes.
	binary.BigEndian.PutUint32(field[1:], uint32(payloadLen))
	field = append(field, make([]byte, payloadLen)...)
	bodyHex := hex.EncodeToString(field)
	var body bytes.Buffer
	body.Write([]byte{0x98, 0x01, 0x83, 0x7a})
	lengthBytes := make([]byte, 4)
	// #nosec G115 -- bounded by MaxVerifyBlockBodyBytes.
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(bodyHex)))
	body.Write(lengthBytes)
	body.WriteString(bodyHex)
	body.Write([]byte{0x60, 0x60})
	require.Equal(t, ledger.MaxVerifyBlockBodyBytes, body.Len())
	verifyBlockBodyOf(
		t,
		hex.EncodeToString(body.Bytes()),
		[][]string{{bodyHex, "", ""}},
	)
}

// Minimal entries fill a definite-length body exactly to both count limits:
// the bytes it carries and MaxVerifyBlockBodyTxs.
func TestVerifyBlockBodyAcceptsTransactionCountAtLimit(t *testing.T) {
	const count = ledger.MaxVerifyBlockBodyTxs
	txs := make([][]string, count)
	for i := range txs {
		txs[i] = []string{"", "", ""}
	}
	verifyBlockBodyOf(
		t,
		blockBodyHex(count, []byte{0x83, 0x60, 0x60, 0x60}),
		txs,
	)
}

func TestVerifyBlockBodyAcceptsIndefiniteLengthBody(t *testing.T) {
	// Each "a0" field is the hex of an empty CBOR map.
	entry := "83" + strings.Repeat("626130", 3)
	verifyBlockBodyOf(
		t,
		"9f"+entry+entry+"ff",
		[][]string{{"a0", "a0", "a0"}, {"a0", "a0", "a0"}},
	)
}
