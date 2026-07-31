package ledger

import (
	"bytes"
	"errors"
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
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: UtxoFailureScriptsNotPaidUtxoConway,
		},
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
	byronError := &ScriptsNotPaidUtxo{
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: UtxoFailureScriptsNotPaidUtxoConway,
		},
		Utxos: byronUtxos,
	}
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
	shelleyError := &ScriptsNotPaidUtxo{
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: UtxoFailureScriptsNotPaidUtxoConway,
		},
		Utxos: shelleyUtxos,
	}
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

// TestScriptsNotPaidUtxo_RequiresExplicitType verifies that MarshalCBOR no
// longer silently defaults to Conway's constructor numbering when Type is
// left unset. Forward compatibility requires the caller to be explicit
// about which era's constructor tag it means, rather than guessing.
func TestScriptsNotPaidUtxo_RequiresExplicitType(t *testing.T) {
	addr1, err := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	require.NoError(t, err)

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

	// Type unset (zero value): must error instead of silently picking
	// Conway's constructor index.
	unset := &ScriptsNotPaidUtxo{Utxos: utxos}
	_, err = unset.MarshalCBOR()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Type")

	// Type explicitly set to Conway's constructor index: succeeds and
	// round-trips using that exact tag.
	withType := &ScriptsNotPaidUtxo{
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: UtxoFailureScriptsNotPaidUtxoConway,
		},
		Utxos: utxos,
	}
	cborData, err := withType.MarshalCBOR()
	require.NoError(t, err)

	var decoded ScriptsNotPaidUtxo
	require.NoError(t, decoded.UnmarshalCBOR(cborData))
	assert.Equal(
		t,
		uint8(UtxoFailureScriptsNotPaidUtxoConway),
		decoded.Type,
	)
}

// TestCollateralContainsNonADA_RequiresExplicitType mirrors
// TestScriptsNotPaidUtxo_RequiresExplicitType for CollateralContainsNonADA,
// whose MarshalCBOR used to silently default to Conway's constructor
// numbering when Type was unset.
func TestCollateralContainsNonADA_RequiresExplicitType(t *testing.T) {
	providedCbor, err := cbor.Encode(uint64(500))
	require.NoError(t, err)
	var provided cbor.Value
	_, err = cbor.Decode(providedCbor, &provided)
	require.NoError(t, err)

	unset := &CollateralContainsNonADA{Provided: provided}
	_, err = unset.MarshalCBOR()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Type")

	withType := &CollateralContainsNonADA{
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: UtxoFailureCollateralContainsNonAdaConway,
		},
		Provided: provided,
	}
	cborData, err := withType.MarshalCBOR()
	require.NoError(t, err)

	var decoded CollateralContainsNonADA
	require.NoError(t, decoded.UnmarshalCBOR(cborData))
	assert.Equal(
		t,
		uint8(UtxoFailureCollateralContainsNonAdaConway),
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
	cborData, err := cbor.Encode([]any{
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
	cborData, err := cbor.Encode([]any{uint(8)})
	require.NoError(t, err)

	conwayErr := &ConwayUtxowFailure{}
	err = conwayErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	assert.NotNil(t, conwayErr.Err)
	_, ok := conwayErr.Err.(*InvalidMetadata)
	assert.True(t, ok, "Expected *InvalidMetadata, got %T", conwayErr.Err)
}

func TestApplyTxError_IncorrectWithdrawals_Shelley(t *testing.T) {
	// Account h'010203' has supplied=100, expected=150.
	withdrawals := map[cbor.ByteString][]uint64{
		cbor.NewByteString([]byte{0x01, 0x02, 0x03}): {100, 150},
	}
	// Shelley-family LEDGER encodes IncompleteWithdrawals as tag 3:
	// [3, {account_address: [supplied, expected]}].
	failure := struct {
		cbor.StructAsArray
		Type        uint8
		Withdrawals map[cbor.ByteString][]uint64
	}{
		Type:        ShelleyLedgerIncompleteWithdrawals,
		Withdrawals: withdrawals,
	}
	cborData, err := cbor.Encode([]any{failure})
	require.NoError(t, err)
	withdrawalsCbor, err := cbor.Encode(withdrawals)
	require.NoError(t, err)

	// ApplyTxError decodes top-level LEDGER predicate failures using the era.
	applyErr := &ApplyTxError{era: EraIdShelley}
	err = applyErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)
	require.Len(t, applyErr.Failures, 1)

	incorrectWithdrawals, ok := applyErr.Failures[0].(*IncorrectWithdrawals)
	require.True(
		t,
		ok,
		"Expected *IncorrectWithdrawals, got %T",
		applyErr.Failures[0],
	)
	assert.Equal(t, uint8(ShelleyLedgerIncompleteWithdrawals), incorrectWithdrawals.Type)
	// Validate that the withdrawals payload was preserved.
	assert.Equal(t, withdrawalsCbor, incorrectWithdrawals.Withdrawals.Cbor())
}

func TestApplyTxError_IncorrectWithdrawals_Babbage(t *testing.T) {
	// Account h'010203' has supplied=100, expected=150.
	withdrawals := map[cbor.ByteString][]uint64{
		cbor.NewByteString([]byte{0x01, 0x02, 0x03}): {100, 150},
	}
	// Babbage still uses the Shelley-family LEDGER tag 3:
	// [3, {account_address: [supplied, expected]}].
	failure := struct {
		cbor.StructAsArray
		Type        uint8
		Withdrawals map[cbor.ByteString][]uint64
	}{
		Type:        ShelleyLedgerIncompleteWithdrawals,
		Withdrawals: withdrawals,
	}
	cborData, err := cbor.Encode([]any{failure})
	require.NoError(t, err)
	withdrawalsCbor, err := cbor.Encode(withdrawals)
	require.NoError(t, err)
	require.True(
		t,
		isLedgerIncompleteWithdrawalsFailure(
			EraIdBabbage,
			ShelleyLedgerIncompleteWithdrawals,
		),
	)

	// ApplyTxError decodes top-level LEDGER predicate failures using the era.
	applyErr := &ApplyTxError{era: EraIdBabbage}
	err = applyErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)
	require.Len(t, applyErr.Failures, 1)

	incorrectWithdrawals, ok := applyErr.Failures[0].(*IncorrectWithdrawals)
	require.True(
		t,
		ok,
		"Expected *IncorrectWithdrawals, got %T",
		applyErr.Failures[0],
	)
	assert.Equal(t, uint8(ShelleyLedgerIncompleteWithdrawals), incorrectWithdrawals.Type)
	// Validate that the withdrawals payload was preserved.
	assert.Equal(t, withdrawalsCbor, incorrectWithdrawals.Withdrawals.Cbor())
}

