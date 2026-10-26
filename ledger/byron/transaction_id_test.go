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

package byron_test

import (
	"encoding/hex"
	"hash/crc32"
	"sync"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
)

func TestByronTransactionBodyIdConcurrentFirstCall(t *testing.T) {
	var body byron.ByronTransactionBody
	body.SetCbor([]byte{0x82, 0x80, 0x80})
	want := common.Blake2b256Hash(body.Cbor())
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			if got := body.WireId(); got != want {
				t.Errorf("unexpected hash: got %x, want %x", got, want)
			}
		}()
	}
	close(start)
	workers.Wait()
}

func TestByronTransactionBodyIdSetCborInvalidatesOnlyCopy(t *testing.T) {
	var body byron.ByronTransactionBody
	body.SetCbor([]byte{0x82, 0x80, 0x80})
	wantOriginal := body.WireId()

	for name, setCbor := range map[string]func(*byron.ByronTransactionBody, []byte){
		"SetCbor":          (*byron.ByronTransactionBody).SetCbor,
		"SetCborReference": (*byron.ByronTransactionBody).SetCborReference,
	} {
		t.Run(name, func(t *testing.T) {
			copyBody := body
			newWire := []byte{0x82, 0x81, 0x80, 0x80}
			setCbor(&copyBody, newWire)
			if got := body.WireId(); got != wantOriginal {
				t.Fatalf("original hash changed after copy invalidation: got %x, want %x", got, wantOriginal)
			}
			if got, want := copyBody.WireId(), common.Blake2b256Hash(newWire); got != want {
				t.Fatalf("copy hash mismatch after invalidation: got %x, want %x", got, want)
			}

			setCbor(&copyBody, nil)
			if got, want := copyBody.WireId(), common.Blake2b256Hash(nil); got != want {
				t.Fatalf("nil CBOR hash mismatch: got %x, want %x", got, want)
			}
		})
	}
}

