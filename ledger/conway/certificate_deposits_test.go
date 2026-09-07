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

package conway_test

import (
	"crypto/ed25519"
	"errors"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	mockledger "github.com/blinklabs-io/ouroboros-mock/ledger"
	"github.com/stretchr/testify/require"
)

const (
	certificateDepositInputAmount = uint64(2_000_000_000_000)
	certificateDepositFee         = uint64(200_000)
	certificateDepositTxId        = "d228b482a1aae768e4a796380f49e021d9c21f70d3c12cb186b188dedfc0ee22"
)

type certificateDepositCredentialKey struct {
	credType uint
	hash     common.Blake2b224
}

type certificateDepositLedgerState struct {
	common.LedgerState
	deposits map[certificateDepositCredentialKey]uint64
}

func (s certificateDepositLedgerState) StakeCredentialDeposit(
	cred common.Credential,
) (*uint64, error) {
	deposit, found := s.deposits[certificateDepositCredentialKey{
		credType: cred.CredType,
		hash:     cred.Credential,
	}]
	if !found {
		return nil, nil
	}
	return &deposit, nil
}

type certificateDepositCredentialFixture struct {
	credential common.Credential
	privateKey ed25519.PrivateKey
	nativeCbor []byte
}

func newCertificateDepositCredentialFixture(
	t *testing.T,
	credType uint,
) certificateDepositCredentialFixture {
	t.Helper()
	switch credType {
	case common.CredentialTypeAddrKeyHash:
		privateKey := ed25519.NewKeyFromSeed(make([]byte, ed25519.SeedSize))
		publicKey := privateKey.Public().(ed25519.PublicKey)
		return certificateDepositCredentialFixture{
			credential: common.Credential{
				CredType:   credType,
				Credential: common.Blake2b224Hash(publicKey),
			},
			privateKey: privateKey,
		}
	case common.CredentialTypeScriptHash:
		nativeCbor, err := cbor.Encode([]any{uint64(1), []any{}})
		require.NoError(t, err)
		var nativeScript common.NativeScript
		_, err = cbor.Decode(nativeCbor, &nativeScript)
		require.NoError(t, err)
		return certificateDepositCredentialFixture{
			credential: common.Credential{
				CredType:   credType,
				Credential: common.Blake2b224(nativeScript.Hash()),
			},
			nativeCbor: nativeCbor,
		}
	default:
		t.Fatalf("unsupported credential type %d", credType)
		return certificateDepositCredentialFixture{}
	}
}

func certificateDepositPparams() *conway.ConwayProtocolParameters {
	return &conway.ConwayProtocolParameters{
		KeyDeposit:   2_000_000,
		DRepDeposit:  500_000_000,
		MaxTxSize:    16_384,
		MaxValueSize: 5_000,
		ProtocolVersion: common.ProtocolParametersProtocolVersion{
			Major: common.ProtocolVersionVanRossem,
		},
	}
}

func certificateDepositOutput(t *testing.T, amount uint64) cbor.RawMessage {
	t.Helper()
	encoded, err := cbor.Encode([]any{
		append([]byte{0x61}, make([]byte, 28)...),
		amount,
	})
	require.NoError(t, err)
	return cbor.RawMessage(encoded)
}