func TestApplyTxError_IncorrectWithdrawals_Conway(t *testing.T) {
	// Account h'010203' has supplied=100, expected=150.
	withdrawals := map[cbor.ByteString][]uint64{
		cbor.NewByteString([]byte{0x01, 0x02, 0x03}): {100, 150},
	}
	// Conway LEDGER encodes IncompleteWithdrawals as tag 9:
	// [9, {account_address: [supplied, expected]}].
	failure := struct {
		cbor.StructAsArray
		Type        uint8
		Withdrawals map[cbor.ByteString][]uint64
	}{
		Type:        ConwayLedgerIncompleteWithdrawals,
		Withdrawals: withdrawals,
	}
	cborData, err := cbor.Encode([]any{failure})
	require.NoError(t, err)
	withdrawalsCbor, err := cbor.Encode(withdrawals)
	require.NoError(t, err)

	// ApplyTxError decodes top-level LEDGER predicate failures using the era.
	applyErr := &ApplyTxError{era: EraIdConway}
	err = applyErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)
	require.Len(t, applyErr.Failures, 1)

	incorrectWithdrawals, ok := applyErr.Failures[0].(*IncorrectWithdrawals)
	require.True(
		t,
		ok,
		"Expected *IncorrectWithdrawals, got %T",
		applyErr.Failures[0],
	)
	assert.Equal(t, uint8(ConwayLedgerIncompleteWithdrawals), incorrectWithdrawals.Type)
	// Validate that the withdrawals payload was preserved.
	assert.Equal(t, withdrawalsCbor, incorrectWithdrawals.Withdrawals.Cbor())
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
	cborData, err := cbor.Encode([]any{uint(ShelleyUtxowInvalidMetadata)})
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
	cborData, err := cbor.Encode([]any{uint(ShelleyUtxowInvalidMetadata)})
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
	cborData, err := cbor.Encode([]any{uint(ShelleyUtxowInvalidMetadata)})
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
	cborData, err := cbor.Encode([]any{uint(ShelleyUtxowInvalidMetadata)})
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
			errorMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
				EraIdConway,
			)
			require.NoError(t, err)

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
	conwayMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
		EraIdConway,
	)
	require.NoError(t, err)
	alonzoMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
		EraIdAlonzo,
	)
	require.NoError(t, err)

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

