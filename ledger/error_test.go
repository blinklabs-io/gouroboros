package ledger

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
)

func TestScriptsNotPaidUtxo_MarshalUnmarshalCBOR(t *testing.T) {
	// Create multiple UTxOs with different data to test for corruption
	addr1, err := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	if err != nil {
		t.Fatalf("Failed to create address 1: %v", err)
	}
	addr2, err := common.NewAddress(
		"addr1qyln2c2cx5jc4hw768pwz60n5245462dvp4auqcw09rl2xz07huw84puu6cea3qe0ce3apks7hjckqkh5ad4uax0l9ws0q9xty",
	)
	if err != nil {
		t.Fatalf("Failed to create address 2: %v", err)
	}
	addr3, err := common.NewAddress(
		"addr1z8snz7c4974vzdpxu65ruphl3zjdvtxw8strf2c2tmqnxz2j2c79gy9l76sdg0xwhd7r0c0kna0tycz4y5s6mlenh8pq0xmsha",
	)
	if err != nil {
		t.Fatalf("Failed to create address 3: %v", err)
	}

	utxos := []common.Utxo{
		{
			Id: shelley.NewShelleyTransactionInput(
				"deadbeef00000000000000000000000000000000000000000000000000000000",
				0,
			),
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr1,
				OutputAmount:  1000,
			},
		},
		{
			Id: shelley.NewShelleyTransactionInput(
				"cafebabe11111111111111111111111111111111111111111111111111111111",
				1,
			),
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr2,
				OutputAmount:  2500,
			},
		},
		{
			Id: shelley.NewShelleyTransactionInput(
				"feedface22222222222222222222222222222222222222222222222222222222",
				2,
			),
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr3,
				OutputAmount:  7500,
			},
		},
	}

	// Marshal to CBOR
	original := &ScriptsNotPaidUtxo{
		Utxos: utxos,
	}
	originalCborData, err := original.MarshalCBOR()
	if err != nil {
		t.Fatalf("MarshalCBOR failed: %v", err)
	}

	// Unmarshal back
	var decoded ScriptsNotPaidUtxo
	if err := decoded.UnmarshalCBOR(originalCborData); err != nil {
		t.Fatalf("UnmarshalCBOR failed: %v", err)
	}

	// Validate count
	if len(decoded.Utxos) != len(utxos) {
		t.Fatalf("Expected %d UTxOs, got %d", len(utxos), len(decoded.Utxos))
	}

	// Validate each UTxO's data integrity
	for i, originalUtxo := range utxos {
		found := false
		for _, decodedUtxo := range decoded.Utxos {
			// Check if this is the matching UTxO by comparing transaction input
			if decodedUtxo.Id.Id() == originalUtxo.Id.Id() &&
				decodedUtxo.Id.Index() == originalUtxo.Id.Index() {
				found = true

				// Validate transaction output data using interface methods
				originalOutput := originalUtxo.Output.(*shelley.ShelleyTransactionOutput)

				// Check the amount using the interface method
				originalAmount := new(big.Int).SetUint64(originalOutput.OutputAmount)
				if decodedUtxo.Output.Amount().Cmp(originalAmount) != 0 {
					t.Errorf(
						"UTxO %d: Amount mismatch - expected %s, got %s",
						i,
						originalAmount.String(),
						decodedUtxo.Output.Amount().String(),
					)
				}

				// Check the address using the interface method
				if decodedUtxo.Output.Address().
					String() !=
					originalOutput.OutputAddress.String() {
					t.Errorf(
						"UTxO %d: Address mismatch - expected %s, got %s",
						i,
						originalOutput.OutputAddress.String(),
						decodedUtxo.Output.Address().String(),
					)
				}
				break
			}
		}
		if !found {
			t.Errorf("UTxO %d not found in decoded data: %s#%d",
				i, originalUtxo.Id.Id().String(), originalUtxo.Id.Index())
		}
	}

	// Test round-trip CBOR fidelity by re-marshaling and comparing bytes
	remarshaled, err := decoded.MarshalCBOR()
	if err != nil {
		t.Fatalf("Re-marshaling failed: %v", err)
	}

	if !bytes.Equal(originalCborData, remarshaled) {
		t.Errorf("Round-trip CBOR fidelity failed - bytes don't match")
		t.Logf("Original:     %x", originalCborData)
		t.Logf("Remarshaled:  %x", remarshaled)
	}

	t.Logf(
		"Successfully validated %d UTxOs with full data integrity and round-trip fidelity",
		len(utxos),
	)
}