func certificateDepositTransaction(
	t *testing.T,
	fixture certificateDepositCredentialFixture,
	certificateCbors [][]byte,
	certificateValueDelta int64,
	withdrawalAmount uint64,
) *conway.ConwayTransaction {
	t.Helper()
	inputCbor, err := cbor.Encode(
		shelley.NewShelleyTransactionInput(certificateDepositTxId, 0),
	)
	require.NoError(t, err)
	outputAmount := int64(certificateDepositInputAmount) +
		certificateValueDelta + int64(withdrawalAmount) -
		int64(certificateDepositFee)
	require.Positive(t, outputAmount)
	certificates := make([]cbor.RawMessage, len(certificateCbors))
	for idx, certificateCbor := range certificateCbors {
		certificates[idx] = cbor.RawMessage(certificateCbor)
	}
	bodyFields := map[int]any{
		0: cbor.Tag{
			Number:  258,
			Content: []cbor.RawMessage{cbor.RawMessage(inputCbor)},
		},
		1: []cbor.RawMessage{
			certificateDepositOutput(t, uint64(outputAmount)),
		},
		2: certificateDepositFee,
		4: certificates,
	}
	if withdrawalAmount > 0 {
		header := byte(0xe1)
		if fixture.credential.CredType == common.CredentialTypeScriptHash {
			header = 0xf1
		}
		rewardAddress := append(
			[]byte{header},
			fixture.credential.Credential.Bytes()...,
		)
		keyCbor, err := cbor.Encode(rewardAddress)
		require.NoError(t, err)
		valueCbor, err := cbor.Encode(withdrawalAmount)
		require.NoError(t, err)
		withdrawalsCbor := []byte{0xa1}
		withdrawalsCbor = append(withdrawalsCbor, keyCbor...)
		withdrawalsCbor = append(withdrawalsCbor, valueCbor...)
		bodyFields[5] = cbor.RawMessage(withdrawalsCbor)
	}
	bodyCbor, err := cbor.Encode(bodyFields)
	require.NoError(t, err)
	var body conway.ConwayTransactionBody
	_, err = cbor.Decode(bodyCbor, &body)
	require.NoError(t, err)

	probe := &conway.ConwayTransaction{Body: body, TxIsValid: true}
	witnessFields := map[int]any{}
	if fixture.privateKey != nil {
		publicKey := fixture.privateKey.Public().(ed25519.PublicKey)
		hash := probe.Hash()
		witnessFields[0] = []any{
			[]any{[]byte(publicKey), ed25519.Sign(fixture.privateKey, hash[:])},
		}
	}
	if fixture.nativeCbor != nil {
		witnessFields[1] = []cbor.RawMessage{
			cbor.RawMessage(fixture.nativeCbor),
		}
	}
	witnessCbor, err := cbor.Encode(witnessFields)
	require.NoError(t, err)
	txCbor, err := cbor.Encode([]any{
		cbor.RawMessage(bodyCbor),
		cbor.RawMessage(witnessCbor),
		true,
		nil,
	})
	require.NoError(t, err)
	var tx conway.ConwayTransaction
	_, err = cbor.Decode(txCbor, &tx)
	require.NoError(t, err)
	return &tx
}

func runCertificateDepositProductionRules(
	t *testing.T,
	tx *conway.ConwayTransaction,
	ls common.LedgerState,
	pp common.ProtocolParameters,
) error {
	t.Helper()
	for _, rule := range conway.UtxoValidationRules {
		if err := rule(tx, 0, ls, pp); err != nil {
			return err
		}
	}
	return nil
}

func certificateDepositBaseState(
	pool common.PoolKeyHash,
) *mockledger.MockLedgerState {
	return mockledger.NewLedgerStateBuilder().
		WithUtxos([]common.Utxo{{
			Id: shelley.NewShelleyTransactionInput(
				certificateDepositTxId,
				0,
			),
			Output: shelley.ShelleyTransactionOutput{
				OutputAmount: certificateDepositInputAmount,
			},
		}}).
		WithNetworkId(1).
		WithPoolRegistrations([]common.PoolRegistrationCertificate{{
			Operator: pool,
		}}).
		Build()
}