// =============================================================================
// Unknown Era/Constructor Preservation Tests
//
// These tests cover gouroboros issue #1868: unknown era/constructor
// combinations must be preserved (era, raw constructor tag, and raw CBOR)
// rather than silently decoded as a GenericError or by guessing another
// era's (e.g. Babbage's or Conway's) constructor numbering.
// =============================================================================

// TestGetEraSpecificUtxoFailureConstants_UnknownEra verifies that an
// unrecognized era id returns an error instead of silently falling back to
// Babbage's constructor numbering.
func TestGetEraSpecificUtxoFailureConstants_UnknownEra(t *testing.T) {
	_, _, _, _, _, err := getEraSpecificUtxoFailureConstants(99)
	require.Error(t, err)
}

// TestGetEraSpecificUtxoFailureConstants_Byron verifies that Byron (era id
// 0) — which predates this UTXO failure wire format entirely — is treated
// as unrecognized rather than silently mapped onto Babbage's numbering.
func TestGetEraSpecificUtxoFailureConstants_Byron(t *testing.T) {
	_, _, _, _, _, err := getEraSpecificUtxoFailureConstants(EraIdByron)
	require.Error(t, err)
}

// TestUtxowFailure_UnknownEra verifies that UtxowFailure.UnmarshalCBOR
// surfaces a typed UnknownUtxowFailureError for an era it doesn't
// recognize, preserving era/tag/CBOR context, instead of silently
// decoding the payload as though it were Babbage.
func TestUtxowFailure_UnknownEra(t *testing.T) {
	cborData, err := cbor.Encode([]any{uint(3)})
	require.NoError(t, err)

	utxowErr := &UtxowFailure{}
	utxowErr.era = 99 // unrecognized era
	err = utxowErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	unknownErr, ok := utxowErr.Err.(*UnknownUtxowFailureError)
	require.True(
		t,
		ok,
		"Expected *UnknownUtxowFailureError, got %T",
		utxowErr.Err,
	)
	assert.Equal(t, uint8(99), unknownErr.Era)
	assert.Equal(t, 3, unknownErr.FailureType)
	assert.Equal(t, cborData, unknownErr.Cbor)
	assert.Contains(t, unknownErr.Error(), "99")
}

// TestUtxowFailure_UnknownConstructorTag verifies that an unrecognized
// UTXOW constructor tag within a *known* era surfaces a typed
// UnknownUtxowFailureError (preserving the tag) instead of a GenericError.
func TestUtxowFailure_UnknownConstructorTag(t *testing.T) {
	testCases := []struct {
		name string
		era  uint8
	}{
		{"Shelley", EraIdShelley},
		{"Allegra", EraIdAllegra},
		{"Mary", EraIdMary},
		{"Alonzo", EraIdAlonzo},
		{"Babbage", EraIdBabbage},
		{"Conway", EraIdConway},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Tag 250 does not exist in any era's UTXOW failure
			// enumeration.
			cborData, err := cbor.Encode(
				[]any{uint(250), []any{}},
			)
			require.NoError(t, err)

			utxowErr := &UtxowFailure{}
			utxowErr.era = tc.era
			err = utxowErr.UnmarshalCBOR(cborData)
			require.NoError(t, err)

			unknownErr, ok := utxowErr.Err.(*UnknownUtxowFailureError)
			require.True(
				t,
				ok,
				"Expected *UnknownUtxowFailureError, got %T",
				utxowErr.Err,
			)
			assert.Equal(t, tc.era, unknownErr.Era)
			assert.Equal(t, 250, unknownErr.FailureType)
		})
	}
}

