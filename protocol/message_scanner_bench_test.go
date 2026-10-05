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

package protocol

import (
	"bytes"
	"testing"
)

// BenchmarkMessageScannerSegmentedDelivery feeds a near-limit message to the
// scanner one small segment at a time, as a peer pacing its segments would.
// Every iteration fails unless the scanner processed each message byte exactly
// once, so a scanner that reparses the accepted prefix cannot pass.
func BenchmarkMessageScannerSegmentedDelivery(b *testing.B) {
	const (
		payloadSize = 1024 * 1024
		segmentSize = 256
	)
	// [1, bytes(payloadSize)]
	message := []byte{0x82, 0x01, 0x5a, 0x00, 0x10, 0x00, 0x00}
	message = append(message, bytes.Repeat([]byte{0x42}, payloadSize)...)

	b.SetBytes(int64(len(message)))
	b.ReportAllocs()
	for b.Loop() {
		scanner := messageScanner{}
		var result messageScanResult
		typeSeen := false
		for end := segmentSize; ; end += segmentSize {
			end = min(end, len(message))
			var err error
			result, err = scanner.scan(message[:end], 0)
			if err != nil {
				b.Fatal(err)
			}
			if result.hasMessageType && !typeSeen {
				// The scanner reports the message type before finishing the
				// segment; the read loop scans again, so do the same.
				typeSeen = true
				result, err = scanner.scan(message[:end], 0)
				if err != nil {
					b.Fatal(err)
				}
			}
			if end == len(message) {
				break
			}
		}
		if !result.complete || result.messageLength != len(message) {
			b.Fatalf("scan incomplete: %+v", result)
		}
		if scanner.processedBytes != len(message) {
			b.Fatalf(
				"processed %d bytes for a %d byte message",
				scanner.processedBytes,
				len(message),
			)
		}
	}
}