func TestCertificateRegistrationDepositsProductionPath(t *testing.T) {
	pp := certificateDepositPparams()
	pool := common.PoolKeyHash(common.Blake2b224Hash([]byte("pool")))
	drepAbstain := []any{uint64(common.DrepTypeAbstain)}
	credentialTypes := []uint{
		common.CredentialTypeAddrKeyHash,
		common.CredentialTypeScriptHash,
	}
	type certificateCase struct {
		name     string
		expected uint64
		build    func(common.Credential, int64) []any
	}
	cases := []certificateCase{
		{
			name:     "stake registration",
			expected: uint64(pp.KeyDeposit),
			build: func(cred common.Credential, amount int64) []any {
				return []any{
					uint64(7),
					[]any{cred.CredType, cred.Credential.Bytes()},
					amount,
				}
			},
		},
		{
			name:     "stake registration delegation",
			expected: uint64(pp.KeyDeposit),
			build: func(cred common.Credential, amount int64) []any {
				return []any{
					uint64(11),
					[]any{cred.CredType, cred.Credential.Bytes()},
					pool.Bytes(),
					amount,
				}
			},
		},
		{
			name:     "vote registration delegation",
			expected: uint64(pp.KeyDeposit),
			build: func(cred common.Credential, amount int64) []any {
				return []any{
					uint64(12),
					[]any{cred.CredType, cred.Credential.Bytes()},
					drepAbstain,
					amount,
				}
			},
		},
		{
			name:     "stake vote registration delegation",
			expected: uint64(pp.KeyDeposit),
			build: func(cred common.Credential, amount int64) []any {
				return []any{
					uint64(13),
					[]any{cred.CredType, cred.Credential.Bytes()},
					pool.Bytes(),
					drepAbstain,
					amount,
				}
			},
		},
		{
			name:     "DRep registration",
			expected: pp.DRepDeposit,
			build: func(cred common.Credential, amount int64) []any {
				return []any{
					uint64(16),
					[]any{cred.CredType, cred.Credential.Bytes()},
					amount,
					nil,
				}
			},
		},
	}
	for _, credType := range credentialTypes {
		fixture := newCertificateDepositCredentialFixture(t, credType)
		t.Run(
			"legacy stake registration/credential-"+string(rune('0'+credType)),
			func(t *testing.T) {
				certificateCbor, err := cbor.Encode([]any{
					uint64(0),
					[]any{
						fixture.credential.CredType,
						fixture.credential.Credential.Bytes(),
					},
				})
				require.NoError(t, err)
				// Legacy stake-registration certificates do not carry a
				// credential-auth purpose.  In particular, an explicit native
				// script witness is extraneous; script credentials become
				// authorized by the later operation that uses them.
				legacyFixture := fixture
				legacyFixture.nativeCbor = nil
				tx := certificateDepositTransaction(
					t,
					legacyFixture,
					[][]byte{certificateCbor},
					-int64(pp.KeyDeposit),
					0,
				)
				ls := certificateDepositLedgerState{
					LedgerState: certificateDepositBaseState(pool),
					deposits:    map[certificateDepositCredentialKey]uint64{},
				}
				require.NoError(
					t,
					runCertificateDepositProductionRules(t, tx, ls, pp),
				)
			},
		)
		for _, testCase := range cases {
			t.Run(
				testCase.name+"/credential-"+string(rune('0'+credType)),
				func(t *testing.T) {
					for _, valid := range []bool{true, false} {
						t.Run(
							map[bool]string{true: "valid", false: "invalid"}[valid],
							func(t *testing.T) {
								amount := int64(testCase.expected)
								if !valid {
									amount--
								}
								certificateCbor, err := cbor.Encode(
									testCase.build(fixture.credential, amount),
								)
								require.NoError(t, err)
								tx := certificateDepositTransaction(
									t,
									fixture,
									[][]byte{certificateCbor},
									-amount,
									0,
								)
								ls := certificateDepositLedgerState{
									LedgerState: certificateDepositBaseState(
										pool,
									),
									deposits: map[certificateDepositCredentialKey]uint64{},
								}
								err = runCertificateDepositProductionRules(
									t,
									tx,
									ls,
									pp,
								)
								if valid {
									require.NoError(t, err)
									return
								}
								var target conway.CertificateDepositIncorrectError
								require.True(
									t,
									errors.As(err, &target),
									"unexpected error: %v",
									err,
								)
							},
						)
					}
				},
			)
		}
	}
}