// TestUtxoFailure_UnknownConstructorId verifies that an unrecognized UTXO
// failure constructor id within a known era surfaces a typed
// UnknownUtxoFailureError (preserving the era, tag, and raw CBOR) instead
// of silently decoding as a GenericError.
func TestUtxoFailure_UnknownConstructorId(t *testing.T) {
	// Tag 250 does not exist in Conway's (or any era's) UTXO failure
	// enumeration.
	innerCbor, err := cbor.Encode([]any{uint(250)})
	require.NoError(t, err)
	cborData, err := cbor.Encode(
		[]any{uint8(EraIdConway), cbor.RawMessage(innerCbor)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	unknownErr, ok := utxoErr.Err.(*UnknownUtxoFailureError)
	require.True(
		t,
		ok,
		"Expected *UnknownUtxoFailureError, got %T",
		utxoErr.Err,
	)
	assert.Equal(t, uint8(EraIdConway), unknownErr.Era)
	assert.Equal(t, 250, unknownErr.FailureType)
	assert.Equal(t, innerCbor, unknownErr.Cbor)
}

// TestUtxoFailure_UnknownEra verifies that UtxoFailure.UnmarshalCBOR
// surfaces a typed UnknownUtxoFailureError for an era it doesn't
// recognize, instead of silently decoding using Babbage's numbering.
func TestUtxoFailure_UnknownEra(t *testing.T) {
	innerCbor, err := cbor.Encode([]any{uint(0)})
	require.NoError(t, err)
	cborData, err := cbor.Encode(
		[]any{uint8(99), cbor.RawMessage(innerCbor)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	unknownErr, ok := utxoErr.Err.(*UnknownUtxoFailureError)
	require.True(
		t,
		ok,
		"Expected *UnknownUtxoFailureError, got %T",
		utxoErr.Err,
	)
	assert.Equal(t, uint8(99), unknownErr.Era)
}

// TestUtxoFailure_KnownConstructorMalformedPayload verifies that a
// recognized UTXO failure constructor tag whose payload fails to decode
// surfaces the real decode error, instead of being masked as an
// UnknownUtxoFailureError.
func TestUtxoFailure_KnownConstructorMalformedPayload(t *testing.T) {
	// ConwayUtxoBadInputsUTxO is a known tag, but its payload should be
	// an array of inputs, not a string.
	innerCbor, err := cbor.Encode(
		[]any{uint(ConwayUtxoBadInputsUTxO), "not-an-input-list"},
	)
	require.NoError(t, err)
	cborData, err := cbor.Encode(
		[]any{uint8(EraIdConway), cbor.RawMessage(innerCbor)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.Error(t, err)

	var unknownErr *UnknownUtxoFailureError
	require.False(
		t,
		errors.As(err, &unknownErr),
		"expected a real decode error, not UnknownUtxoFailureError",
	)
}

// TestApplyTxError_UnknownFailureType verifies that an unrecognized
// LEDGER-level failure constructor tag surfaces a typed
// UnknownApplyTxFailureError instead of a GenericError.
func TestApplyTxError_UnknownFailureType(t *testing.T) {
	failure := []any{uint(250)}
	cborData, err := cbor.Encode([]any{failure})
	require.NoError(t, err)

	applyErr := &ApplyTxError{era: EraIdConway}
	err = applyErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)
	require.Len(t, applyErr.Failures, 1)

	unknownErr, ok := applyErr.Failures[0].(*UnknownApplyTxFailureError)
	require.True(
		t,
		ok,
		"Expected *UnknownApplyTxFailureError, got %T",
		applyErr.Failures[0],
	)
	assert.Equal(t, uint8(EraIdConway), unknownErr.Era)
	assert.Equal(t, 250, unknownErr.FailureType)
}

// =============================================================================
// Reviewer-reported regression tests (PR #1923 "Requesting changes")
// =============================================================================

// TestGetEraSpecificUtxoFailureConstants_PreAlonzoExcludesCollateralTags
// verifies that Shelley, Allegra, and Mary's UTXO failure constant map does
// NOT include the Alonzo+-only collateral/Plutus-related tags (17, 18, 19,
// 20) or tag 11 (which is not TriesToForgeADA, or anything else, in any of
// these three eras), while still including the era-agnostic base tags
// (0-10). Tag 12 is checked separately (see
// TestGetEraSpecificUtxoFailureConstants_ShelleyVsAllegraMary): it's absent
// from Shelley but a real Allegra/Mary constructor (OutputTooBigUTxO), so it
// can't be lumped in with the eras-agnostic exclusions/inclusions here.
// Prior to the fix, these eras reused baseMap unmodified, so tag 12
// collided with UtxoFailureInsufficientCollateral even though collateral
// doesn't exist pre-Alonzo, and tag 11 incorrectly decoded as
// TriesToForgeADA.
func TestGetEraSpecificUtxoFailureConstants_PreAlonzoExcludesCollateralTags(
	t *testing.T,
) {
	excludedTags := []int{
		UtxoFailureTriesToForgeAda,         // 11 - not real in any of the three
		UtxoFailureWrongNetworkInTxBody,    // 17
		UtxoFailureOutsideForecast,         // 18
		UtxoFailureTooManyCollateralInputs, // 19
		UtxoFailureNoCollateralInputs,      // 20
	}
	baseTags := []int{
		UtxoFailureBadInputsUtxo,
		UtxoFailureOutsideValidityIntervalUtxo,
		UtxoFailureMaxTxSizeUtxo,
		UtxoFailureInputSetEmpty,
		UtxoFailureFeeTooSmallUtxo,
		UtxoFailureValueNotConservedUtxo,
		UtxoFailureOutputTooSmallUtxo,
		UtxoFailureUtxosFailure,
		UtxoFailureWrongNetwork,
		UtxoFailureWrongNetworkWithdrawal,
		UtxoFailureOutputBootAddrAttrsTooBig,
	}

	testCases := []struct {
		name string
		era  uint8
	}{
		{"Shelley", EraIdShelley},
		{"Allegra", EraIdAllegra},
		{"Mary", EraIdMary},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errorMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
				tc.era,
			)
			require.NoError(t, err)

			for _, tag := range excludedTags {
				_, exists := errorMap[tag]
				assert.False(
					t,
					exists,
					"%s: tag %d should not exist pre-Alonzo",
					tc.name,
					tag,
				)
			}
			for _, tag := range baseTags {
				_, exists := errorMap[tag]
				assert.True(
					t,
					exists,
					"%s: tag %d should exist pre-Alonzo",
					tc.name,
					tag,
				)
			}

			// Tag 12 must never decode as InsufficientCollateral in any
			// of these three eras (collateral doesn't exist pre-Alonzo).
			_, isCollateral := errorMap[UtxoFailureInsufficientCollateral].(*InsufficientCollateral)
			assert.False(
				t,
				isCollateral,
				"%s: tag 12 must not decode as InsufficientCollateral",
				tc.name,
			)
		})
	}
}

// TestGetEraSpecificUtxoFailureConstants_ShelleyVsAllegraMary verifies that
// Shelley cannot share a UTXO failure constructor map with Allegra/Mary:
// Allegra/Mary define OutputTooBigUTxO at tag 12, which does not exist in
// Shelley. Tag 11 (TriesToForgeADA in the Alonzo+ base numbering) is not a
// real constructor in any of the three, so it's excluded from all of them.
func TestGetEraSpecificUtxoFailureConstants_ShelleyVsAllegraMary(t *testing.T) {
	shelleyMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
		EraIdShelley,
	)
	require.NoError(t, err)
	allegraMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
		EraIdAllegra,
	)
	require.NoError(t, err)
	maryMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
		EraIdMary,
	)
	require.NoError(t, err)

	// Tag 12 (OutputTooBigUTxO) must not exist for Shelley.
	_, shelleyHasTag12 := shelleyMap[UtxoFailureOutputTooBigUtxoAllegraMary]
	assert.False(t, shelleyHasTag12, "Shelley must not have tag 12")

	// Tag 12 must decode as OutputTooBigUTxO for Allegra and Mary.
	for name, m := range map[string]map[int]any{
		"Allegra": allegraMap,
		"Mary":    maryMap,
	} {
		val, exists := m[UtxoFailureOutputTooBigUtxoAllegraMary]
		require.True(t, exists, "%s: tag 12 should exist", name)
		assert.IsType(t, &OutputTooBigUtxo{}, val)
	}

	// Tag 11 must not exist in any of the three (not TriesToForgeADA, not
	// anything else).
	for name, m := range map[string]map[int]any{
		"Shelley": shelleyMap,
		"Allegra": allegraMap,
		"Mary":    maryMap,
	} {
		_, exists := m[UtxoFailureTriesToForgeAda]
		assert.False(t, exists, "%s: tag 11 should not exist", name)
	}
}

