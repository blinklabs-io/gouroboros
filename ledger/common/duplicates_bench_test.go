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

package common_test

import (
	"fmt"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
)

// benchmarkDelegationCertificates builds distinct stake delegations of one
// type, which is the case the typeCounts gate cannot skip and the one ordinary
// transactions hit.
func benchmarkDelegationCertificates(count int) []common.CertificateWrapper {
	certificates := make([]common.CertificateWrapper, 0, count)
	for idx := range count {
		credential := common.Credential{
			CredType:   common.CredentialTypeAddrKeyHash,
			Credential: common.Blake2b224Hash([]byte(fmt.Sprintf("cred-%d", idx))),
		}
		certificates = append(certificates, common.CertificateWrapper{
			Type: uint(common.CertificateTypeStakeDelegation),
			Certificate: &common.StakeDelegationCertificate{
				CertType:        uint(common.CertificateTypeStakeDelegation),
				StakeCredential: &credential,
				PoolKeyHash: common.PoolKeyHash(
					common.Blake2b224Hash([]byte("pool")),
				),
			},
		})
	}
	return certificates
}

func BenchmarkValidateCertificateSet(b *testing.B) {
	for _, count := range []int{2, 16} {
		certificates := benchmarkDelegationCertificates(count)
		b.Run(fmt.Sprintf("same_type_%d", count), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := common.ValidateCertificateSet(certificates); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
