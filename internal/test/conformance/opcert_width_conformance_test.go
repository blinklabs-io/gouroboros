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

package conformance

import (
	"path"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/allegra"
	"github.com/blinklabs-io/gouroboros/ledger/alonzo"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/dijkstra"
	"github.com/blinklabs-io/gouroboros/ledger/mary"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/ouroboros-mock/fixtures"
)

// goldenHeaderDir is the CardanoNodeToNodeVersion2 golden directory in the
// embedded upstream fixtures. Each Header_* golden is
// [era_id, #6.24(bytes .cbor header)].
const goldenHeaderDir = "upstream/ouroboros-consensus/ouroboros-consensus-cardano/golden/cardano/CardanoNodeToNodeVersion2"

// tPraosOCertOffset is the index of the operational certificate sequence
// number in the flat Shelley-family (TPraos) header body array; the KES period
// is the element after it. Shelley through Alonzo share the shape.
const tPraosOCertOffset = 10

// cPraosOCertIndex is the index of the nested operational certificate array in
// the Babbage-family (CPraos) header body array. Within that array the
// sequence number is element 1 and the KES period element 2.
const cPraosOCertIndex = 8

// readGoldenHeader returns the era id and header CBOR carried by an upstream
// Header_* golden.
func readGoldenHeader(t *testing.T, name string) (uint, []byte) {
	t.Helper()
	raw, err := fixtures.EmbeddedFixtures().
		ReadFile(path.Join(goldenHeaderDir, name))
	if err != nil {
		t.Fatalf("read golden %s: %v", name, err)
	}
	var envelope struct {
		cbor.StructAsArray
		EraId  uint
		Header cbor.Tag
	}
	if _, err := cbor.Decode(raw, &envelope); err != nil {
		t.Fatalf("decode golden envelope %s: %v", name, err)
	}
	headerCbor, ok := envelope.Header.Content.([]byte)
	if !ok {
		t.Fatalf(
			"golden %s header tag content is %T, want []byte",
			name,
			envelope.Header.Content,
		)
	}
	return envelope.EraId, headerCbor
}

// setOCertFields rewrites the operational certificate sequence number and KES
// period in a header, leaving every other byte of the header untouched.
func setOCertFields(
	t *testing.T,
	headerCbor []byte,
	tpraos bool,
	sequenceNumber uint64,
	kesPeriod uint64,
) []byte {
	t.Helper()
	var header []cbor.RawMessage
	if _, err := cbor.Decode(headerCbor, &header); err != nil {
		t.Fatalf("decode header array: %v", err)
	}
	if len(header) != 2 {
		t.Fatalf("header array has %d elements, want 2", len(header))
	}
	var body []cbor.RawMessage
	if _, err := cbor.Decode(header[0], &body); err != nil {
		t.Fatalf("decode header body array: %v", err)
	}
	encode := func(v uint64) cbor.RawMessage {
		encoded, err := cbor.Encode(v)
		if err != nil {
			t.Fatalf("encode %d: %v", v, err)
		}
		return encoded
	}
	if tpraos {
		if len(body) <= tPraosOCertOffset+1 {
			t.Fatalf("TPraos header body has %d elements", len(body))
		}
		body[tPraosOCertOffset] = encode(sequenceNumber)
		body[tPraosOCertOffset+1] = encode(kesPeriod)
	} else {
		if len(body) <= cPraosOCertIndex {
			t.Fatalf("CPraos header body has %d elements", len(body))
		}
		var ocert []cbor.RawMessage
		if _, err := cbor.Decode(body[cPraosOCertIndex], &ocert); err != nil {
			t.Fatalf("decode operational certificate array: %v", err)
		}
		if len(ocert) != 4 {
			t.Fatalf(
				"operational certificate array has %d elements, want 4",
				len(ocert),
			)
		}
		ocert[1] = encode(sequenceNumber)
		ocert[2] = encode(kesPeriod)
		encodedOCert, err := cbor.Encode(ocert)
		if err != nil {
			t.Fatalf("encode operational certificate array: %v", err)
		}
		body[cPraosOCertIndex] = encodedOCert
	}
	encodedBody, err := cbor.Encode(body)
	if err != nil {
		t.Fatalf("encode header body array: %v", err)
	}
	header[0] = encodedBody
	encodedHeader, err := cbor.Encode(header)
	if err != nil {
		t.Fatalf("encode header array: %v", err)
	}
	return encodedHeader
}