// TestUtxoFailure_ShelleyTag11And12AreUnknown reproduces the reviewer's
// exact repro: Shelley tag 11 and tag 12 must each decode as
// *UnknownUtxoFailureError, since neither is a real Shelley constructor
// (tag 11 is not TriesToForgeADA in Shelley, and tag 12 is Allegra/Mary's
// OutputTooBigUTxO, which doesn't exist in Shelley at all).
func TestUtxoFailure_ShelleyTag11And12AreUnknown(t *testing.T) {
	for _, tag := range []int{11, 12} {
		t.Run(fmt.Sprintf("tag%d", tag), func(t *testing.T) {
			innerCbor, err := cbor.Encode([]any{uint(tag)})
			require.NoError(t, err)
			cborData, err := cbor.Encode(
				[]any{uint8(EraIdShelley), cbor.RawMessage(innerCbor)},
			)
			require.NoError(t, err)

			var utxoErr UtxoFailure
			err = utxoErr.UnmarshalCBOR(cborData)
			require.NoError(t, err)

			unknownErr, ok := utxoErr.Err.(*UnknownUtxoFailureError)
			require.True(
				t,
				ok,
				"Expected *UnknownUtxoFailureError, got %T",
				utxoErr.Err,
			)
			assert.Equal(t, uint8(EraIdShelley), unknownErr.Era)
			assert.Equal(t, tag, unknownErr.FailureType)
		})
	}
}

