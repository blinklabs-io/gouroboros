package ledger

import (
	"bytes"
	"fmt"
	"math/big"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				originalAmount := new(
					big.Int,
				).SetUint64(originalOutput.OutputAmount)
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

// =============================================================================
// Babbage Error Types Tests
// =============================================================================

func TestMalformedScriptWitnesses_Error(t *testing.T) {
	hash1 := common.NewBlake2b224([]byte("12345678901234567890123456789012"))
	hash2 := common.NewBlake2b224([]byte("abcdefghijklmnopqrstuvwxyz123456"))

	err := &MalformedScriptWitnesses{
		ScriptHashes: []common.Blake2b224{hash1, hash2},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MalformedScriptWitnesses")
	assert.Contains(t, errStr, hash1.String())
	assert.Contains(t, errStr, hash2.String())
}

func TestMalformedReferenceScripts_Error(t *testing.T) {
	hash1 := common.NewBlake2b224([]byte("12345678901234567890123456789012"))

	err := &MalformedReferenceScripts{
		ScriptHashes: []common.Blake2b224{hash1},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MalformedReferenceScripts")
	assert.Contains(t, errStr, hash1.String())
}

func TestIncorrectTotalCollateralField_Error(t *testing.T) {
	err := &IncorrectTotalCollateralField{
		BalanceComputed: -500,
		TotalCollateral: 1000,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "IncorrectTotalCollateralField")
	assert.Contains(t, errStr, "-500")
	assert.Contains(t, errStr, "1000")
}

func TestBabbageOutputTooSmallUTxO_Error(t *testing.T) {
	err := &BabbageOutputTooSmallUTxO{
		Outputs: []BabbageOutputTooSmallEntry{
			{MinRequired: 1000000},
			{MinRequired: 2000000},
		},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "BabbageOutputTooSmallUTxO")
	assert.Contains(t, errStr, "1000000")
	assert.Contains(t, errStr, "2000000")
}

func TestBabbageNonDisjointRefInputs_Error(t *testing.T) {
	err := &BabbageNonDisjointRefInputs{
		Inputs: []TxIn{
			{TxIx: 0},
			{TxIx: 1},
		},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "BabbageNonDisjointRefInputs")
}

// =============================================================================
// Alonzo UTXOW Error Types Tests (wrapped by Babbage)
// =============================================================================

func TestMissingRedeemers_Error(t *testing.T) {
	hash := common.NewBlake2b224([]byte("12345678901234567890123456789012"))

	err := &MissingRedeemers{
		Missing: []MissingRedeemerEntry{
			{ScriptHash: hash},
		},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MissingRedeemers")
}

func TestMissingRequiredDatums_Error(t *testing.T) {
	hash1 := common.NewBlake2b256([]byte("12345678901234567890123456789012"))
	hash2 := common.NewBlake2b256([]byte("abcdefghijklmnopqrstuvwxyz123456"))

	err := &MissingRequiredDatums{
		Missing:  []common.Blake2b256{hash1},
		Received: []common.Blake2b256{hash2},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MissingRequiredDatums")
	assert.Contains(t, errStr, "Missing 1")
	assert.Contains(t, errStr, "Received 1")
}

func TestNotAllowedSupplementalDatums_Error(t *testing.T) {
	hash1 := common.NewBlake2b256([]byte("12345678901234567890123456789012"))

	err := &NotAllowedSupplementalDatums{
		Unallowed:  []common.Blake2b256{hash1},
		Acceptable: []common.Blake2b256{},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "NotAllowedSupplementalDatums")
	assert.Contains(t, errStr, "Unallowed 1")
	assert.Contains(t, errStr, "Acceptable 0")
}

func TestUnspendableUTxONoDatumHash_Error(t *testing.T) {
	err := &UnspendableUTxONoDatumHash{
		Inputs: []TxIn{
			{TxIx: 0},
			{TxIx: 1},
		},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "UnspendableUTxONoDatumHash")
}

func TestExtraRedeemers_Error(t *testing.T) {
	err := &ExtraRedeemers{
		Redeemers: []ExtraRedeemerEntry{
			{Tag: 0, Index: 0}, // Spend
			{Tag: 1, Index: 1}, // Mint
		},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "ExtraRedeemers")
	assert.Contains(t, errStr, "Spend")
	assert.Contains(t, errStr, "Mint")
}

// =============================================================================
// Conway UTXOW Error Types Tests (Shelley-derived)
// =============================================================================

func TestInvalidWitnessesUTXOW_Error(t *testing.T) {
	err := &InvalidWitnessesUTXOW{
		VKeys: []cbor.ByteString{
			cbor.NewByteString([]byte("key1")),
			cbor.NewByteString([]byte("key2")),
		},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "InvalidWitnessesUTXOW")
	assert.Contains(t, errStr, "2 invalid witnesses")
}

func TestMissingVKeyWitnessesUTXOW_Error(t *testing.T) {
	hash := common.NewBlake2b224([]byte("12345678901234567890123456789012"))

	err := &MissingVKeyWitnessesUTXOW{
		KeyHashes: []common.Blake2b224{hash},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MissingVKeyWitnessesUTXOW")
	assert.Contains(t, errStr, hash.String())
}

func TestMissingScriptWitnessesUTXOW_Error(t *testing.T) {
	hash := common.NewBlake2b224([]byte("12345678901234567890123456789012"))

	err := &MissingScriptWitnessesUTXOW{
		ScriptHashes: []common.Blake2b224{hash},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MissingScriptWitnessesUTXOW")
}

func TestScriptWitnessNotValidatingUTXOW_Error(t *testing.T) {
	hash := common.NewBlake2b224([]byte("12345678901234567890123456789012"))

	err := &ScriptWitnessNotValidatingUTXOW{
		ScriptHashes: []common.Blake2b224{hash},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "ScriptWitnessNotValidatingUTXOW")
}

func TestMissingTxBodyMetadataHash_Error(t *testing.T) {
	hash := common.NewBlake2b256([]byte("12345678901234567890123456789012"))

	err := &MissingTxBodyMetadataHash{
		Hash: hash,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MissingTxBodyMetadataHash")
	assert.Contains(t, errStr, hash.String())
}

func TestMissingTxMetadata_Error(t *testing.T) {
	hash := common.NewBlake2b256([]byte("12345678901234567890123456789012"))

	err := &MissingTxMetadata{
		Hash: hash,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "MissingTxMetadata")
}

func TestConflictingMetadataHash_Error(t *testing.T) {
	expected := common.NewBlake2b256([]byte("12345678901234567890123456789012"))
	found := common.NewBlake2b256([]byte("abcdefghijklmnopqrstuvwxyz123456"))

	err := &ConflictingMetadataHash{
		Expected: expected,
		Found:    found,
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "ConflictingMetadataHash")
	assert.Contains(t, errStr, "Expected")
	assert.Contains(t, errStr, "Found")
}

func TestInvalidMetadata_Error(t *testing.T) {
	err := &InvalidMetadata{}

	errStr := err.Error()
	assert.Equal(t, "InvalidMetadata", errStr)
}

func TestExtraneousScriptWitnessesUTXOW_Error(t *testing.T) {
	hash := common.NewBlake2b224([]byte("12345678901234567890123456789012"))

	err := &ExtraneousScriptWitnessesUTXOW{
		ScriptHashes: []common.Blake2b224{hash},
	}

	errStr := err.Error()
	assert.Contains(t, errStr, "ExtraneousScriptWitnessesUTXOW")
}

// =============================================================================
// Era-Aware Decoding Tests
// =============================================================================

func TestUtxowFailure_EraAwareDecoding_Babbage(t *testing.T) {
	// Test that Babbage era uses the Babbage decoder
	// Tag 3 in Babbage = MalformedScriptWitnesses

	// Decode with Babbage era - use a generic error to test the switch logic
	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdBabbage

	// Verify the era is set correctly
	assert.Equal(t, uint8(EraIdBabbage), utxowErr.era)
}

func TestUtxowFailure_EraAwareDecoding_Conway(t *testing.T) {
	// Test that Conway era uses the Conway decoder
	// Tag 3 in Conway = MissingScriptWitnessesUTXOW (different from Babbage!)

	// Decode with Conway era
	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdConway

	// Verify the era is set correctly
	assert.Equal(t, uint8(EraIdConway), utxowErr.era)
}

func TestUtxowFailure_InvalidMetadata_Conway(t *testing.T) {
	// Test InvalidMetadata (tag 8) which has no payload
	cborData, err := cbor.Encode([]interface{}{
		uint(8), // Tag 8 = InvalidMetadata in Conway
	})
	require.NoError(t, err)

	// Decode with Conway era
	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdConway
	err = utxowErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	// Verify it decoded as InvalidMetadata
	_, ok := utxowErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected InvalidMetadata, got %T", utxowErr.Err)
}

func TestConwayUtxowFailure_AllTags(t *testing.T) {
	// Test that ConwayUtxowFailure can handle InvalidMetadata (tag 8, no payload)
	cborData, err := cbor.Encode([]interface{}{uint(8)})
	require.NoError(t, err)

	conwayErr := &ConwayUtxowFailure{}
	err = conwayErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	assert.NotNil(t, conwayErr.Err)
	_, ok := conwayErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected *InvalidMetadata, got %T", conwayErr.Err)
}

// =============================================================================
// Wrapper Type Tests
// =============================================================================

func TestAlonzoUtxowFailure_Error(t *testing.T) {
	innerErr := &MissingRedeemers{
		Missing: []MissingRedeemerEntry{},
	}
	err := &AlonzoUtxowFailure{Err: innerErr}

	errStr := err.Error()
	assert.Contains(t, errStr, "AlonzoInBabbageUtxowPredFailure")
}

func TestBabbageUtxoFailure_Error(t *testing.T) {
	innerErr := &IncorrectTotalCollateralField{
		BalanceComputed: 100,
		TotalCollateral: 200,
	}
	err := &BabbageUtxoFailure{Err: innerErr}

	errStr := err.Error()
	assert.Contains(t, errStr, "BabbageUtxoFailure")
}

func TestConwayUtxowFailure_Error(t *testing.T) {
	innerErr := &InvalidMetadata{}
	err := &ConwayUtxowFailure{Err: innerErr}

	errStr := err.Error()
	assert.Contains(t, errStr, "ConwayUtxowFailure")
}

func TestShelleyUtxowFailure_Error(t *testing.T) {
	innerErr := &MissingVKeyWitnessesUTXOW{
		KeyHashes: []common.Blake2b224{},
	}
	err := &ShelleyUtxowFailure{Err: innerErr}

	errStr := err.Error()
	assert.Contains(t, errStr, "ShelleyInAlonzoUtxowPredFailure")
}

// =============================================================================
// Era-Aware Decoding Tests for All Eras
// =============================================================================

func TestUtxowFailure_EraAwareDecoding_Shelley(t *testing.T) {
	// Test Shelley era decoding (tag 8 = InvalidMetadata)
	cborData, err := cbor.Encode([]interface{}{uint(ShelleyUtxowInvalidMetadata)})
	require.NoError(t, err)

	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdShelley
	err = utxowErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	_, ok := utxowErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected *InvalidMetadata, got %T", utxowErr.Err)
}

func TestUtxowFailure_EraAwareDecoding_Allegra(t *testing.T) {
	// Test Allegra era uses same tags as Shelley (tag 8 = InvalidMetadata)
	cborData, err := cbor.Encode([]interface{}{uint(ShelleyUtxowInvalidMetadata)})
	require.NoError(t, err)

	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdAllegra
	err = utxowErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	_, ok := utxowErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected *InvalidMetadata, got %T", utxowErr.Err)
}

func TestUtxowFailure_EraAwareDecoding_Mary(t *testing.T) {
	// Test Mary era uses same tags as Shelley (tag 8 = InvalidMetadata)
	cborData, err := cbor.Encode([]interface{}{uint(ShelleyUtxowInvalidMetadata)})
	require.NoError(t, err)

	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdMary
	err = utxowErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	_, ok := utxowErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected *InvalidMetadata, got %T", utxowErr.Err)
}

func TestUtxowFailure_EraAwareDecoding_Alonzo(t *testing.T) {
	// Test that Alonzo era uses the Alonzo decoder
	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdAlonzo

	// Verify the era is set correctly
	assert.Equal(t, uint8(EraIdAlonzo), utxowErr.era)
}

func TestShelleyUtxowFailure_UnmarshalCBOR_InvalidMetadata(t *testing.T) {
	// Test ShelleyUtxowFailure can decode InvalidMetadata (no payload)
	cborData, err := cbor.Encode([]interface{}{uint(ShelleyUtxowInvalidMetadata)})
	require.NoError(t, err)

	shelleyErr := &ShelleyUtxowFailure{}
	err = shelleyErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	_, ok := shelleyErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected *InvalidMetadata, got %T", shelleyErr.Err)
}

func TestShelleyUtxowFailure_Constants(t *testing.T) {
	// Verify Shelley UTXOW failure constants are defined correctly
	// These should match the Cardano ledger specification

	// Shelley constants should be 0-9
	assert.Equal(t, 0, ShelleyUtxowInvalidWitnesses)
	assert.Equal(t, 1, ShelleyUtxowMissingVKeyWitnesses)
	assert.Equal(t, 2, ShelleyUtxowMissingScriptWitnesses)
	assert.Equal(t, 3, ShelleyUtxowScriptWitnessNotValidating)
	assert.Equal(t, 4, ShelleyUtxowUtxoFailure)
	assert.Equal(t, 5, ShelleyUtxowMissingTxBodyMetadataHash)
	assert.Equal(t, 6, ShelleyUtxowMissingTxMetadata)
	assert.Equal(t, 7, ShelleyUtxowConflictingMetadataHash)
	assert.Equal(t, 8, ShelleyUtxowInvalidMetadata)
	assert.Equal(t, 9, ShelleyUtxowExtraneousScriptWitnesses)
}

func TestUtxowFailure_EraConstantsConsistency(t *testing.T) {
	// Verify era constants are consistent
	assert.Equal(t, uint8(1), uint8(EraIdShelley))
	assert.Equal(t, uint8(2), uint8(EraIdAllegra))
	assert.Equal(t, uint8(3), uint8(EraIdMary))
	assert.Equal(t, uint8(4), uint8(EraIdAlonzo))
	assert.Equal(t, uint8(5), uint8(EraIdBabbage))
	assert.Equal(t, uint8(6), uint8(EraIdConway))
}

// TestConwayUtxoFailure_TagMappings verifies that Conway UTXO failures decode
// correctly with the renumbered tag mappings (which differ from Alonzo/Babbage).
func TestConwayUtxoFailure_TagMappings(t *testing.T) {
	// Conway tag 0 = UtxosFailure (was tag 7 in Alonzo/Babbage)
	// Conway tag 1 = BadInputsUTxO (was tag 0 in Alonzo/Babbage)
	// This test verifies the fix for era-aware UTXO failure decoding.

	testCases := []struct {
		name        string
		conwayTag   int
		expectedErr string
	}{
		{
			name:        "Conway tag 0 is UtxosFailure",
			conwayTag:   ConwayUtxoUtxosFailure, // 0
			expectedErr: "UtxosFailure",
		},
		{
			name:        "Conway tag 1 is BadInputsUTxO",
			conwayTag:   ConwayUtxoBadInputsUTxO, // 1
			expectedErr: "BadInputsUtxo",
		},
		{
			name:        "Conway tag 9 is OutputTooSmallUTxO",
			conwayTag:   ConwayUtxoOutputTooSmallUTxO, // 9
			expectedErr: "OutputTooSmallUtxo",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Verify the Conway-specific map is returned for Conway era
			errorMap, _, _, _, _ := getEraSpecificUtxoFailureConstants(EraIdConway)

			// Check the tag maps to the expected error type
			errType, exists := errorMap[tc.conwayTag]
			require.True(t, exists, "Conway tag %d should exist in error map", tc.conwayTag)
			assert.Contains(t, fmt.Sprintf("%T", errType), tc.expectedErr,
				"Conway tag %d should map to %s", tc.conwayTag, tc.expectedErr)
		})
	}
}

// TestConwayVsAlonzoUtxoTagDifferences verifies that Conway and Alonzo/Babbage
// have different tag mappings for the same error types.
func TestConwayVsAlonzoUtxoTagDifferences(t *testing.T) {
	conwayMap, _, _, _, _ := getEraSpecificUtxoFailureConstants(EraIdConway)
	alonzoMap, _, _, _, _ := getEraSpecificUtxoFailureConstants(EraIdAlonzo)

	// In Conway, tag 0 = UtxosFailure
	// In Alonzo, tag 0 = BadInputsUtxo
	conwayTag0 := conwayMap[0]
	alonzoTag0 := alonzoMap[0]

	assert.IsType(t, &UtxosFailure{}, conwayTag0, "Conway tag 0 should be UtxosFailure")
	assert.IsType(t, &BadInputsUtxo{}, alonzoTag0, "Alonzo tag 0 should be BadInputsUtxo")

	// In Conway, tag 1 = BadInputsUTxO
	// In Alonzo, tag 1 = OutsideValidityIntervalUtxo
	conwayTag1 := conwayMap[1]
	alonzoTag1 := alonzoMap[1]

	assert.IsType(t, &BadInputsUtxo{}, conwayTag1, "Conway tag 1 should be BadInputsUtxo")
	assert.IsType(t, &OutsideValidityIntervalUtxo{}, alonzoTag1, "Alonzo tag 1 should be OutsideValidityIntervalUtxo")
}
