package common_test

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/plutigo/builtin"
	"github.com/blinklabs-io/plutigo/cek"
	"github.com/blinklabs-io/plutigo/data"
	"github.com/blinklabs-io/plutigo/lang"
	"github.com/blinklabs-io/plutigo/syn"
	"github.com/stretchr/testify/require"
)

// encodeRedeemerAssertScript builds
//
//	\datum \redeemer \ctx ->
//	    force (ifThenElse (equalsByteString (serialiseData redeemer) want)
//	                      (delay ()) (delay error))
//
// The script fails outright unless serialiseData produced exactly `want`, so
// evaluation succeeds only when the redeemer reached the script in the
// reference implementation's encoding. Unlike a cost-based probe this cannot
// be masked by cost-model quantization.
func encodeRedeemerAssertScript(
	t *testing.T,
	version lang.LanguageVersion,
	want []byte,
) []byte {
	t.Helper()
	// Applied as ((program datum) redeemer) context, so inside the body the
	// De Bruijn indexes are 1=context, 2=redeemer, 3=datum.
	cond := &syn.Apply[syn.DeBruijn]{
		Function: &syn.Apply[syn.DeBruijn]{
			Function: &syn.Builtin{DefaultFunction: builtin.EqualsByteString},
			Argument: &syn.Apply[syn.DeBruijn]{
				Function: &syn.Builtin{DefaultFunction: builtin.SerialiseData},
				Argument: &syn.Var[syn.DeBruijn]{Name: syn.DeBruijn(2)},
			},
		},
		Argument: &syn.Constant{Con: &syn.ByteString{Inner: want}},
	}
	var term syn.Term[syn.DeBruijn] = &syn.Force[syn.DeBruijn]{
		Term: &syn.Apply[syn.DeBruijn]{
			Function: &syn.Apply[syn.DeBruijn]{
				Function: &syn.Apply[syn.DeBruijn]{
					// ifThenElse is polymorphic: it must be forced before it
					// will accept term arguments.
					Function: &syn.Force[syn.DeBruijn]{
						Term: &syn.Builtin{DefaultFunction: builtin.IfThenElse},
					},
					Argument: cond,
				},
				Argument: &syn.Delay[syn.DeBruijn]{
					Term: &syn.Constant{Con: &syn.Unit{}},
				},
			},
			Argument: &syn.Delay[syn.DeBruijn]{Term: &syn.Error{}},
		},
	}
	for range 3 {
		term = &syn.Lambda[syn.DeBruijn]{Body: term}
	}
	flat, err := syn.Encode(&syn.Program[syn.DeBruijn]{Version: version, Term: term})
	require.NoError(t, err)
	wrapper, err := cbor.Encode(flat)
	require.NoError(t, err)
	return wrapper
}

func mustDecodeData(t *testing.T, hexStr string) data.PlutusData {
	t.Helper()
	raw, err := hex.DecodeString(hexStr)
	require.NoError(t, err)
	pd, err := data.Decode(raw)
	require.NoError(t, err)
	return pd
}

// TestEvaluateNormalizesRedeemerEncoding pins the contract that a V1/V2
// script observes the same redeemer bytes no matter which definite- or
// indefinite-length encoding the transaction used on the wire. Decode
// preserves that choice; cardano-ledger does not carry it into script-visible
// data, so without normalization a script hashing serialiseData output
// diverges from the rest of the network.
func TestEvaluateNormalizesRedeemerEncoding(t *testing.T) {
	// The same semantic value, Constr 0 [I 0], in both encodings.
	const definiteHex = "d8798100"     // tag 121, definite 1-element array
	const indefiniteHex = "d8799f00ff" // tag 121, indefinite 1-element array

	definite := mustDecodeData(t, definiteHex)
	indefinite := mustDecodeData(t, indefiniteHex)

	// Precondition: these really are distinct on the wire, otherwise the test
	// proves nothing.
	defEnc, err := data.Encode(definite)
	require.NoError(t, err)
	indefEnc, err := data.Encode(indefinite)
	require.NoError(t, err)
	require.NotEqual(t, hex.EncodeToString(defEnc), hex.EncodeToString(indefEnc),
		"fixture no longer exercises the encoding difference")
	// ...and that they are the same value once normalized.
	normDef, err := data.Encode(data.Normalize(definite))
	require.NoError(t, err)
	normIndef, err := data.Encode(data.Normalize(indefinite))
	require.NoError(t, err)
	require.Equal(t, hex.EncodeToString(normDef), hex.EncodeToString(normIndef))

	datum := mustDecodeData(t, "d87980")
	ctx := mustDecodeData(t, "d87980")

	for _, tc := range []struct {
		name string
		eval func(s []byte, redeemer data.PlutusData) (common.ExUnits, error)
	}{
		{
			name: "PlutusV1",
			eval: func(s []byte, redeemer data.PlutusData) (common.ExUnits, error) {
				return common.PlutusV1Script(s).Evaluate(
					datum, redeemer, ctx, common.ExUnits{},
					cek.NewDefaultEvalContext(
						lang.LanguageVersionV1, cek.ProtoVersion{Major: 11},
					),
				)
			},
		},
		{
			name: "PlutusV2",
			eval: func(s []byte, redeemer data.PlutusData) (common.ExUnits, error) {
				return common.PlutusV2Script(s).Evaluate(
					datum, redeemer, ctx, common.ExUnits{},
					cek.NewDefaultEvalContext(
						lang.LanguageVersionV2, cek.ProtoVersion{Major: 11},
					),
				)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The script demands the canonical encoding, which is what the
			// reference implementation always produces.
			s := encodeRedeemerAssertScript(
				t, lang.LanguageVersion{1, 0, 0}, normDef,
			)
			_, err := tc.eval(s, indefinite)
			require.NoError(t, err, "indefinite-encoded redeemer should evaluate")
			_, err = tc.eval(s, definite)
			require.NoError(t, err,
				"definite-encoded redeemer must be normalized before reaching the script")
			t.Log("both wire encodings reached the script as canonical bytes")
		})
	}
}