// headerOCertFields returns the decoded operational certificate sequence
// number and KES period for any Shelley-family header.
func headerOCertFields(
	t *testing.T,
	header common.BlockHeader,
) (uint64, uint64) {
	t.Helper()
	switch h := header.(type) {
	case *shelley.ShelleyBlockHeader:
		return h.Body.OpCertSequenceNumber, h.Body.OpCertKesPeriod
	case *allegra.AllegraBlockHeader:
		return h.Body.OpCertSequenceNumber, h.Body.OpCertKesPeriod
	case *mary.MaryBlockHeader:
		return h.Body.OpCertSequenceNumber, h.Body.OpCertKesPeriod
	case *alonzo.AlonzoBlockHeader:
		return h.Body.OpCertSequenceNumber, h.Body.OpCertKesPeriod
	case *dijkstra.DijkstraBlockHeader:
		return h.Body.OpCert.SequenceNumber, h.Body.OpCert.KesPeriod
	case *conway.ConwayBlockHeader:
		return h.Body.OpCert.SequenceNumber, h.Body.OpCert.KesPeriod
	case *babbage.BabbageBlockHeader:
		return h.Body.OpCert.SequenceNumber, h.Body.OpCert.KesPeriod
	default:
		t.Fatalf("unexpected header type %T", header)
		return 0, 0
	}
}

// TestGoldenHeaderOCertCounterAndKesPeriodWidth pins the operational
// certificate sequence number and KES period at their reference widths.
//
// cardano-ledger decodes the counter as Word64 and the period as a Word
// newtype with plain decCBOR and no bound (decodeOCertFields,
// libs/cardano-protocol/src/Cardano/Protocol/TPraos/OCert.hs), and the CDDL
// declares sequence_number = uint .size 8 and kes_period = uint .size 8. A
// header carrying a counter or period at or above 2^32 therefore decodes; the
// counter is bounded against ledger state by the OCERT rule and the period by
// the KES expiry check, not by the wire type.
func TestGoldenHeaderOCertCounterAndKesPeriodWidth(t *testing.T) {
	const (
		wideSequenceNumber = uint64(1) << 32
		wideKesPeriod      = uint64(1)<<32 + 7
	)
	for _, testDef := range []struct {
		name   string
		golden string
		tpraos bool
	}{
		{name: "Shelley", golden: "Header_Shelley", tpraos: true},
		{name: "Allegra", golden: "Header_Allegra", tpraos: true},
		{name: "Mary", golden: "Header_Mary", tpraos: true},
		{name: "Alonzo", golden: "Header_Alonzo", tpraos: true},
		{name: "Babbage", golden: "Header_Babbage"},
		{name: "Conway", golden: "Header_Conway"},
		{name: "Dijkstra", golden: "Header_Dijkstra"},
	} {
		t.Run(testDef.name, func(t *testing.T) {
			eraId, headerCbor := readGoldenHeader(t, testDef.golden)
			blockType, ok := ledger.BlockHeaderToBlockTypeMap[eraId]
			if !ok {
				t.Fatalf("no block type for era id %d", eraId)
			}
			if _, err := ledger.NewBlockHeaderFromCbor(blockType, headerCbor); err != nil {
				t.Fatalf("unmodified golden failed to decode: %v", err)
			}
			modified := setOCertFields(
				t,
				headerCbor,
				testDef.tpraos,
				wideSequenceNumber,
				wideKesPeriod,
			)
			header, err := ledger.NewBlockHeaderFromCbor(blockType, modified)
			if err != nil {
				t.Fatalf(
					"header with sequence number %d and KES period %d failed to decode: %v",
					wideSequenceNumber,
					wideKesPeriod,
					err,
				)
			}
			sequenceNumber, kesPeriod := headerOCertFields(t, header)
			if sequenceNumber != wideSequenceNumber {
				t.Errorf(
					"decoded sequence number %d, want %d",
					sequenceNumber,
					wideSequenceNumber,
				)
			}
			if kesPeriod != wideKesPeriod {
				t.Errorf(
					"decoded KES period %d, want %d",
					kesPeriod,
					wideKesPeriod,
				)
			}
			// The bound the decoder does not carry is the KES expiry
			// check against the block's own slot, which still rejects
			// this certificate.
			if _, err := ledger.ValidateKesPeriod(
				kesPeriod,
				header.SlotNumber(),
				129600,
				62,
			); err == nil {
				t.Error(
					"KES period validation accepted a certificate from the future",
				)
			}
		})
	}
}