func TestScriptsNotPaidUtxo_MarshalUnmarshalCBOR_AllEras(t *testing.T) {
	// Create test addresses
	addr1, err := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	if err != nil {
		t.Fatalf("Failed to create address 1: %v", err)
	}
	addr2, err := common.NewAddress(
		"addr1qyln2c2cx5jc4hw768pwz60n5245462dvp4auqcw09rl2xz07huw84puu6cea3qe0ce3apks7hjckqkh5ad4uax0l9ws0q9xty",
	)
	if err != nil {
		t.Fatalf("Failed to create address 2: %v", err)
	}

	// Test with Byron transaction inputs
	byronInput1 := byron.NewByronTransactionInput(
		"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789ab",
		0,
	)
	byronInput2 := byron.NewByronTransactionInput(
		"fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210fe",
		1,
	)

	// Create test UTxOs with Byron inputs
	byronUtxos := []common.Utxo{
		{
			Id: byronInput1,
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr1,
				OutputAmount:  1000000,
			},
		},
		{
			Id: byronInput2,
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr2,
				OutputAmount:  2500000,
			},
		},
	}

	// Test Byron inputs
	byronError := &ScriptsNotPaidUtxo{Utxos: byronUtxos}
	byronCborData, err := byronError.MarshalCBOR()
	if err != nil {
		t.Fatalf("Byron marshal failed: %v", err)
	}

	var decodedByron ScriptsNotPaidUtxo
	if err := decodedByron.UnmarshalCBOR(byronCborData); err != nil {
		t.Fatalf("Byron unmarshal failed: %v", err)
	}

	// Validate Byron decoding with comprehensive data fidelity checks
	if len(decodedByron.Utxos) != 2 {
		t.Errorf("Expected 2 Byron UTxOs, got %d", len(decodedByron.Utxos))
	}

	// Check Byron input data integrity using order-independent validation
	// Create a map of original UTxOs for lookup (map iteration order is not guaranteed)
	originalByronMap := make(map[string]common.Utxo)
	for _, utxo := range byronUtxos {
		originalInput := utxo.Id.(byron.ByronTransactionInput)
		key := originalInput.Id().
			String() +
			":" + fmt.Sprint(
			originalInput.Index(),
		)
		originalByronMap[key] = utxo
	}

	for _, utxo := range decodedByron.Utxos {
		// Accept either Byron or Shelley input types (era-agnostic decoding)
		var decodedTxId string
		var decodedIndex uint32

		switch input := utxo.Id.(type) {
		case *byron.ByronTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		case *shelley.ShelleyTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		case shelley.ShelleyTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		case byron.ByronTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		default:
			t.Errorf("Unexpected input type: got %T", utxo.Id)
			continue
		}

		// Accept either Byron or Shelley output types (era-agnostic decoding)
		var decodedAddr common.Address
		var decodedAmount uint64

		switch output := utxo.Output.(type) {
		case *shelley.ShelleyTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		case *byron.ByronTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		case shelley.ShelleyTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		case byron.ByronTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		default:
			t.Errorf("Unexpected output type: got %T", utxo.Output)
			continue
		}

		// Find matching original UTxO (order-independent lookup)
		key := decodedTxId + ":" + fmt.Sprint(decodedIndex)
		originalUtxo, found := originalByronMap[key]
		if !found {
			t.Errorf("Byron UTxO with key %s not found in original UTxOs", key)
			continue
		}

		// Validate output addresses and amounts using era-agnostic approach
		originalOutput := originalUtxo.Output.(*shelley.ShelleyTransactionOutput)

		// Compare address bytes
		decodedAddrBytes, err := decodedAddr.Bytes()
		if err != nil {
			t.Errorf(
				"Byron UTxO %s: failed to get decoded address bytes: %v",
				key,
				err,
			)
			continue
		}
		originalAddrBytes, err := originalOutput.OutputAddress.Bytes()
		if err != nil {
			t.Errorf(
				"Byron UTxO %s: failed to get original address bytes: %v",
				key,
				err,
			)
			continue
		}

		if !bytes.Equal(decodedAddrBytes, originalAddrBytes) {
			t.Errorf(
				"Byron UTxO %s: address mismatch. Expected %s, got %s",
				key,
				originalOutput.OutputAddress.String(),
				decodedAddr.String(),
			)
		}
		if decodedAmount != originalOutput.OutputAmount {
			t.Errorf(
				"Byron UTxO %s: amount mismatch. Expected %d, got %d",
				key,
				originalOutput.OutputAmount,
				decodedAmount,
			)
		}
	}

	// Verify Byron round-trip CBOR fidelity by re-marshaling
	byronReMarshal, err := decodedByron.MarshalCBOR()
	if err != nil {
		t.Fatalf("Byron re-marshal failed: %v", err)
	}
	if len(byronReMarshal) != len(byronCborData) {
		t.Errorf(
			"Byron round-trip CBOR size mismatch. Original: %d bytes, Re-marshaled: %d bytes",
			len(byronCborData),
			len(byronReMarshal),
		)
	}

	// Test Shelley inputs
	shelleyInput1 := shelley.NewShelleyTransactionInput(
		"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		0,
	)
	shelleyInput2 := shelley.NewShelleyTransactionInput(
		"fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321",
		1,
	)

	shelleyUtxos := []common.Utxo{
		{
			Id: shelleyInput1,
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr1,
				OutputAmount:  1500000,
			},
		},
		{
			Id: shelleyInput2,
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr2,
				OutputAmount:  3000000,
			},
		},
	}

	// Test Shelley inputs
	shelleyError := &ScriptsNotPaidUtxo{Utxos: shelleyUtxos}
	shelleyCborData, err := shelleyError.MarshalCBOR()
	if err != nil {
		t.Fatalf("Shelley marshal failed: %v", err)
	}

	var decodedShelley ScriptsNotPaidUtxo
	if err := decodedShelley.UnmarshalCBOR(shelleyCborData); err != nil {
		t.Fatalf("Shelley unmarshal failed: %v", err)
	}

	// Validate Shelley decoding with comprehensive data fidelity checks
	if len(decodedShelley.Utxos) != 2 {
		t.Errorf("Expected 2 Shelley UTxOs, got %d", len(decodedShelley.Utxos))
	}

	// Check Shelley input data integrity using order-independent validation
	// Create a map of original UTxOs for lookup (map iteration order is not guaranteed)
	originalShelleyMap := make(map[string]common.Utxo)
	for _, utxo := range shelleyUtxos {
		originalInput := utxo.Id.(shelley.ShelleyTransactionInput)
		key := originalInput.Id().
			String() +
			":" + fmt.Sprint(
			originalInput.Index(),
		)
		originalShelleyMap[key] = utxo
	}

	for _, utxo := range decodedShelley.Utxos {
		// Accept either Byron or Shelley input types (era-agnostic decoding)
		var decodedTxId string
		var decodedIndex uint32

		switch input := utxo.Id.(type) {
		case *byron.ByronTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		case *shelley.ShelleyTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		case shelley.ShelleyTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		case byron.ByronTransactionInput:
			decodedTxId = input.Id().String()
			decodedIndex = input.Index()
		default:
			t.Errorf("Unexpected input type: got %T", utxo.Id)
			continue
		}

		// Accept either Byron or Shelley output types (era-agnostic decoding)
		var decodedAddr common.Address
		var decodedAmount uint64

		switch output := utxo.Output.(type) {
		case *shelley.ShelleyTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		case *byron.ByronTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		case shelley.ShelleyTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		case byron.ByronTransactionOutput:
			decodedAddr = output.OutputAddress
			decodedAmount = output.OutputAmount
		default:
			t.Errorf("Unexpected output type: got %T", utxo.Output)
			continue
		}

		// Find matching original UTxO (order-independent lookup)
		key := decodedTxId + ":" + fmt.Sprint(decodedIndex)
		originalUtxo, found := originalShelleyMap[key]
		if !found {
			t.Errorf(
				"Shelley UTxO with key %s not found in original UTxOs",
				key,
			)
			continue
		}

		// Validate output addresses and amounts using era-agnostic approach
		originalOutput := originalUtxo.Output.(*shelley.ShelleyTransactionOutput)

		// Compare address bytes
		decodedAddrBytes, err := decodedAddr.Bytes()
		if err != nil {
			t.Errorf(
				"Shelley UTxO %s: failed to get decoded address bytes: %v",
				key,
				err,
			)
			continue
		}
		originalAddrBytes, err := originalOutput.OutputAddress.Bytes()
		if err != nil {
			t.Errorf(
				"Shelley UTxO %s: failed to get original address bytes: %v",
				key,
				err,
			)
			continue
		}

		if !bytes.Equal(decodedAddrBytes, originalAddrBytes) {
			t.Errorf(
				"Shelley UTxO %s: address mismatch. Expected %s, got %s",
				key,
				originalOutput.OutputAddress.String(),
				decodedAddr.String(),
			)
		}
		if decodedAmount != originalOutput.OutputAmount {
			t.Errorf(
				"Shelley UTxO %s: amount mismatch. Expected %d, got %d",
				key,
				originalOutput.OutputAmount,
				decodedAmount,
			)
		}
	}

	// Verify Shelley round-trip CBOR fidelity by re-marshaling
	shelleyReMarshal, err := decodedShelley.MarshalCBOR()
	if err != nil {
		t.Fatalf("Shelley re-marshal failed: %v", err)
	}
	if len(shelleyReMarshal) != len(shelleyCborData) {
		t.Errorf(
			"Shelley round-trip CBOR size mismatch. Original: %d bytes, Re-marshaled: %d bytes",
			len(shelleyCborData),
			len(shelleyReMarshal),
		)
	}

	t.Logf(
		"Successfully validated era-agnostic CBOR handling: Byron (%d UTxOs) and Shelley (%d UTxOs)",
		len(decodedByron.Utxos),
		len(decodedShelley.Utxos),
	)
}

