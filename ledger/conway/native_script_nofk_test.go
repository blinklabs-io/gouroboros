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
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/stretchr/testify/require"
)

// Preprod block 5184855 (slot 133902219). Its first transaction carries the
// native script [3, -1, [pubkey, pubkey]] in its witness set.
// conway.cddl types script_n_of_k's n as int64.
const preprod5184855Hex = "85828a1a004f1d571a07fb2f8b58209bb45a4c44f4e8f973fa1fca719038cb57440504c7a510cf5ceda4329f5169485820bae7fccf57e9292efa30244aed16f64471ac32957c50e5261c33d31226df84ba5820d754ac94a56d8ca60309a56c42d38a6eb910250b771cfc039644464d7c2a384c8258403457578b025369c1c66b99643db01216949d3d7565be2f37daef424db1ab095c8f818be38e6e317431b441935002e60a98c3cfca3289d68f1009503400cc51ba5850025c01a216937e4d928fa4b55316ee7703d6562d754e21fc42325ea20360ea2d790101350e248a217de38649c2e93d93788e6c383afe1239ac7f29ea1dc20128a886b9fa3f1c3309e8ea103c4b80800619018c5820be756b6339b37957dda1edab84f1ab24d951bf26b3b35fed951da5fe8d917b688458201b45647e9c697cf678ac27db1a81c0ef89dccc97cd1281ff601aa57edea2b040011903ed584027af52dedb2d4703e5762258b621844b433efc3602733143c791874ce21680e63164518a01a646d49272e8685316ef833885cb830521c5c26db530474239000b820b005901c0769a8f93205ab6cfd072d4d561adc5ef2d25fea87600d7f4c973abb9f2ef73600a866d93abb18f2d73bf954c3ebd7c27efb237fa2db3d0cfa05bc994a1361a0c83615430dfc6d94462dfb1043cb72028161f175a62240eabf57278fd6b4b652329f934030760e0509e0418c79011f9cb1ae8ba3ed63fe17dd8e2a6ffb2fbf326b34d340f548683aacaf89f448812d3ae05452bcb6e3235d6597c8ec6989194f16ba14a1febabaf69eb6b8bebd557674326c1e1e40c73d9492ec4aaf581ea2f971f1bd0f19eea34ebac5e270bebb332412a1131308ef8402bec738f29a537cf83288c157a37970fdd272f070105f8d449296c4c1132105628015692ae7cd0db4bf6a48a900233465fa851c8881df683acdc9a9656b22285e92ba2a8e735a4c2d485fe99b3c3ca00a6c906104c4c580b5b13c49e9bf2d9c900a5fedfc90c5635501803e1fae0fee31e46993808126bf7a91746fa9a7c381274a8901135bc2a133f727594d8d27faeab5bed0126bf5b1dd975af690b0798352319c628b41b75de358002aeb226c72048e574ff765c5a01606b4cb6b0a99fc162670875b2103f1850149e1eefa76ce98c63491b282636d80ea61a6ee1b8293f6245ce6ee9ba9ce12c82a300d9010281825820fa72e22e7661608e75c8837b26bfc7b9fb61ddcf3456f12fb58d57ab7437002f00018182581d60d8188ff2e2bd2384524e2e52b9f0ee0dc19e50ab7baac6d771f6d45e1a001c095b021a00027b25a300d901028182582028afe751287e7f5ee7df73545027960ea0cb1687d07a64937550f0fdac1c0b8300018282581d703530cc9ae7f2895111a99b7a02184dd7c0cea7424f1632d73951b1d71a001e848082581d60d8188ff2e2bd2384524e2e52b9f0ee0dc19e50ab7baac6d771f6d45e1a00239e6a021a0002872d82a101d9010281830320828200581c3118644aa21ba172c82732ce80d1c94cdcb5f2e8891e1ad2645707188200581ce07caf4bf751495f75774ace30552441e4df84d141e5d1f5029cb04da100d90102818258204d567caf498bfdd22cf2b954f77b7c0a1739c9aeda649649dc665efda6e20c705840aa310403b2a6ae1cf7988c7f1a2ad48bfd4dd77a2c1196058d627c9e987309acd978ebbb6f3ee5b339e390d315a3e659bbf353afb4775f57fe3c8123ed571e02a080"

func TestConwayBlockNofKInt64Threshold(t *testing.T) {
	raw, err := hex.DecodeString(preprod5184855Hex)
	require.NoError(t, err)
	blk, err := conway.NewConwayBlockFromCbor(raw)
	require.NoError(t, err)
	require.Equal(t,
		"8ddb2e9618495b11dc5bf2820d177ec38453faaf81afbfdb02ab99a7ebc9e2f8",
		blk.Hash().String(),
	)
	txs := blk.Transactions()
	require.Len(t, txs, 2)
	witnesses := txs[0].Witnesses()
	if witnesses == nil {
		t.Fatal("transaction 0 has no witness set")
	}
	scripts := witnesses.NativeScripts()
	require.Len(t, scripts, 1)
	require.Equal(t,
		"678f1ce0680cd787baac04fe6e30a82c1fbf2998bee934cc040960d4",
		scripts[0].Hash().String(),
	)
	// Haskell evalTimelock: isValidMOf n _ = n <= 0 || ...
	require.True(t, scripts[0].Evaluate(0, 0, ^uint64(0), map[common.Blake2b224]bool{}))
}