func TestByronTransactionIdsSeparateCanonicalAndWireHashes(t *testing.T) {
	address, err := common.NewByronAddressFromParts(
		0,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	if err != nil {
		t.Fatalf("create Byron address: %v", err)
	}
	// Bytes returns the Byron wire address, [#6.24(payload), crc].
	encodedAddress, err := address.Bytes()
	if err != nil {
		t.Fatalf("encode Byron address: %v", err)
	}
	// Byron uses indefinite-length input and output lists. The output amount
	// also uses a valid non-shortest integer encoding.
	wireBody := append([]byte{0x83, 0x9f, 0xff, 0x9f, 0x82}, encodedAddress...)
	wireBody = append(wireBody, 0x18, 0x01, 0xff, 0xa0)
	canonicalBody := append([]byte{0x83, 0x9f, 0xff, 0x9f, 0x82}, encodedAddress...)
	canonicalBody = append(canonicalBody, 0x01, 0xff, 0xa0)
	var body byron.ByronTransactionBody
	if err := body.UnmarshalCBOR(wireBody); err != nil {
		t.Fatalf("decode Byron transaction body: %v", err)
	}
	if got, want := body.WireId(), common.Blake2b256Hash(wireBody); got != want {
		t.Fatalf("wire hash mismatch: got %x, want %x", got, want)
	}
	if got, want := body.Id(), common.Blake2b256Hash(canonicalBody); got != want {
		t.Fatalf("canonical transaction ID mismatch: got %x, want %x", got, want)
	}
	if body.Id() == body.WireId() {
		t.Fatal("non-shortest integer encoding produced identical hashes")
	}
	tx := &byron.ByronTransaction{Body: body}
	produced := tx.Produced()
	if len(produced) != 1 || produced[0].Id.Id() != body.Id() {
		t.Fatalf("produced UTxO did not use canonical transaction ID: %+v", produced)
	}
	var canonical byron.ByronTransactionBody
	if err := canonical.UnmarshalCBOR(canonicalBody); err != nil {
		t.Fatalf("decode canonical transaction body: %v", err)
	}
	if canonical.Id() != canonical.WireId() {
		t.Fatal("canonical transaction hashes differ")
	}
}

func TestByronTransactionBodyIdMatchesWireIdForCanonicalWireForm(t *testing.T) {
	t.Parallel()
	// Mainnet transaction 6497b33b...4d9e (slot 3313): inputs and outputs
	// carry their tag-24 wrappers on the wire.
	const wantId = "6497b33b10fa2619c6efbd9f874ecd1c91badb10bf70850732aab45b90524d9e"
	bodyCbor, err := hex.DecodeString(
		"839f8200d8185824825820a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a200700ff" +
			"9f8282d818584283581c6c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633ca101581e581c010d876783fb2b4d0d17c86df29af8d35356ed3d1827bf4744f06700001a8dc672c11a000f4240ffa0",
	)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	var body byron.ByronTransactionBody
	if err := body.UnmarshalCBOR(bodyCbor); err != nil {
		t.Fatalf("decode Byron transaction body: %v", err)
	}
	if got := body.WireId().String(); got != wantId {
		t.Fatalf("WireId = %s, want %s", got, wantId)
	}
	if got := body.Id().String(); got != wantId {
		t.Fatalf("Id = %s, want %s (WireId)", got, wantId)
	}
}

func TestByronInputOutputMarshalCBORRoundTripsWireForm(t *testing.T) {
	t.Parallel()
	address, err := common.NewByronAddressFromParts(
		0,
		make([]byte, common.AddressHashSize),
		common.ByronAddressAttributes{},
	)
	if err != nil {
		t.Fatalf("create Byron address: %v", err)
	}
	input, err := byron.NewByronTransactionInput(
		"a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a2007", 3,
	)
	if err != nil {
		t.Fatalf("create input: %v", err)
	}
	inputCbor, err := cbor.Encode(input)
	if err != nil {
		t.Fatalf("encode input: %v", err)
	}
	// [0, #6.24(...)]
	if len(inputCbor) < 3 || inputCbor[0] != 0x82 || inputCbor[1] != 0x00 || inputCbor[2] != 0xd8 {
		t.Fatalf("input not in wire form: %x", inputCbor)
	}
	var gotInput byron.ByronTransactionInput
	if err := gotInput.UnmarshalCBOR(inputCbor); err != nil {
		t.Fatalf("decode encoded input: %v", err)
	}
	if gotInput.Id() != input.Id() || gotInput.Index() != input.Index() {
		t.Fatalf("input round trip mismatch: %v != %v", gotInput, input)
	}
	output := byron.ByronTransactionOutput{OutputAddress: address, OutputAmount: 1}
	outputCbor, err := cbor.Encode(output)
	if err != nil {
		t.Fatalf("encode output: %v", err)
	}
	// [[#6.24(...), crc], amount]: the address array follows the output
	// array header, and the address payload is tag 24 wrapped.
	if len(outputCbor) < 3 || outputCbor[0] != 0x82 || outputCbor[1] != 0x82 || outputCbor[2] != 0xd8 {
		t.Fatalf("output not in wire form: %x", outputCbor)
	}
	var gotOutput byron.ByronTransactionOutput
	if err := gotOutput.UnmarshalCBOR(outputCbor); err != nil {
		t.Fatalf("decode encoded output: %v", err)
	}
	if gotOutput.Address().String() != address.String() || gotOutput.Amount().Uint64() != 1 {
		t.Fatalf("output round trip mismatch: %v", gotOutput)
	}
}

func TestByronTransactionBodyIdMatchesWireIdForBuiltOutputs(t *testing.T) {
	t.Parallel()
	bodyCbor, err := hex.DecodeString(
		"839f8200d8185824825820a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a200700ff" +
			"9f8282d818584283581c6c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633ca101581e581c010d876783fb2b4d0d17c86df29af8d35356ed3d1827bf4744f06700001a8dc672c11a000f4240ffa0",
	)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	var decoded byron.ByronTransactionBody
	if err := decoded.UnmarshalCBOR(bodyCbor); err != nil {
		t.Fatalf("decode Byron transaction body: %v", err)
	}
	// An output built in code has no stored CBOR, so it must encode to the
	// same wire bytes the decoded output carried.
	wireOutput := decoded.TxOutputs[0]
	built := byron.ByronTransactionOutput{
		OutputAddress: wireOutput.OutputAddress,
		OutputAmount:  wireOutput.OutputAmount,
	}
	builtCbor, err := cbor.Encode(built)
	if err != nil {
		t.Fatalf("encode built output: %v", err)
	}
	if hex.EncodeToString(builtCbor) != hex.EncodeToString(wireOutput.Cbor()) {
		t.Fatalf("built output = %x, want wire form %x", builtCbor, wireOutput.Cbor())
	}
	body := byron.ByronTransactionBody{
		TxInputs:   decoded.TxInputs,
		TxOutputs:  []byron.ByronTransactionOutput{built},
		Attributes: decoded.Attributes,
	}
	if got, want := body.Id(), decoded.WireId(); got != want {
		t.Fatalf("Id of body with built output = %s, want %s", got, want)
	}
}

// byronTxBodyWithAddress returns a one-input, one-output Byron transaction
// body whose output address is addrHex.
func byronTxBodyWithAddress(t *testing.T, addrHex string) []byte {
	t.Helper()
	body, err := hex.DecodeString(
		"839f8200d8185824825820a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a200700ff" +
			"9f82" + addrHex + "1a000f4240ffa0",
	)
	if err != nil {
		t.Fatalf("decode body hex: %v", err)
	}
	return body
}

// byronAddressHex builds [tag24Hdr || bstrHdr || payload, crc] with the CRC
// encoded by crcHdr, a CBOR uint head of 1, 2, 3, 5, or 9 bytes.
func byronAddressHex(t *testing.T, tag24Hdr, bstrHdr string, payload []byte, crcHdr string) string {
	t.Helper()
	crc := crc32.ChecksumIEEE(payload)
	var crcBytes []byte
	switch crcHdr {
	case "":
		var err error
		if crcBytes, err = cbor.Encode(crc); err != nil {
			t.Fatalf("encode crc: %v", err)
		}
	case "1b":
		crcBytes = []byte{0x1b, 0, 0, 0, 0, byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)}
	default:
		t.Fatalf("unsupported crc head %q", crcHdr)
	}
	return "82" + tag24Hdr + bstrHdr + hex.EncodeToString(payload) + hex.EncodeToString(crcBytes)
}

