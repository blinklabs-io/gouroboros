package shelley_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShelleyTransactionInputSetCoalescesUntaggedDuplicates(t *testing.T) {
	txId := make([]byte, common.Blake2b256Size)
	txId[0] = 1
	// The first index is the non-minimal encoding 0x1800. The second copy is
	// canonical, so normalization must not be used for wire round-tripping.
	wire := []byte{0x82, 0x82, 0x58, 0x20}
	wire = append(wire, txId...)
	wire = append(wire, 0x18, 0x00, 0x82, 0x58, 0x20)
	wire = append(wire, txId...)
	wire = append(wire, 0x00)

	var set shelley.ShelleyTransactionInputSet
	_, err := cbor.Decode(wire, &set)
	require.NoError(t, err)
	expected := shelley.ShelleyTransactionInput{
		TxId:        common.Blake2b256(txId),
		OutputIndex: 0,
	}
	assert.Equal(t, []shelley.ShelleyTransactionInput{expected}, set.Items())

	encoded, err := cbor.Encode(&set)
	require.NoError(t, err)
	assert.Equal(t, wire, encoded)
}

func TestShelleyTransactionInputSetOnlyCoalescesOnDecode(t *testing.T) {
	input := shelley.NewShelleyTransactionInput(
		"0101010101010101010101010101010101010101010101010101010101010101",
		0,
	)
	set := shelley.NewShelleyTransactionInputSet(
		[]shelley.ShelleyTransactionInput{input, input},
	)

	assert.Equal(t, []shelley.ShelleyTransactionInput{input, input}, set.Items())
}

func TestShelleyTransactionOutputString(t *testing.T) {
	addrStr := "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"
	addr, _ := common.NewAddress(addrStr)
	out := shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	s := out.String()
	expected := fmt.Sprintf(
		"(ShelleyTransactionOutput address=%s amount=456)",
		addrStr,
	)
	if s != expected {
		t.Fatalf("unexpected string: %s", s)
	}
}

func TestShelleyOutputTooSmallErrorFormatting(t *testing.T) {
	addrStr := "addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd"
	addr, _ := common.NewAddress(addrStr)
	out := &shelley.ShelleyTransactionOutput{
		OutputAddress: addr,
		OutputAmount:  456,
	}
	errStr := shelley.OutputTooSmallUtxoError{
		Outputs: []common.TransactionOutput{out},
	}.Error()
	expected := fmt.Sprintf(
		"output too small: (ShelleyTransactionOutput address=%s amount=456)",
		addrStr,
	)
	if errStr != expected {
		t.Fatalf("unexpected error: %s", errStr)
	}
}

func encodeShelleyHeaderWithPrevHash(
	t *testing.T,
	prevHash cbor.RawMessage,
) ([]byte, []byte) {
	t.Helper()
	bodyCbor, err := cbor.Encode([]any{
		uint64(0),
		uint64(0),
		prevHash,
		common.IssuerVkey{},
		[]byte{0},
		common.VrfResult{},
		common.VrfResult{},
		uint64(0),
		common.Blake2b256{},
		[]byte{0},
		uint32(0),
		uint32(0),
		[]byte{0},
		uint64(0),
		uint64(0),
	})
	require.NoError(t, err)
	headerCbor, err := cbor.Encode([]any{
		cbor.RawMessage(bodyCbor),
		[]byte{0},
	})
	require.NoError(t, err)
	return headerCbor, bodyCbor
}

// TestShelleyBlockHeaderPreviousHashDecoding mirrors the Babbage coverage: this
// header body serves Shelley through Alonzo, so an origin header rejected here
// makes a chain starting in any of those eras undecodable from its first block.
func TestShelleyBlockHeaderPreviousHashDecoding(t *testing.T) {
	t.Run("origin null", func(t *testing.T) {
		headerCbor, bodyCbor := encodeShelleyHeaderWithPrevHash(
			t,
			cbor.RawMessage{0xf6},
		)
		header, err := shelley.NewShelleyBlockHeaderFromCbor(headerCbor)
		require.NoError(t, err)
		assert.Equal(t, common.Blake2b256{}, header.PrevHash())
		assert.Equal(t, bodyCbor, header.Body.Cbor())
		assert.Equal(t, headerCbor, header.Cbor())
		assert.Equal(t, common.Blake2b256Hash(headerCbor), header.Hash())
	})

	t.Run("exact hash", func(t *testing.T) {
		expected := common.Blake2b256(bytes.Repeat([]byte{0x42}, 32))
		prevHash, err := cbor.Encode(expected)
		require.NoError(t, err)
		headerCbor, _ := encodeShelleyHeaderWithPrevHash(
			t,
			cbor.RawMessage(prevHash),
		)
		header, err := shelley.NewShelleyBlockHeaderFromCbor(headerCbor)
		require.NoError(t, err)
		assert.Equal(t, expected, header.PrevHash())
	})

	t.Run("short hash", func(t *testing.T) {
		prevHash, err := cbor.Encode([]byte{0x42})
		require.NoError(t, err)
		headerCbor, _ := encodeShelleyHeaderWithPrevHash(
			t,
			cbor.RawMessage(prevHash),
		)
		_, err = shelley.NewShelleyBlockHeaderFromCbor(headerCbor)
		require.ErrorContains(t, err, "expected 32 bytes, got 1")
	})

	t.Run("non-byte value", func(t *testing.T) {
		headerCbor, _ := encodeShelleyHeaderWithPrevHash(
			t,
			cbor.RawMessage{0x00},
		)
		_, err := shelley.NewShelleyBlockHeaderFromCbor(headerCbor)
		require.ErrorContains(t, err, "expected CBOR byte string")
	})

	t.Run("global hash remains strict", func(t *testing.T) {
		var hash common.Blake2b256
		_, err := cbor.Decode(cbor.RawMessage{0xf6}, &hash)
		require.ErrorContains(t, err, "expected CBOR byte string")
	})
}