// TestGoldenHeaderOCertFieldsRejectNonIntegers keeps the negative controls the
// widened fields still owe: the operational certificate counter and KES period
// are unsigned integers, so a negative value and a byte string are still
// rejected at decode.
func TestGoldenHeaderOCertFieldsRejectNonIntegers(t *testing.T) {
	for _, testDef := range []struct {
		name   string
		golden string
		tpraos bool
	}{
		{name: "Shelley", golden: "Header_Shelley", tpraos: true},
		{name: "Babbage", golden: "Header_Babbage"},
	} {
		t.Run(testDef.name, func(t *testing.T) {
			eraId, headerCbor := readGoldenHeader(t, testDef.golden)
			blockType := ledger.BlockHeaderToBlockTypeMap[eraId]
			for _, badCase := range []struct {
				name  string
				value cbor.RawMessage
			}{
				// -1
				{name: "negative", value: cbor.RawMessage{0x20}},
				// h'00'
				{name: "bytes", value: cbor.RawMessage{0x41, 0x00}},
				// 2^64
				{
					name: "bignum",
					value: cbor.RawMessage{
						0xc2, 0x49, 0x01, 0x00, 0x00,
						0x00, 0x00, 0x00, 0x00, 0x00,
						0x00,
					},
				},
			} {
				t.Run(badCase.name, func(t *testing.T) {
					modified := setOCertRaw(
						t,
						headerCbor,
						testDef.tpraos,
						badCase.value,
					)
					if _, err := ledger.NewBlockHeaderFromCbor(blockType, modified); err == nil {
						t.Errorf(
							"header with a %s sequence number decoded",
							badCase.name,
						)
					}
				})
			}
		})
	}
}

// setOCertRaw replaces the operational certificate sequence number with
// arbitrary CBOR.
func setOCertRaw(
	t *testing.T,
	headerCbor []byte,
	tpraos bool,
	value cbor.RawMessage,
) []byte {
	t.Helper()
	var header []cbor.RawMessage
	if _, err := cbor.Decode(headerCbor, &header); err != nil {
		t.Fatalf("decode header array: %v", err)
	}
	var body []cbor.RawMessage
	if _, err := cbor.Decode(header[0], &body); err != nil {
		t.Fatalf("decode header body array: %v", err)
	}
	if tpraos {
		body[tPraosOCertOffset] = value
	} else {
		var ocert []cbor.RawMessage
		if _, err := cbor.Decode(body[cPraosOCertIndex], &ocert); err != nil {
			t.Fatalf("decode operational certificate array: %v", err)
		}
		ocert[1] = value
		encodedOCert, err := cbor.Encode(ocert)
		if err != nil {
			t.Fatalf("encode operational certificate array: %v", err)
		}
		body[cPraosOCertIndex] = encodedOCert
	}
	encodedBody, err := cbor.Encode(body)
	if err != nil {
		t.Fatalf("encode header body array: %v", err)
	}
	header[0] = encodedBody
	encodedHeader, err := cbor.Encode(header)
	if err != nil {
		t.Fatalf("encode header array: %v", err)
	}
	return encodedHeader
}