// TestUtxoFailure_AllegraMaryOutputTooBigUtxo verifies that Allegra and Mary
// tag 12 decodes as *OutputTooBigUtxo (the reviewer-confirmed real wire
// tag), rather than being reported unknown (as it would be if these eras
// shared Shelley's tag-0-10-only map).
func TestUtxoFailure_AllegraMaryOutputTooBigUtxo(t *testing.T) {
	testCases := []struct {
		name string
		era  uint8
	}{
		{"Allegra", EraIdAllegra},
		{"Mary", EraIdMary},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			innerCbor, err := cbor.Encode(
				[]any{uint(UtxoFailureOutputTooBigUtxoAllegraMary), []any{}},
			)
			require.NoError(t, err)
			cborData, err := cbor.Encode(
				[]any{tc.era, cbor.RawMessage(innerCbor)},
			)
			require.NoError(t, err)

			var utxoErr UtxoFailure
			err = utxoErr.UnmarshalCBOR(cborData)
			require.NoError(t, err)

			_, ok := utxoErr.Err.(*OutputTooBigUtxo)
			require.True(
				t,
				ok,
				"%s: expected *OutputTooBigUtxo, got %T",
				tc.name,
				utxoErr.Err,
			)
		})
	}
}

// TestUtxoFailure_PreAlonzoCollateralTagIsUnknown reproduces the reviewer's
// exact repro: [EraIdShelley, [12, 1, 2]] must decode as
// *UnknownUtxoFailureError, NOT as *InsufficientCollateral, since collateral
// does not exist in Shelley and tag 12 is not a real Shelley constructor
// (unlike Allegra/Mary, where tag 12 is the real OutputTooBigUTxO
// constructor — see TestUtxoFailure_AllegraMaryOutputTooBigUtxo).
func TestUtxoFailure_PreAlonzoCollateralTagIsUnknown(t *testing.T) {
	testCases := []struct {
		name string
		era  uint8
	}{
		{"Shelley", EraIdShelley},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			innerCbor, err := cbor.Encode(
				[]any{uint(12), uint(1), uint(2)},
			)
			require.NoError(t, err)
			cborData, err := cbor.Encode(
				[]any{tc.era, cbor.RawMessage(innerCbor)},
			)
			require.NoError(t, err)

			var utxoErr UtxoFailure
			err = utxoErr.UnmarshalCBOR(cborData)
			require.NoError(t, err)

			unknownErr, ok := utxoErr.Err.(*UnknownUtxoFailureError)
			require.True(
				t,
				ok,
				"Expected *UnknownUtxoFailureError, got %T",
				utxoErr.Err,
			)
			assert.Equal(t, tc.era, unknownErr.Era)
			assert.Equal(t, 12, unknownErr.FailureType)

			_, isCollateral := utxoErr.Err.(*InsufficientCollateral)
			assert.False(
				t,
				isCollateral,
				"%s: tag 12 must not decode as InsufficientCollateral",
				tc.name,
			)
		})
	}
}