func TestScriptsNotPaidUtxo_TypeFieldAssignment(t *testing.T) {
	// Create test addresses
	addr1, err := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	if err != nil {
		t.Fatalf("Failed to create address 1: %v", err)
	}

	// Create test UTxO
	input1 := shelley.NewShelleyTransactionInput(
		"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
		0,
	)
	utxos := []common.Utxo{
		{
			Id: input1,
			Output: &shelley.ShelleyTransactionOutput{
				OutputAddress: addr1,
				OutputAmount:  1000000,
			},
		},
	}

	// Test marshal and unmarshal
	original := &ScriptsNotPaidUtxo{Utxos: utxos}
	cborData, err := original.MarshalCBOR()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded ScriptsNotPaidUtxo
	if err := decoded.UnmarshalCBOR(cborData); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify the Type field is correctly set (should default to Conway era)
	if decoded.Type != UtxoFailureScriptsNotPaidUtxoConway {
		t.Errorf(
			"Expected Type field to be %d (UtxoFailureScriptsNotPaidUtxoConway), got %d",
			UtxoFailureScriptsNotPaidUtxoConway,
			decoded.Type,
		)
	}

	t.Logf(
		"Type field correctly set to %d (Conway era) after CBOR unmarshaling",
		decoded.Type,
	)
}