func TestCertificateDeregistrationStateProductionPath(t *testing.T) {
	pp := certificateDepositPparams()
	pool := common.PoolKeyHash(common.Blake2b224Hash([]byte("pool")))
	for _, credType := range []uint{
		common.CredentialTypeAddrKeyHash,
		common.CredentialTypeScriptHash,
	} {
		t.Run("credential-"+string(rune('0'+credType)), func(t *testing.T) {
			fixture := newCertificateDepositCredentialFixture(t, credType)
			credentialCbor := []any{
				fixture.credential.CredType,
				fixture.credential.Credential.Bytes(),
			}
			buildState := func(registered bool, balance, deposit uint64) common.LedgerState {
				builder := mockledger.NewLedgerStateBuilder().
					WithUtxos([]common.Utxo{{
						Id: shelley.NewShelleyTransactionInput(certificateDepositTxId, 0),
						Output: shelley.ShelleyTransactionOutput{
							OutputAmount: certificateDepositInputAmount,
						},
					}}).
					WithNetworkId(1).
					WithPoolRegistrations([]common.PoolRegistrationCertificate{{Operator: pool}})
				deposits := map[certificateDepositCredentialKey]uint64{}
				if registered {
					builder.WithRewardAccountCredentialBalance(
						fixture.credential,
						balance,
					)
					deposits[certificateDepositCredentialKey{
						credType: fixture.credential.CredType,
						hash:     fixture.credential.Credential,
					}] = deposit
				}
				return certificateDepositLedgerState{
					LedgerState: builder.Build(),
					deposits:    deposits,
				}
			}
			buildDeregistration := func(refund int64, withdrawal uint64) *conway.ConwayTransaction {
				certificateCbor, err := cbor.Encode(
					[]any{uint64(8), credentialCbor, refund},
				)
				require.NoError(t, err)
				return certificateDepositTransaction(
					t,
					fixture,
					[][]byte{certificateCbor},
					refund,
					withdrawal,
				)
			}
			buildLegacyDeregistration := func(refund uint64) *conway.ConwayTransaction {
				certificateCbor, err := cbor.Encode([]any{
					uint64(1),
					credentialCbor,
				})
				require.NoError(t, err)
				return certificateDepositTransaction(
					t,
					fixture,
					[][]byte{certificateCbor},
					int64(refund),
					0,
				)
			}

			t.Run("valid recorded refund", func(t *testing.T) {
				tx := buildDeregistration(int64(pp.KeyDeposit), 0)
				require.NoError(t, runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, 0, uint64(pp.KeyDeposit)),
					pp,
				))
			})
			t.Run("incorrect refund", func(t *testing.T) {
				tx := buildDeregistration(int64(pp.KeyDeposit)+1, 0)
				err := runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, 0, uint64(pp.KeyDeposit)),
					pp,
				)
				var target conway.CertificateRefundIncorrectError
				require.True(
					t,
					errors.As(err, &target),
					"unexpected error: %v",
					err,
				)
			})
			t.Run("legacy recorded refund", func(t *testing.T) {
				tx := buildLegacyDeregistration(uint64(pp.KeyDeposit))
				require.NoError(t, runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, 0, uint64(pp.KeyDeposit)),
					pp,
				))
			})
			// A credential may have registered before KeyDeposit changed. Its
			// legacy deregistration refunds the deposit recorded in ledger
			// state rather than the current protocol parameter.
			t.Run("legacy refund follows recorded deposit", func(t *testing.T) {
				recorded := uint64(pp.KeyDeposit) + 1
				tx := buildLegacyDeregistration(recorded)
				require.NoError(t, runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, 0, recorded),
					pp,
				))
			})
			t.Run("legacy refund of the parameter is not conserved", func(t *testing.T) {
				// Balancing against the current parameter while state records a
				// larger deposit must fail value conservation, which proves the
				// refund is taken from ledger state.
				tx := buildLegacyDeregistration(uint64(pp.KeyDeposit))
				err := runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, 0, uint64(pp.KeyDeposit)+1),
					pp,
				)
				var target shelley.ValueNotConservedUtxoError
				require.True(
					t,
					errors.As(err, &target),
					"unexpected error: %v",
					err,
				)
			})
			t.Run("unregistered", func(t *testing.T) {
				tx := buildDeregistration(int64(pp.KeyDeposit), 0)
				err := runCertificateDepositProductionRules(
					t,
					tx,
					buildState(false, 0, 0),
					pp,
				)
				var target conway.StakeCredentialNotRegisteredError
				require.True(
					t,
					errors.As(err, &target),
					"unexpected error: %v",
					err,
				)
			})
			t.Run("nonzero reward balance", func(t *testing.T) {
				tx := buildDeregistration(int64(pp.KeyDeposit), 0)
				err := runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, 5_000_000, uint64(pp.KeyDeposit)),
					pp,
				)
				var target conway.StakeCredentialNonZeroRewardBalanceError
				require.True(
					t,
					errors.As(err, &target),
					"unexpected error: %v",
					err,
				)
			})
			t.Run("withdraw all before deregistration", func(t *testing.T) {
				const balance = uint64(5_000_000)
				tx := buildDeregistration(int64(pp.KeyDeposit), balance)
				withdrawalPparams := *pp
				withdrawalPparams.ProtocolVersion.Major = common.ProtocolVersionDijkstra
				require.NoError(t, runCertificateDepositProductionRules(
					t,
					tx,
					buildState(true, balance, uint64(pp.KeyDeposit)),
					&withdrawalPparams,
				))
			})
		})
	}
}