func TestByronTransactionBodyIdCanonicalizesOutputAddress(t *testing.T) {
	t.Parallel()
	const hash = "581c6c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633c"
	// Attributes: 1 => derivation path bytes(h'aabbcc'), 2 => network magic.
	const (
		derivation = "44" + "43aabbcc"
		magic      = "45" + "1a2d964a09"
	)
	payloadOf := func(attrs string) []byte {
		b, err := hex.DecodeString("83" + hash + attrs + "00")
		if err != nil {
			t.Fatalf("decode payload hex: %v", err)
		}
		return b
	}
	canonicalPayload := payloadOf("a2" + "01" + derivation + "02" + magic)
	bstrHdr := func(n int) string { return hex.EncodeToString([]byte{0x58, byte(n)}) }
	canonicalAddr := byronAddressHex(t, "d818", bstrHdr(len(canonicalPayload)), canonicalPayload, "")
	canonicalBody := byronTxBodyWithAddress(t, canonicalAddr)
	var canonical byron.ByronTransactionBody
	if err := canonical.UnmarshalCBOR(canonicalBody); err != nil {
		t.Fatalf("decode canonical body: %v", err)
	}
	if canonical.Id() != canonical.WireId() {
		t.Fatalf("canonical body Id %s differs from WireId %s", canonical.Id(), canonical.WireId())
	}
	reorderedPayload := payloadOf("a2" + "02" + magic + "01" + derivation)
	longKeyPayload := payloadOf("a2" + "1801" + derivation + "02" + magic)
	longValuePayload := payloadOf("a2" + "01" + "5804" + "43aabbcc" + "02" + magic)
	for name, addr := range map[string]string{
		"non-shortest CRC": byronAddressHex(
			t, "d818", bstrHdr(len(canonicalPayload)), canonicalPayload, "1b",
		),
		"non-shortest payload length": byronAddressHex(
			t, "d818", "5900"+hex.EncodeToString([]byte{byte(len(canonicalPayload))}),
			canonicalPayload, "",
		),
		"non-shortest tag 24 head": byronAddressHex(
			t, "d90018", bstrHdr(len(canonicalPayload)), canonicalPayload, "",
		),
		"attributes out of key order": byronAddressHex(
			t, "d818", bstrHdr(len(reorderedPayload)), reorderedPayload, "",
		),
		"non-shortest attribute key": byronAddressHex(
			t, "d818", bstrHdr(len(longKeyPayload)), longKeyPayload, "",
		),
		"non-shortest attribute value length": byronAddressHex(
			t, "d818", bstrHdr(len(longValuePayload)), longValuePayload, "",
		),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wire := byronTxBodyWithAddress(t, addr)
			var body byron.ByronTransactionBody
			if err := body.UnmarshalCBOR(wire); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.WireId() == canonical.WireId() {
				t.Fatal("variant encoding is byte-identical to the canonical body")
			}
			if got, want := body.Id(), canonical.Id(); got != want {
				t.Fatalf("Id = %s, want canonical %s", got, want)
			}
			tx := &byron.ByronTransaction{Body: body}
			produced := tx.Produced()
			if len(produced) != 1 || produced[0].Id.Id() != canonical.Id() {
				t.Fatalf("produced UTxO not keyed by canonical Id: %+v", produced)
			}
		})
	}
}