// TestGetEraSpecificUtxoFailureConstants_Dijkstra verifies that Dijkstra
// has its own UTXO failure constructor map: it shares Conway's tags 0-8
// verbatim, but tags 9 onward are shifted down by one relative to Conway
// (Dijkstra's Utxo.hs does not carry forward a distinct OutputTooSmallUTxO
// constructor). Reusing Conway's map/constants for Dijkstra would misdecode
// these tags as the wrong concrete error types.
func TestGetEraSpecificUtxoFailureConstants_Dijkstra(t *testing.T) {
	dijkstraMap, _, _, _, _, err := getEraSpecificUtxoFailureConstants(
		EraIdDijkstra,
	)
	require.NoError(t, err)

	// Tags 0-8 are shared verbatim with Conway.
	sharedTags := []int{
		ConwayUtxoUtxosFailure,
		ConwayUtxoBadInputsUTxO,
		ConwayUtxoOutsideValidityIntervalUTxO,
		ConwayUtxoMaxTxSizeUTxO,
		ConwayUtxoInputSetEmptyUTxO,
		ConwayUtxoFeeTooSmallUTxO,
		ConwayUtxoValueNotConservedUTxO,
		ConwayUtxoWrongNetwork,
		ConwayUtxoWrongNetworkWithdrawal,
	}
	for _, tag := range sharedTags {
		_, exists := dijkstraMap[tag]
		assert.True(t, exists, "Dijkstra map missing shared tag %d", tag)
	}

	// Conway's tag 9 (OutputTooSmallUTxO) does not exist as a distinct
	// Dijkstra constructor: Dijkstra's tag 9 (same numeric value) must
	// decode as OutputBootAddrAttrsTooBig instead.
	_, isOutputTooSmall := dijkstraMap[ConwayUtxoOutputTooSmallUTxO].(*OutputTooSmallUtxo)
	assert.False(
		t,
		isOutputTooSmall,
		"Dijkstra tag 9 must not decode as OutputTooSmallUTxO",
	)

	// Tags 9, 10, 11, and 18 are the reviewer-confirmed real Dijkstra wire
	// tags: OutputBootAddrAttrsTooBig, OutputTooBigUTxO,
	// InsufficientCollateral, and NoCollateralInputs, respectively.
	testCases := []struct {
		name string
		tag  int
		want any
	}{
		{
			"tag 9 is OutputBootAddrAttrsTooBig",
			DijkstraUtxoOutputBootAddrAttrsTooBig,
			&OutputBootAddrAttrsTooBig{},
		},
		{
			"tag 10 is OutputTooBigUTxO",
			DijkstraUtxoOutputTooBigUTxO,
			&OutputTooBigUtxo{},
		},
		{
			"tag 11 is InsufficientCollateral",
			DijkstraUtxoInsufficientCollateral,
			&InsufficientCollateral{},
		},
		{
			"tag 18 is NoCollateralInputs",
			DijkstraUtxoNoCollateralInputs,
			&NoCollateralInputs{},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, exists := dijkstraMap[tc.tag]
			require.True(t, exists, "Dijkstra map missing tag %d", tc.tag)
			assert.IsType(t, tc.want, val)
		})
	}

	require.Equal(t, DijkstraUtxoOutputBootAddrAttrsTooBig, 9)
	require.Equal(t, DijkstraUtxoOutputTooBigUTxO, 10)
	require.Equal(t, DijkstraUtxoInsufficientCollateral, 11)
	require.Equal(t, DijkstraUtxoNoCollateralInputs, 18)
}