func TestDRepDeregistrationRefundProductionPath(t *testing.T) {
	pp := certificateDepositPparams()
	for _, credType := range []uint{
		common.CredentialTypeAddrKeyHash,
		common.CredentialTypeScriptHash,
	} {
		fixture := newCertificateDepositCredentialFixture(t, credType)
		credentialCbor := []any{
			fixture.credential.CredType,
			fixture.credential.Credential.Bytes(),
		}
		for _, refund := range []int64{
			int64(pp.DRepDeposit),
			int64(pp.DRepDeposit) + 1,
		} {
			certificateCbor, err := cbor.Encode(
				[]any{uint64(17), credentialCbor, refund},
			)
			require.NoError(t, err)
			tx := certificateDepositTransaction(
				t,
				fixture,
				[][]byte{certificateCbor},
				refund,
				0,
			)
			baseState := mockledger.NewLedgerStateBuilder().
				WithUtxos([]common.Utxo{{
					Id: shelley.NewShelleyTransactionInput(
						certificateDepositTxId,
						0,
					),
					Output: shelley.ShelleyTransactionOutput{
						OutputAmount: certificateDepositInputAmount,
					},
				}}).
				WithNetworkId(1).
				WithDRepRegistrations([]common.DRepRegistration{{
					Credential: fixture.credential.Credential,
					Deposit:    pp.DRepDeposit,
				}}).
				Build()
			ls := certificateDepositLedgerState{
				LedgerState: baseState,
				deposits:    map[certificateDepositCredentialKey]uint64{},
			}
			err = runCertificateDepositProductionRules(t, tx, ls, pp)
			if refund == int64(pp.DRepDeposit) {
				require.NoError(t, err)
				continue
			}
			var target conway.CertificateRefundIncorrectError
			require.True(
				t,
				errors.As(err, &target),
				"unexpected error: %v",
				err,
			)
		}
		certificateCbor, err := cbor.Encode([]any{
			uint64(17),
			credentialCbor,
			int64(pp.DRepDeposit),
		})
		require.NoError(t, err)
		tx := certificateDepositTransaction(
			t,
			fixture,
			[][]byte{certificateCbor},
			int64(pp.DRepDeposit),
			0,
		)
		ls := certificateDepositLedgerState{
			LedgerState: mockledger.NewLedgerStateBuilder().
				WithUtxos([]common.Utxo{{
					Id: shelley.NewShelleyTransactionInput(
						certificateDepositTxId,
						0,
					),
					Output: shelley.ShelleyTransactionOutput{
						OutputAmount: certificateDepositInputAmount,
					},
				}}).
				WithNetworkId(1).
				Build(),
			deposits: map[certificateDepositCredentialKey]uint64{},
		}
		err = runCertificateDepositProductionRules(t, tx, ls, pp)
		var target conway.DRepNotRegisteredError
		require.True(t, errors.As(err, &target), "unexpected error: %v", err)
	}
}

func TestCertificateDepositStateFoldProductionPath(t *testing.T) {
	pp := certificateDepositPparams()
	pool := common.PoolKeyHash(common.Blake2b224Hash([]byte("pool")))
	for _, credType := range []uint{
		common.CredentialTypeAddrKeyHash,
		common.CredentialTypeScriptHash,
	} {
		fixture := newCertificateDepositCredentialFixture(t, credType)
		credentialCbor := []any{
			fixture.credential.CredType,
			fixture.credential.Credential.Bytes(),
		}
		registrationCbor, err := cbor.Encode([]any{
			uint64(7),
			credentialCbor,
			int64(pp.KeyDeposit),
		})
		require.NoError(t, err)
		deregistrationCbor, err := cbor.Encode([]any{
			uint64(8),
			credentialCbor,
			int64(pp.KeyDeposit),
		})
		require.NoError(t, err)
		tx := certificateDepositTransaction(
			t,
			fixture,
			[][]byte{registrationCbor, deregistrationCbor},
			0,
			0,
		)
		ls := certificateDepositLedgerState{
			LedgerState: certificateDepositBaseState(pool),
			deposits:    map[certificateDepositCredentialKey]uint64{},
		}
		require.NoError(t, runCertificateDepositProductionRules(t, tx, ls, pp))
	}
}

var _ common.StakeCredentialDepositState = certificateDepositLedgerState{}
