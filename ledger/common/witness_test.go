package common_test

import (
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

func TestVkeyWitnessRejectsInvalidWidths(t *testing.T) {
	for _, tc := range []struct {
		name      string
		vkey      []byte
		signature []byte
	}{
		{name: "short key", vkey: make([]byte, 31), signature: make([]byte, 64)},
		{name: "short signature", vkey: make([]byte, 32), signature: make([]byte, 63)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := cbor.Encode(common.VkeyWitness{
				Vkey: tc.vkey, Signature: tc.signature,
			})
			require.NoError(t, err)
			var witness common.VkeyWitness
			_, err = cbor.Decode(raw, &witness)
			require.Error(t, err)
		})
	}
}

func TestVkeyWitnessRejectsIndefiniteKey(t *testing.T) {
	key := append([]byte{0x5f, 0x58, 0x20}, make([]byte, 32)...)
	key = append(key, 0xff)
	raw, err := cbor.Encode([]any{
		cbor.RawMessage(key),
		make([]byte, 64),
	})
	require.NoError(t, err)
	var witness common.VkeyWitness
	_, err = cbor.Decode(raw, &witness)
	require.Error(t, err)
}

func TestBootstrapWitnessRejectsInvalidWidths(t *testing.T) {
	for _, tc := range []struct {
		name      string
		publicKey []byte
		signature []byte
	}{
		{name: "short key", publicKey: make([]byte, 31), signature: make([]byte, 64)},
		{name: "short signature", publicKey: make([]byte, 32), signature: make([]byte, 63)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := cbor.Encode(common.BootstrapWitness{
				PublicKey: tc.publicKey,
				Signature: tc.signature,
			})
			require.NoError(t, err)
			var witness common.BootstrapWitness
			_, err = cbor.Decode(raw, &witness)
			require.Error(t, err)
		})
	}
}

func TestBootstrapWitnessRejectsIndefiniteSignature(t *testing.T) {
	signature := append([]byte{0x5f, 0x58, 0x40}, make([]byte, 64)...)
	signature = append(signature, 0xff)
	raw, err := cbor.Encode([]any{
		make([]byte, 32),
		cbor.RawMessage(signature),
		[]byte{},
		[]byte{},
	})
	require.NoError(t, err)
	var witness common.BootstrapWitness
	_, err = cbor.Decode(raw, &witness)
	require.Error(t, err)
}