func TestByronTransactionOutputEncodesNonByronAddressAsByteString(t *testing.T) {
	t.Parallel()
	// The Byron output decoder accepts a Shelley address byte string, so the
	// encoder must emit it as a CBOR item rather than as raw address bytes.
	wire := byronTxBodyWithAddress(
		t, "581d616c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633c",
	)
	var decoded byron.ByronTransactionBody
	if err := decoded.UnmarshalCBOR(wire); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if got, want := decoded.Id(), decoded.WireId(); got != want {
		t.Fatalf("Id = %s, want WireId %s", got, want)
	}
	built := byron.ByronTransactionBody{
		TxInputs: decoded.TxInputs,
		TxOutputs: []byron.ByronTransactionOutput{{
			OutputAddress: decoded.TxOutputs[0].OutputAddress,
			OutputAmount:  decoded.TxOutputs[0].OutputAmount,
		}},
		Attributes: decoded.Attributes,
	}
	if got, want := built.Id(), decoded.WireId(); got != want {
		t.Fatalf("Id of body with built output = %s, want %s", got, want)
	}
}

func TestByronTransactionBodyIdCanonicalizesInputsAndFraming(t *testing.T) {
	t.Parallel()
	const (
		txHash = "a12a839c25a01fa5d118167db5acdbd9e38172ae8f00e5ac0a4997ef792a2007"
		input  = "8200d8185824825820" + txHash + "00"
		output = "8282d818584283581c6c9982e7f2b6dcc5eaa880e8014568913c8868d9f0f86eb687b2633ca101581e581c010d876783fb2b4d0d17c86df29af8d35356ed3d1827bf4744f06700001a8dc672c11a000f4240"
	)
	decode := func(t *testing.T, h string) byron.ByronTransactionBody {
		t.Helper()
		raw, err := hex.DecodeString(h)
		if err != nil {
			t.Fatalf("decode hex: %v", err)
		}
		var body byron.ByronTransactionBody
		if err := body.UnmarshalCBOR(raw); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		return body
	}
	canonical := decode(t, "839f"+input+"ff9f"+output+"ffa0")
	if got := canonical.Id().String(); got != "6497b33b10fa2619c6efbd9f874ecd1c91badb10bf70850732aab45b90524d9e" {
		t.Fatalf("canonical Id = %s", got)
	}
	for name, h := range map[string]string{
		"non-shortest input index":       "839f" + "8200d8185825825820" + txHash + "1800" + "ff9f" + output + "ffa0",
		"non-shortest input tag 24 head": "839f" + "8200d900185824825820" + txHash + "00" + "ff9f" + output + "ffa0",
		"definite-length lists":          "8381" + input + "81" + output + "a0",
		"non-shortest amount":            "839f" + input + "ff9f" + output[:len(output)-10] + "1b00000000000f4240" + "ffa0",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			body := decode(t, h)
			if body.WireId() == canonical.WireId() {
				t.Fatal("variant encoding is byte-identical to the canonical body")
			}
			if got, want := body.Id(), canonical.Id(); got != want {
				t.Fatalf("Id = %s, want canonical %s", got, want)
			}
		})
	}
	t.Run("non-shortest transaction attribute key", func(t *testing.T) {
		t.Parallel()
		want := decode(t, "839f"+input+"ff9f"+output+"ffa1014100")
		body := decode(t, "839f"+input+"ff9f"+output+"ffa118014100")
		if want.Id() != want.WireId() {
			t.Fatalf("canonical attributes: Id %s differs from WireId %s", want.Id(), want.WireId())
		}
		if got := body.Id(); got != want.Id() {
			t.Fatalf("Id = %s, want canonical %s", got, want.Id())
		}
	})
}