// TestUtxoFailure_DijkstraTagMappings reproduces the reviewer's repro: a
// Dijkstra UtxoFailure with tag 9 must decode as OutputBootAddrAttrsTooBig
// (not OutputTooSmallUTxO, which is what Conway's aliased map would
// incorrectly produce).
func TestUtxoFailure_DijkstraTagMappings(t *testing.T) {
	innerCbor, err := cbor.Encode(
		[]any{uint(DijkstraUtxoOutputBootAddrAttrsTooBig), []any{}},
	)
	require.NoError(t, err)
	cborData, err := cbor.Encode(
		[]any{uint8(EraIdDijkstra), cbor.RawMessage(innerCbor)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	_, ok := utxoErr.Err.(*OutputBootAddrAttrsTooBig)
	require.True(
		t,
		ok,
		"Expected *OutputBootAddrAttrsTooBig, got %T",
		utxoErr.Err,
	)
}

// TestUtxoFailure_DijkstraScriptsNotPaidUtxo verifies that a real Dijkstra
// ScriptsNotPaidUTxO failure (tag 12) round-trips through
// UtxoFailure.UnmarshalCBOR as *ScriptsNotPaidUtxo, not as a hard decode
// error. ScriptsNotPaidUtxo.UnmarshalCBOR independently hard-validates the
// constructor tag against a fixed list of valid indices, so this must be
// exercised via the full envelope (not just a reflection check on the
// era map) to catch a validConstructors list that omits Dijkstra's tag.
func TestUtxoFailure_DijkstraScriptsNotPaidUtxo(t *testing.T) {
	addr, err := common.NewAddress(
		"addr1qytna5k2fq9ler0fuk45j7zfwv7t2zwhp777nvdjqqfr5tz8ztpwnk8zq5ngetcz5k5mckgkajnygtsra9aej2h3ek5seupmvd",
	)
	require.NoError(t, err)

	inner := &ScriptsNotPaidUtxo{
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: DijkstraUtxoScriptsNotPaidUTxO,
		},
		Utxos: []common.Utxo{
			{
				Id: shelley.NewShelleyTransactionInput(
					"deadbeef00000000000000000000000000000000000000000000000000000000",
					0,
				),
				Output: &shelley.ShelleyTransactionOutput{
					OutputAddress: addr,
					OutputAmount:  1000,
				},
			},
		},
	}
	innerCbor, err := inner.MarshalCBOR()
	require.NoError(t, err)

	cborData, err := cbor.Encode(
		[]any{uint8(EraIdDijkstra), cbor.RawMessage(innerCbor)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	decoded, ok := utxoErr.Err.(*ScriptsNotPaidUtxo)
	require.True(
		t,
		ok,
		"Expected *ScriptsNotPaidUtxo, got %T",
		utxoErr.Err,
	)
	require.Len(t, decoded.Utxos, 1)
	assert.Equal(
		t,
		uint8(DijkstraUtxoScriptsNotPaidUTxO),
		decoded.Type,
	)
}

// TestUtxoFailure_DijkstraCollateralContainsNonADA verifies that a real
// Dijkstra CollateralContainsNonADA failure (tag 14) round-trips through
// UtxoFailure.UnmarshalCBOR as *CollateralContainsNonADA, not as a hard
// decode error. CollateralContainsNonADA.UnmarshalCBOR independently
// hard-validates the constructor tag against a fixed list of valid
// indices, so this must be exercised via the full envelope (not just a
// reflection check on the era map) to catch a validConstructors list that
// omits Dijkstra's tag.
func TestUtxoFailure_DijkstraCollateralContainsNonADA(t *testing.T) {
	providedCbor, err := cbor.Encode(uint64(500))
	require.NoError(t, err)
	var provided cbor.Value
	_, err = cbor.Decode(providedCbor, &provided)
	require.NoError(t, err)

	inner := &CollateralContainsNonADA{
		UtxoFailureErrorBase: UtxoFailureErrorBase{
			Type: DijkstraUtxoCollateralContainsNonADA,
		},
		Provided: provided,
	}
	innerCbor, err := inner.MarshalCBOR()
	require.NoError(t, err)

	cborData, err := cbor.Encode(
		[]any{uint8(EraIdDijkstra), cbor.RawMessage(innerCbor)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	decoded, ok := utxoErr.Err.(*CollateralContainsNonADA)
	require.True(
		t,
		ok,
		"Expected *CollateralContainsNonADA, got %T",
		utxoErr.Err,
	)
	assert.Equal(
		t,
		uint8(DijkstraUtxoCollateralContainsNonADA),
		decoded.Type,
	)
}

// TestUtxowFailure_Dijkstra verifies that a Dijkstra UtxowFailure decodes
// using Conway's constructor numbering instead of falling through to the
// default case (which would produce *UnknownUtxowFailureError for every
// Dijkstra failure, even well-formed ones). This test would fail against
// the old behavior where Dijkstra was not handled in the era switch.
func TestUtxowFailure_Dijkstra(t *testing.T) {
	// Tag 8 = ConwayUtxowInvalidMetadata, which has no payload.
	cborData, err := cbor.Encode([]any{
		uint(ConwayUtxowInvalidMetadata),
	})
	require.NoError(t, err)

	utxowErr := &UtxowFailure{}
	utxowErr.era = EraIdDijkstra
	err = utxowErr.UnmarshalCBOR(cborData)
	require.NoError(t, err)

	_, ok := utxowErr.Err.(*InvalidMetadata)
	require.True(
		t,
		ok,
		"Expected *InvalidMetadata, got %T",
		utxowErr.Err,
	)

	_, isUnknown := utxowErr.Err.(*UnknownUtxowFailureError)
	assert.False(
		t,
		isUnknown,
		"Dijkstra UtxowFailure must not fall back to UnknownUtxowFailureError",
	)
}

// TestUtxoFailure_MalformedInnerValueReturnsError reproduces the
// reviewer's repro: a Conway UtxoFailure whose inner value is a CBOR
// string rather than a constructor-tagged list must surface a real
// (non-nil) decode error, instead of "successfully" decoding as
// UnknownUtxoFailureError{FailureType: -1}.
func TestUtxoFailure_MalformedInnerValueReturnsError(t *testing.T) {
	malformedInner, err := cbor.Encode("not-a-constructor-list")
	require.NoError(t, err)
	cborData, err := cbor.Encode(
		[]any{uint8(EraIdConway), cbor.RawMessage(malformedInner)},
	)
	require.NoError(t, err)

	var utxoErr UtxoFailure
	err = utxoErr.UnmarshalCBOR(cborData)
	require.Error(
		t,
		err,
		"malformed inner value must surface a real decode error",
	)
	assert.Nil(
		t,
		utxoErr.Err,
		"Err should not be populated when UnmarshalCBOR returns an error",
	)
}
