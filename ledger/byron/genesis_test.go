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
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger/byron"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockForever is an io.Reader whose Read never returns, simulating a
// connection or stream that stays open indefinitely past the data a caller
// cares about.
type blockForever struct{}

func (blockForever) Read([]byte) (int, error) {
	select {}
}

const byronGenesisConfig = `
{
    "avvmDistr": {
        "-0BJDi-gauylk4LptQTgjMeo7kY9lTCbZv12vwOSTZk=": "9999300000000",
        "-0Np4pyTOWF26iXWVIvu6fhz9QupwWRS2hcCaOEYlw0=": "3760024000000",
        "ocEVzr7ctJ3NyCWj_356QsTyINKoJlwCCAgKFGPjvqg=": "2074165643000000",
        "5Y1_PK90x1jCNzzQwthIt0RGT7E3PAPwDI-tbHN39l8=": "648176763000000"
    },
    "blockVersionData": {
        "heavyDelThd": "300000000000",
        "maxBlockSize": "2000000",
        "maxHeaderSize": "2000000",
        "maxProposalSize": "700",
        "maxTxSize": "4096",
        "mpcThd": "20000000000000",
        "scriptVersion": 0,
        "slotDuration": "20000",
        "softforkRule": {
            "initThd": "900000000000000",
            "minThd": "600000000000000",
            "thdDecrement": "50000000000000"
        },
        "txFeePolicy": {
            "multiplier": "43946000000",
            "summand": "155381000000000"
        },
        "unlockStakeEpoch": "18446744073709551615",
        "updateImplicit": "10000",
        "updateProposalThd": "100000000000000",
        "updateVoteThd": "1000000000000"
    },
	"ftsSeed": "76617361206f7061736120736b6f766f726f64612047677572646120626f726f64612070726f766f6461",
    "protocolConsts": {
        "k": 2160,
        "protocolMagic": 764824073,
        "vssMaxTTL": 6,
        "vssMinTTL": 2
    },
    "startTime": 1506203091,
    "bootStakeholders": {
        "1deb82908402c7ee3efeb16f369d97fba316ee621d09b32b8969e54b": 1,
        "65904a89e6d0e5f881513d1736945e051b76f095eca138ee869d543d": 1,
        "5411c7bf87c252609831a337a713e4859668cba7bba70a9c3ef7c398": 1,

        "6c9e14978b9d6629b8703f4f25e9df6ed4814b930b8403b0d45350ea": 1,
        "43011479a595b300e0726910d0b602ffcdd20466a3b8ceeacd3fbc26": 1,

        "5071d8802ddd05c59f4db907bd1749e82e6242caf6512b20a8368fcf": 1,
        "af2800c124e599d6dec188a75f8bfde397ebb778163a18240371f2d1": 1
    },
    "heavyDelegation": {
        "1deb82908402c7ee3efeb16f369d97fba316ee621d09b32b8969e54b":{"cert":"c8b39f094dc00608acb2d20ff274cb3e0c022ccb0ce558ea7c1a2d3a32cd54b42cc30d32406bcfbb7f2f86d05d2032848be15b178e3ad776f8b1bc56a671400d","delegatePk":"6MA6A8Cy3b6kGVyvOfQeZp99JR7PIh+7LydcCl1+BdGQ3MJG9WyOM6wANwZuL2ZN2qmF6lKECCZDMI3eT1v+3w==","issuerPk":"UHMxYf2vtsjLb64OJb35VVEFs2eO+wjxd1uekN5PXHe8yM7/+NkBHLJ4so/dyG2bqwmWVtd6eFbHYZEIy/ZXUg==","omega":0},
        "65904a89e6d0e5f881513d1736945e051b76f095eca138ee869d543d":{"cert":"552741f728196e62f218047b944b24ce4d374300d04b9b281426f55aa000d53ded66989ad5ea0908e6ff6492001ff18ece6c7040a934060759e9ae09863bf203","delegatePk":"X93u2t4nFNbbL54RBHQ9LY2Bjs3cMG4XYQjbFMqt1EG0V9WEDGD4hAuZyPeMKQriKdT4Qx5ni6elRcNWB7lN2w==","issuerPk":"C9sfXvPZlAN1k/ImYlXxNKVkZYuy34FLO5zvuW2jT6nIiFkchbdw/TZybV89mRxmiCiv/Hu+CHL9aZE25mTZ2A==","omega":0},
        "5411c7bf87c252609831a337a713e4859668cba7bba70a9c3ef7c398":{"cert":"c946fd596bdb31949aa435390de19a549c9698cad1813e34ff2431bc06190188188f4e84001380713e3f916c7526096e7c4855904bff40385007b81e1e657d0e","delegatePk":"i1Mgdin5ow5LIBUETzN8AXNavmckPBlHDJ2ujHtzJ5gJiHufQg1vcO4enVDBYFKHjlRLZctdRL1pF1ayhM2Cmw==","issuerPk":"mm+jQ8jGw23ho1Vv60Eb/fhwjVr4jehibQ/Gv6Tuu22Zq4PZDWZTGtkSLT+ctLydBbJkSGclMqaNp5b5MoQx/Q==","omega":0},

        "6c9e14978b9d6629b8703f4f25e9df6ed4814b930b8403b0d45350ea":{"cert":"8ab43e904b06e799c1817c5ced4f3a7bbe15cdbf422dea9d2d5dc2c6105ce2f4d4c71e5d4779f6c44b770a133636109949e1f7786acb5a732bcdea0470fea406","delegatePk":"8U9xLcYA15MFLUhC1QzvpOZYhOps+DcHB564zjAu/IXa6SLV6zg40rkXhPBIJNJnZ7+2W9NqNudP7EbQnZiFjQ==","issuerPk":"JlZuhvxrmxd8hIDidbKxErVz9tBz+d7qU7jZnE7ZdrM1srOELw44AAHwkLySPKqWke2RFeKG2pQh4nRcesyH8Q==","omega":0},
        "43011479a595b300e0726910d0b602ffcdd20466a3b8ceeacd3fbc26":{"cert":"cf6ddc111545f61c2442b68bd7864ea952c428d145438948ef48a4af7e3f49b175564007685be5ae3c9ece0ab27de09721db0cb63aa67dc081a9f82d7e84210d","delegatePk":"kYDYGOac2ZfjRmPEGKZIwHby4ZzUGU5IbhWdhYC8bNqBNERAxq0OUwb9A1vvkoHaXY+9OPWfWI9wgQFu5hET0g==","issuerPk":"0pZchpkBIxeYxdAtOfyip5qkfD6FSSG1hVyC/RRwiRUX4fp3FlXsjK0T7PblcZrcU5L8BX4XA9X1gzEeg3Ri8Q==","omega":0},

        "5071d8802ddd05c59f4db907bd1749e82e6242caf6512b20a8368fcf":{"cert":"496b29b5c57e8ac7cffc6e8b5e40b3d260e407ad4d09792decb0a22d54da7f8828265688a18aa1a5c76d9e7477a5f4a650501409fdcd3855b300fd2e2bc3c605","delegatePk":"icKfjErye3rMvliXR4IBNOu6ocrzzpSScKPQx9z9VBsd7zJtLvDbeANByeJh8EiQze7x+cmfbZC47cp9PPwJiA==","issuerPk":"mTqPBW0tPlCwrGATnxDfj4Ej1ffEgXtA2sK13YqpSoLoU2gy5jEt38B4fXtTEMgVZVraT9vPaxIpfURY7Mwt+w==","omega":0},
        "af2800c124e599d6dec188a75f8bfde397ebb778163a18240371f2d1":{"cert":"e03e62f083df5576360e60a32e22bbb07b3c8df4fcab8079f1d6f61af3954d242ba8a06516c395939f24096f3df14e103a7d9c2b80a68a9363cf1f27c7a4e307","delegatePk":"YSYalbdhPua/IGfa13twNJcpsMUNV7wc8w3g20oec6iF0AVK98I/xsN5GdukHGAqV+LQ+TKaeVS4ZzONb7LJRQ==","issuerPk":"G8l6L+AsKXiAzo7P2Zf+TB7AnuEP7u6faGdgFmsFKB1ig0aP/ZO+ywyVbM3dZC35sSRMkVkRGF+kk1X28iv6uQ==","omega":0}
    },
    "nonAvvmBalances": {},
    "vssCerts": {
        "6bef444609d6e336cb1fe1daba278918dbc5768e6754c2945dd8d25c":{"expiryEpoch":5,"signature":"2d96e4d4a4c506cc5762128b814ffb20afb97d30eb976334cd241a3935bd155ea1d68772b0903bde4584470359206769d83fa2ce55f56a1027ec3c52cb5e8703","signingKey":"6MA6A8Cy3b6kGVyvOfQeZp99JR7PIh+7LydcCl1+BdGQ3MJG9WyOM6wANwZuL2ZN2qmF6lKECCZDMI3eT1v+3w==","vssKey":"WCED6k6ArqOnhQtfNRg0FSCxWmAtocZcyV33AdjMotjGwxI="},
        "eb649333a196ecb024a4a5919d3ce86084014136fd3e884e52ecd057":{"expiryEpoch":5,"signature":"0b115a39935ce6008a4bbad0377f35463fd3510e282186ba43492768a02eb000bd4d3bc50799a24c53879ff2f2587179e797ee1c312acaf107cba67f91cb280b","signingKey":"X93u2t4nFNbbL54RBHQ9LY2Bjs3cMG4XYQjbFMqt1EG0V9WEDGD4hAuZyPeMKQriKdT4Qx5ni6elRcNWB7lN2w==","vssKey":"WCECS11PWxybUHKY2hHmBgm/zYaR2YsqsH+f3uPOp2ydz/E="},
        "5ffca3a329599727e39a7472c5270e54cf59a27b74306cc9f7fd0f5e":{"expiryEpoch":5,"signature":"6cc8d84dd55b41efcf46c4b3086da1fb60c938182b4b66657650839d9fac1e2194a8253dc6d5c107ac0e9e714d1364fff9d2114eae07363d9937ee1d92b69c06","signingKey":"i1Mgdin5ow5LIBUETzN8AXNavmckPBlHDJ2ujHtzJ5gJiHufQg1vcO4enVDBYFKHjlRLZctdRL1pF1ayhM2Cmw==","vssKey":"WCEDca27BxibVjQoA1QJaWx4gAE2MUB0lHfb6jJ3iorXD7s="},
        "ce1e50f578d3043dc78d8777f5723cc7b6ca512d8cdbe8a09aafc9c3":{"expiryEpoch":5,"signature":"2b830f1a79d2baca791a90c3784d74ec9f00267efac5ccd3cd7082b854234f411c237b59f34736933ba626fadc87fd6b2114c44486de692892d7401343990e01","signingKey":"8U9xLcYA15MFLUhC1QzvpOZYhOps+DcHB564zjAu/IXa6SLV6zg40rkXhPBIJNJnZ7+2W9NqNudP7EbQnZiFjQ==","vssKey":"WCECs1+lg8Lsm15FxfY8bhGyRuwe8yOaSH0wwSajLRYeW/s="},
        "0efd6f3b2849d5baf25b3e2bf2d46f88427b4e455fc3dc43f57819c5":{"expiryEpoch":5,"signature":"d381d32a18cd12a1c6ff87da0229c9a5b998fd093ac29f5d932bfc918e7dbc6e1dc292a36c46a3e129c5b1ef661124361426b443480534ff51dacc82bf4b630f","signingKey":"kYDYGOac2ZfjRmPEGKZIwHby4ZzUGU5IbhWdhYC8bNqBNERAxq0OUwb9A1vvkoHaXY+9OPWfWI9wgQFu5hET0g==","vssKey":"WCECgow+hJK+BxjNx0gIYrap+onUsRocObQEVzvJsdj68vw="},
        "1040655f58d5bf2be1c06f983abf66c7f01d28c239f27648a0c73e5d":{"expiryEpoch":5,"signature":"b02e89abb183da7c871bca87a563d38356b44f403348b6a5f24ee4459335290d980db69a6482455aae231a9880defe2fd4212272c4b2ea3da8744a8ba750440a","signingKey":"icKfjErye3rMvliXR4IBNOu6ocrzzpSScKPQx9z9VBsd7zJtLvDbeANByeJh8EiQze7x+cmfbZC47cp9PPwJiA==","vssKey":"WCECQoZjWJSu/6R74CC0ueh7cXmR0sasmTuCqf8X0BtAQ4o="},
        "1fa56ba63cff50d124b6af42f33b245a30fcd1b0170d7704b0b201c7":{"expiryEpoch":5,"signature":"7bb244c4fa1499021b0f2d36515a1f288a33cf00f1b88b57626998b439dcfb03ad88a7bc93101e4d83cdc75329799fbb2ccb28a7212a3e49737b06287d09b00c","signingKey":"YSYalbdhPua/IGfa13twNJcpsMUNV7wc8w3g20oec6iF0AVK98I/xsN5GdukHGAqV+LQ+TKaeVS4ZzONb7LJRQ==","vssKey":"WCECNXeQRqiTZSPDDyeRJ3gl/QzYMLLtNH0yN+XOl17pu8Y="}
    }
}
`

var expectedGenesisObj = byron.ByronGenesis{
	AvvmDistr: map[string]string{
		"-0BJDi-gauylk4LptQTgjMeo7kY9lTCbZv12vwOSTZk=": "9999300000000",
		"-0Np4pyTOWF26iXWVIvu6fhz9QupwWRS2hcCaOEYlw0=": "3760024000000",
		"5Y1_PK90x1jCNzzQwthIt0RGT7E3PAPwDI-tbHN39l8=": "648176763000000",
		"ocEVzr7ctJ3NyCWj_356QsTyINKoJlwCCAgKFGPjvqg=": "2074165643000000",
	},
	BlockVersionData: byron.ByronGenesisBlockVersionData{
		HeavyDelThd:     300000000000,
		MaxBlockSize:    2000000,
		MaxHeaderSize:   2000000,
		MaxProposalSize: 700,
		MaxTxSize:       4096,
		MpcThd:          20000000000000,
		ScriptVersion:   0,
		SlotDuration:    20000,
		SoftforkRule: byron.ByronGenesisBlockVersionDataSoftforkRule{
			InitThd:      900000000000000,
			MinThd:       600000000000000,
			ThdDecrement: 50000000000000,
		},
		TxFeePolicy: byron.ByronGenesisBlockVersionDataTxFeePolicy{
			Multiplier: 43946000000,
			Summand:    155381000000000,
		},
		UnlockStakeEpoch:  0xffffffffffffffff,
		UpdateImplicit:    10000,
		UpdateProposalThd: 100000000000000,
		UpdateVoteThd:     1000000000000,
	},
	FtsSeed: byron.ByronGenesisFtsSeed{
		Value:    "76617361206f7061736120736b6f766f726f64612047677572646120626f726f64612070726f766f6461",
		IsObject: false,
	},
	ProtocolConsts: byron.ByronGenesisProtocolConsts{
		K:             2160,
		ProtocolMagic: 764824073,
		VssMinTTL:     2,
		VssMaxTTL:     6,
	},
	StartTime: 1506203091,
	BootStakeholders: map[string]int{
		"1deb82908402c7ee3efeb16f369d97fba316ee621d09b32b8969e54b": 1,
		"43011479a595b300e0726910d0b602ffcdd20466a3b8ceeacd3fbc26": 1,
		"5071d8802ddd05c59f4db907bd1749e82e6242caf6512b20a8368fcf": 1,
		"5411c7bf87c252609831a337a713e4859668cba7bba70a9c3ef7c398": 1,
		"65904a89e6d0e5f881513d1736945e051b76f095eca138ee869d543d": 1,
		"6c9e14978b9d6629b8703f4f25e9df6ed4814b930b8403b0d45350ea": 1,
		"af2800c124e599d6dec188a75f8bfde397ebb778163a18240371f2d1": 1,
	},
	HeavyDelegation: map[string]byron.ByronGenesisHeavyDelegation{
		"1deb82908402c7ee3efeb16f369d97fba316ee621d09b32b8969e54b": {
			Cert:       "c8b39f094dc00608acb2d20ff274cb3e0c022ccb0ce558ea7c1a2d3a32cd54b42cc30d32406bcfbb7f2f86d05d2032848be15b178e3ad776f8b1bc56a671400d",
			DelegatePk: "6MA6A8Cy3b6kGVyvOfQeZp99JR7PIh+7LydcCl1+BdGQ3MJG9WyOM6wANwZuL2ZN2qmF6lKECCZDMI3eT1v+3w==",
			IssuerPk:   "UHMxYf2vtsjLb64OJb35VVEFs2eO+wjxd1uekN5PXHe8yM7/+NkBHLJ4so/dyG2bqwmWVtd6eFbHYZEIy/ZXUg==",
			Omega:      0,
		},
		"43011479a595b300e0726910d0b602ffcdd20466a3b8ceeacd3fbc26": {
			Cert:       "cf6ddc111545f61c2442b68bd7864ea952c428d145438948ef48a4af7e3f49b175564007685be5ae3c9ece0ab27de09721db0cb63aa67dc081a9f82d7e84210d",
			DelegatePk: "kYDYGOac2ZfjRmPEGKZIwHby4ZzUGU5IbhWdhYC8bNqBNERAxq0OUwb9A1vvkoHaXY+9OPWfWI9wgQFu5hET0g==",
			IssuerPk:   "0pZchpkBIxeYxdAtOfyip5qkfD6FSSG1hVyC/RRwiRUX4fp3FlXsjK0T7PblcZrcU5L8BX4XA9X1gzEeg3Ri8Q==",
			Omega:      0,
		},
		"5071d8802ddd05c59f4db907bd1749e82e6242caf6512b20a8368fcf": {
			Cert:       "496b29b5c57e8ac7cffc6e8b5e40b3d260e407ad4d09792decb0a22d54da7f8828265688a18aa1a5c76d9e7477a5f4a650501409fdcd3855b300fd2e2bc3c605",
			DelegatePk: "icKfjErye3rMvliXR4IBNOu6ocrzzpSScKPQx9z9VBsd7zJtLvDbeANByeJh8EiQze7x+cmfbZC47cp9PPwJiA==",
			IssuerPk:   "mTqPBW0tPlCwrGATnxDfj4Ej1ffEgXtA2sK13YqpSoLoU2gy5jEt38B4fXtTEMgVZVraT9vPaxIpfURY7Mwt+w==",
			Omega:      0,
		},
		"5411c7bf87c252609831a337a713e4859668cba7bba70a9c3ef7c398": {
			Cert:       "c946fd596bdb31949aa435390de19a549c9698cad1813e34ff2431bc06190188188f4e84001380713e3f916c7526096e7c4855904bff40385007b81e1e657d0e",
			DelegatePk: "i1Mgdin5ow5LIBUETzN8AXNavmckPBlHDJ2ujHtzJ5gJiHufQg1vcO4enVDBYFKHjlRLZctdRL1pF1ayhM2Cmw==",
			IssuerPk:   "mm+jQ8jGw23ho1Vv60Eb/fhwjVr4jehibQ/Gv6Tuu22Zq4PZDWZTGtkSLT+ctLydBbJkSGclMqaNp5b5MoQx/Q==",
			Omega:      0,
		},
		"65904a89e6d0e5f881513d1736945e051b76f095eca138ee869d543d": {
			Cert:       "552741f728196e62f218047b944b24ce4d374300d04b9b281426f55aa000d53ded66989ad5ea0908e6ff6492001ff18ece6c7040a934060759e9ae09863bf203",
			DelegatePk: "X93u2t4nFNbbL54RBHQ9LY2Bjs3cMG4XYQjbFMqt1EG0V9WEDGD4hAuZyPeMKQriKdT4Qx5ni6elRcNWB7lN2w==",
			IssuerPk:   "C9sfXvPZlAN1k/ImYlXxNKVkZYuy34FLO5zvuW2jT6nIiFkchbdw/TZybV89mRxmiCiv/Hu+CHL9aZE25mTZ2A==",
			Omega:      0,
		},
		"6c9e14978b9d6629b8703f4f25e9df6ed4814b930b8403b0d45350ea": {
			Cert:       "8ab43e904b06e799c1817c5ced4f3a7bbe15cdbf422dea9d2d5dc2c6105ce2f4d4c71e5d4779f6c44b770a133636109949e1f7786acb5a732bcdea0470fea406",
			DelegatePk: "8U9xLcYA15MFLUhC1QzvpOZYhOps+DcHB564zjAu/IXa6SLV6zg40rkXhPBIJNJnZ7+2W9NqNudP7EbQnZiFjQ==",
			IssuerPk:   "JlZuhvxrmxd8hIDidbKxErVz9tBz+d7qU7jZnE7ZdrM1srOELw44AAHwkLySPKqWke2RFeKG2pQh4nRcesyH8Q==",
			Omega:      0,
		},
		"af2800c124e599d6dec188a75f8bfde397ebb778163a18240371f2d1": {
			Cert:       "e03e62f083df5576360e60a32e22bbb07b3c8df4fcab8079f1d6f61af3954d242ba8a06516c395939f24096f3df14e103a7d9c2b80a68a9363cf1f27c7a4e307",
			DelegatePk: "YSYalbdhPua/IGfa13twNJcpsMUNV7wc8w3g20oec6iF0AVK98I/xsN5GdukHGAqV+LQ+TKaeVS4ZzONb7LJRQ==",
			IssuerPk:   "G8l6L+AsKXiAzo7P2Zf+TB7AnuEP7u6faGdgFmsFKB1ig0aP/ZO+ywyVbM3dZC35sSRMkVkRGF+kk1X28iv6uQ==",
			Omega:      0,
		},
	},
	NonAvvmBalances: map[string]string{},
	VssCerts: map[string]byron.ByronGenesisVssCert{
		"0efd6f3b2849d5baf25b3e2bf2d46f88427b4e455fc3dc43f57819c5": {
			ExpiryEpoch: 5,
			Signature:   "d381d32a18cd12a1c6ff87da0229c9a5b998fd093ac29f5d932bfc918e7dbc6e1dc292a36c46a3e129c5b1ef661124361426b443480534ff51dacc82bf4b630f",
			SigningKey:  "kYDYGOac2ZfjRmPEGKZIwHby4ZzUGU5IbhWdhYC8bNqBNERAxq0OUwb9A1vvkoHaXY+9OPWfWI9wgQFu5hET0g==",
			VssKey:      "WCECgow+hJK+BxjNx0gIYrap+onUsRocObQEVzvJsdj68vw=",
		},
		"1040655f58d5bf2be1c06f983abf66c7f01d28c239f27648a0c73e5d": {
			ExpiryEpoch: 5,
			Signature:   "b02e89abb183da7c871bca87a563d38356b44f403348b6a5f24ee4459335290d980db69a6482455aae231a9880defe2fd4212272c4b2ea3da8744a8ba750440a",
			SigningKey:  "icKfjErye3rMvliXR4IBNOu6ocrzzpSScKPQx9z9VBsd7zJtLvDbeANByeJh8EiQze7x+cmfbZC47cp9PPwJiA==",
			VssKey:      "WCECQoZjWJSu/6R74CC0ueh7cXmR0sasmTuCqf8X0BtAQ4o=",
		},
		"1fa56ba63cff50d124b6af42f33b245a30fcd1b0170d7704b0b201c7": {
			ExpiryEpoch: 5,
			Signature:   "7bb244c4fa1499021b0f2d36515a1f288a33cf00f1b88b57626998b439dcfb03ad88a7bc93101e4d83cdc75329799fbb2ccb28a7212a3e49737b06287d09b00c",
			SigningKey:  "YSYalbdhPua/IGfa13twNJcpsMUNV7wc8w3g20oec6iF0AVK98I/xsN5GdukHGAqV+LQ+TKaeVS4ZzONb7LJRQ==",
			VssKey:      "WCECNXeQRqiTZSPDDyeRJ3gl/QzYMLLtNH0yN+XOl17pu8Y=",
		},
		"5ffca3a329599727e39a7472c5270e54cf59a27b74306cc9f7fd0f5e": {
			ExpiryEpoch: 5,
			Signature:   "6cc8d84dd55b41efcf46c4b3086da1fb60c938182b4b66657650839d9fac1e2194a8253dc6d5c107ac0e9e714d1364fff9d2114eae07363d9937ee1d92b69c06",
			SigningKey:  "i1Mgdin5ow5LIBUETzN8AXNavmckPBlHDJ2ujHtzJ5gJiHufQg1vcO4enVDBYFKHjlRLZctdRL1pF1ayhM2Cmw==",
			VssKey:      "WCEDca27BxibVjQoA1QJaWx4gAE2MUB0lHfb6jJ3iorXD7s=",
		},
		"6bef444609d6e336cb1fe1daba278918dbc5768e6754c2945dd8d25c": {
			ExpiryEpoch: 5,
			Signature:   "2d96e4d4a4c506cc5762128b814ffb20afb97d30eb976334cd241a3935bd155ea1d68772b0903bde4584470359206769d83fa2ce55f56a1027ec3c52cb5e8703",
			SigningKey:  "6MA6A8Cy3b6kGVyvOfQeZp99JR7PIh+7LydcCl1+BdGQ3MJG9WyOM6wANwZuL2ZN2qmF6lKECCZDMI3eT1v+3w==",
			VssKey:      "WCED6k6ArqOnhQtfNRg0FSCxWmAtocZcyV33AdjMotjGwxI=",
		},
		"ce1e50f578d3043dc78d8777f5723cc7b6ca512d8cdbe8a09aafc9c3": {
			ExpiryEpoch: 5,
			Signature:   "2b830f1a79d2baca791a90c3784d74ec9f00267efac5ccd3cd7082b854234f411c237b59f34736933ba626fadc87fd6b2114c44486de692892d7401343990e01",
			SigningKey:  "8U9xLcYA15MFLUhC1QzvpOZYhOps+DcHB564zjAu/IXa6SLV6zg40rkXhPBIJNJnZ7+2W9NqNudP7EbQnZiFjQ==",
			VssKey:      "WCECs1+lg8Lsm15FxfY8bhGyRuwe8yOaSH0wwSajLRYeW/s=",
		},
		"eb649333a196ecb024a4a5919d3ce86084014136fd3e884e52ecd057": {
			ExpiryEpoch: 5,
			Signature:   "0b115a39935ce6008a4bbad0377f35463fd3510e282186ba43492768a02eb000bd4d3bc50799a24c53879ff2f2587179e797ee1c312acaf107cba67f91cb280b",
			SigningKey:  "X93u2t4nFNbbL54RBHQ9LY2Bjs3cMG4XYQjbFMqt1EG0V9WEDGD4hAuZyPeMKQriKdT4Qx5ni6elRcNWB7lN2w==",
			VssKey:      "WCECS11PWxybUHKY2hHmBgm/zYaR2YsqsH+f3uPOp2ydz/E=",
		},
	},
}

func TestGenesisFromJson(t *testing.T) {
	tmpGenesis, err := byron.NewByronGenesisFromReader(
		strings.NewReader(byronGenesisConfig),
	)
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if !reflect.DeepEqual(tmpGenesis, expectedGenesisObj) {
		t.Fatalf(
			"did not get expected object:\n     got: %#v\n  wanted: %#v",
			tmpGenesis,
			expectedGenesisObj,
		)
	}
}

func TestGenesisNonAvvmUtxos(t *testing.T) {
	testAddr := "FHnt4NL7yPXvDWHa8bVs73UEUdJd64VxWXSFNqetECtYfTd9TtJguJ14Lu3feth"
	testAmount := uint64(30_000_000_000_000_000)
	expectedTxId := "4843cf2e582b2f9ce37600e5ab4cc678991f988f8780fed05407f9537f7712bd"
	tmpGenesis := byron.ByronGenesis{
		NonAvvmBalances: map[string]string{
			testAddr: strconv.FormatUint(testAmount, 10),
		},
	}
	tmpGenesisUtxos, err := tmpGenesis.GenesisUtxos()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(tmpGenesisUtxos) != 1 {
		t.Fatalf("did not get expected count of genesis UTxOs")
	}
	tmpUtxo := tmpGenesisUtxos[0]
	if tmpUtxo.Id.Id().String() != expectedTxId {
		t.Fatalf(
			"did not get expected TxID: got %s, wanted %s",
			tmpUtxo.Id.Id().String(),
			expectedTxId,
		)
	}
	if tmpUtxo.Output.Address().String() != testAddr {
		t.Fatalf(
			"did not get expected address: got %s, wanted %s",
			tmpUtxo.Output.Address().String(),
			testAddr,
		)
	}
	if tmpUtxo.Output.Amount().Uint64() != testAmount {
		t.Fatalf(
			"did not get expected amount: got %s, wanted %d",
			tmpUtxo.Output.Amount().String(),
			testAmount,
		)
	}
}

func TestGenesisAvvmUtxos(t *testing.T) {
	testPubkey := "URVk8FxX6Ik9z-Cub09oOxMkp6FwNq27kJUXbjJnfsQ="
	testAmount := uint64(2463071701000000)
	expectedTxId := "0ae3da29711600e94a33fb7441d2e76876a9a1e98b5ebdefbf2e3bc535617616"
	expectedAddr := "Ae2tdPwUPEZKQuZh2UndEoTKEakMYHGNjJVYmNZgJk2qqgHouxDsA5oT83n"
	tmpGenesis := byron.ByronGenesis{
		AvvmDistr: map[string]string{
			testPubkey: strconv.FormatUint(testAmount, 10),
		},
	}
	tmpGenesisUtxos, err := tmpGenesis.GenesisUtxos()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if len(tmpGenesisUtxos) != 1 {
		t.Fatalf("did not get expected count of genesis UTxOs")
	}
	tmpUtxo := tmpGenesisUtxos[0]
	if tmpUtxo.Id.Id().String() != expectedTxId {
		t.Fatalf(
			"did not get expected TxID: got %s, wanted %s",
			tmpUtxo.Id.Id().String(),
			expectedTxId,
		)
	}
	if tmpUtxo.Output.Address().String() != expectedAddr {
		t.Fatalf(
			"did not get expected address: got %s, wanted %s",
			tmpUtxo.Output.Address().String(),
			expectedAddr,
		)
	}
	if tmpUtxo.Output.Amount().Uint64() != testAmount {
		t.Fatalf(
			"did not get expected amount: got %s, wanted %d",
			tmpUtxo.Output.Amount().String(),
			testAmount,
		)
	}
}

func TestGenesisAvvmUtxosRequireNetworkMagic(t *testing.T) {
	pubkey := "URVk8FxX6Ik9z-Cub09oOxMkp6FwNq27kJUXbjJnfsQ="
	pubkeyBytes, err := base64.URLEncoding.DecodeString(pubkey)
	require.NoError(t, err)
	magic := uint32(42)
	address, err := common.NewByronAddressRedeem(
		pubkeyBytes,
		common.ByronAddressAttributes{Network: &magic},
	)
	require.NoError(t, err)
	addressBytes, err := address.Bytes()
	require.NoError(t, err)
	genesis := byron.ByronGenesis{
		AvvmDistr: map[string]string{pubkey: "1000000"},
		ProtocolConsts: byron.ByronGenesisProtocolConsts{
			ProtocolMagic: int(magic),
		},
		RequiresNetworkMagic: "RequiresMagic",
	}
	utxos, err := genesis.GenesisUtxos()
	require.NoError(t, err)
	require.Len(t, utxos, 1)
	require.Equal(t, address.String(), utxos[0].Output.Address().String())
	require.Equal(
		t,
		addressBytes,
		mustAddressBytes(t, utxos[0].Output.Address()),
	)
}

func TestGenesisUtxosRejectAvvmNonAvvmOverlap(t *testing.T) {
	pubkey := "URVk8FxX6Ik9z-Cub09oOxMkp6FwNq27kJUXbjJnfsQ="
	pubkeyBytes, err := base64.URLEncoding.DecodeString(pubkey)
	require.NoError(t, err)
	magic := uint32(42)
	address, err := common.NewByronAddressRedeem(
		pubkeyBytes,
		common.ByronAddressAttributes{Network: &magic},
	)
	require.NoError(t, err)
	for _, amounts := range [][2]string{{"1000000", "1000000"}, {"1000000", "2000000"}} {
		genesis := byron.ByronGenesis{
			AvvmDistr: map[string]string{pubkey: amounts[0]},
			NonAvvmBalances: map[string]string{
				address.String(): amounts[1],
			},
			ProtocolConsts: byron.ByronGenesisProtocolConsts{
				ProtocolMagic: int(magic),
			},
			RequiresNetworkMagic: "RequiresMagic",
		}
		_, err := genesis.GenesisUtxos()
		require.ErrorContains(t, err, "duplicate Byron genesis UTxO reference")
	}
	genesis := byron.ByronGenesis{
		AvvmDistr: map[string]string{pubkey: "1000000"},
		NonAvvmBalances: map[string]string{
			"Ae2tdPwUPEZKQuZh2UndEoTKEakMYHGNjJVYmNZgJk2qqgHouxDsA5oT83n": "2000000",
		},
		ProtocolConsts: byron.ByronGenesisProtocolConsts{
			ProtocolMagic: int(magic),
		},
		RequiresNetworkMagic: "RequiresMagic",
	}
	utxos, err := genesis.GenesisUtxos()
	require.NoError(t, err)
	require.Len(t, utxos, 2)
}

func mustAddressBytes(t *testing.T, addr common.Address) []byte {
	t.Helper()
	data, err := addr.Bytes()
	require.NoError(t, err)
	return data
}

func TestNewByronGenesisFromReader(t *testing.T) {
	jsonData := `{
        "avvmDistr": { "addr1": "1000" },
        "blockVersionData": {
            "heavyDelThd": "1",
            "maxBlockSize": "2",
            "maxHeaderSize": "3",
            "maxProposalSize": "4",
            "maxTxSize": "5",
            "mpcThd": "6",
            "scriptVersion": 1,
            "slotDuration": "7",
            "softforkRule": {
                "initThd": "8",
                "minThd": "9",
                "thdDecrement": "10"
            },
            "txFeePolicy": {
                "multiplier": "11",
                "summand": "12"
            },
            "unlockStakeEpoch": "13",
            "updateImplicit": "14",
            "updateProposalThd": "15",
            "updateVoteThd": "16"
        },
        "ftsSeed": "seed",
        "protocolConsts": {
            "k": 1,
            "protocolMagic": 42,
            "vssMinTtl": 2,
            "vssMaxTtl": 10
        },
        "startTime": 100000,
        "bootStakeholders": { "stakeholder1": 1 },
        "heavyDelegation": {
            "key1": {
                "cert": "cert-val",
                "delegatePk": "delegate-pk",
                "issuerPk": "issuer-pk",
                "omega": 5
            }
        },
        "nonAvvmBalances": { "addr2": "2000" },
        "vssCerts": {
            "cert1": {
                "expiryEpoch": 5,
                "signature": "sig",
                "signingKey": "sign-key",
                "vssKey": "vss-key"
            }
        }
    }`

	expected := byron.ByronGenesis{}
	err := json.Unmarshal([]byte(jsonData), &expected)
	if err != nil {
		t.Errorf("Failed to unmarshal expected: %v", err)
	}

	reader := strings.NewReader(jsonData)
	result, err := byron.NewByronGenesisFromReader(reader)
	if err != nil {
		t.Errorf("Failed to decode: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Errorf(
			"ByronGenesis struct does not match expected.\nGot: %#v\nExpected: %#v",
			result,
			expected,
		)
	}
}

// TestNewByronGenesisFromReaderRejectsNonCanonicalEscapes covers the
// historical canonical-JSON escape grammar the Byron reference parses
// genesis with, which permits only \" and \\ inside a string. It checks
// each disallowed escape (\uXXXX, \/, \n, \r, \t, \b, \f) is rejected, the
// two allowed escapes still decode, and both an ordinary value and the
// full genesis fixture used elsewhere in this file are unaffected.
func TestNewByronGenesisFromReaderRejectsNonCanonicalEscapes(t *testing.T) {
	t.Run("ordinary protocolMagic succeeds", func(t *testing.T) {
		_, err := byron.NewByronGenesisFromReader(strings.NewReader(
			genesisWithParameter(t, []string{"protocolConsts", "protocolMagic"}, 42),
		))
		require.NoError(t, err)
	})

	t.Run("protocolM\\u0061gic is rejected", func(t *testing.T) {
		// The Unicode escape in the key decodes to the ASCII letter 'a',
		// so a standard JSON decoder maps this key onto the same field
		// as a literal "protocolMagic" -- exactly why the canonical-JSON
		// grammar must be enforced before schema decoding, not caught by
		// it.
		//
		// Built with a double-quoted Go string literal (not a raw
		// backtick string) so the doubled backslash below compiles down
		// to the single literal backslash this test needs in front of
		// the escape's hex digits.
		doc := "{\"protocolConsts\": {\"protocolM\\u0061gic\": 42}}"
		require.Contains(t, doc, "protocolM\\u0061gic")
		_, err := byron.NewByronGenesisFromReader(strings.NewReader(doc))
		require.Error(t, err)
	})

	t.Run("string value containing \\/ is rejected", func(t *testing.T) {
		_, err := byron.NewByronGenesisFromReader(
			strings.NewReader(`{"ftsSeed": "abc\/def"}`),
		)
		require.Error(t, err)
	})

	// "\\u0041" is a double-quoted Go string literal so the doubled
	// backslash compiles down to one literal backslash in front of
	// u0041, matching the other entries' single-backslash-escape shape.
	for _, escape := range []string{`\n`, `\r`, `\t`, `\b`, `\f`, "\\u0041"} {
		t.Run("rejects "+escape, func(t *testing.T) {
			_, err := byron.NewByronGenesisFromReader(
				strings.NewReader(`{"ftsSeed": "abc` + escape + `def"}`),
			)
			require.Error(t, err)
		})
	}

	t.Run("accepts escaped quote and backslash", func(t *testing.T) {
		_, err := byron.NewByronGenesisFromReader(strings.NewReader(
			genesisWithParameter(t, []string{"ftsSeed"}, "abc\"\\def"),
		))
		require.NoError(t, err)
	})

	t.Run("full genesis fixture still decodes", func(t *testing.T) {
		_, err := byron.NewByronGenesisFromReader(
			strings.NewReader(byronGenesisConfig),
		)
		require.NoError(t, err)
	})

	t.Run("does not block on a reader left open past the genesis value", func(t *testing.T) {
		// NewByronGenesisFromReader decodes exactly one JSON value, the
		// same as the encoding/json Decode it wraps, and must not read
		// to EOF: a caller's reader (a long-lived connection, a
		// multi-document stream) may never signal EOF at all.
		genesisJSON := []byte(byronGenesisConfig)
		r := io.MultiReader(
			bytes.NewReader(genesisJSON),
			blockForever{},
		)
		done := make(chan error, 1)
		go func() {
			_, err := byron.NewByronGenesisFromReader(r)
			done <- err
		}()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal(
				"NewByronGenesisFromReader blocked reading past the genesis value",
			)
		}
	})
}

func genesisWithParameter(t *testing.T, path []string, value any) string {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(byronGenesisConfig), &document); err != nil {
		t.Fatal(err)
	}
	if document == nil {
		t.Fatal("decoded genesis document is nil")
	}
	current := document
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok || next == nil {
			t.Fatalf("missing genesis object %q", key)
		}
		current = next
	}
	current[path[len(path)-1]] = value
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	return string(encoded)
}

func genesisWithoutField(t *testing.T, path []string) string {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal([]byte(byronGenesisConfig), &document); err != nil {
		t.Fatal(err)
	}
	current := document
	for _, key := range path[:len(path)-1] {
		next, ok := current[key].(map[string]any)
		if !ok {
			t.Fatalf("missing genesis object %q", key)
		}
		current = next
	}
	delete(current, path[len(path)-1])
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	return string(encoded)
}

func TestNewByronGenesisFromReaderRequiredFields(t *testing.T) {
	fields := [][]string{
		{"avvmDistr"},
		{"blockVersionData"},
		{"protocolConsts"},
		{"startTime"},
		{"bootStakeholders"},
		{"heavyDelegation"},
		{"nonAvvmBalances"},
		{"blockVersionData", "heavyDelThd"},
		{"blockVersionData", "maxBlockSize"},
		{"blockVersionData", "maxHeaderSize"},
		{"blockVersionData", "maxProposalSize"},
		{"blockVersionData", "maxTxSize"},
		{"blockVersionData", "mpcThd"},
		{"blockVersionData", "scriptVersion"},
		{"blockVersionData", "slotDuration"},
		{"blockVersionData", "softforkRule"},
		{"blockVersionData", "txFeePolicy"},
		{"blockVersionData", "unlockStakeEpoch"},
		{"blockVersionData", "updateImplicit"},
		{"blockVersionData", "updateProposalThd"},
		{"blockVersionData", "updateVoteThd"},
		{"blockVersionData", "softforkRule", "initThd"},
		{"blockVersionData", "softforkRule", "minThd"},
		{"blockVersionData", "softforkRule", "thdDecrement"},
		{"blockVersionData", "txFeePolicy", "multiplier"},
		{"blockVersionData", "txFeePolicy", "summand"},
		{"protocolConsts", "k"},
		{"protocolConsts", "protocolMagic"},
	}
	for _, path := range fields {
		name := strings.Join(path, ".")
		t.Run(name, func(t *testing.T) {
			_, err := byron.NewByronGenesisFromReader(strings.NewReader(
				genesisWithoutField(t, path),
			))
			require.ErrorContains(t, err, name)
		})
	}
}

func TestNewByronGenesisFromReaderIgnoresUnknownFields(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal([]byte(byronGenesisConfig), &document); err != nil {
		t.Fatal(err)
	}
	if document == nil {
		t.Fatal("decoded genesis document is nil")
	}
	document["futureExtension"] = true
	blockVersionData, ok := document["blockVersionData"].(map[string]any)
	if !ok || blockVersionData == nil {
		t.Fatal("missing blockVersionData object")
	}
	blockVersionData["futureParameter"] = "ignored"
	softforkRule, ok := blockVersionData["softforkRule"].(map[string]any)
	if !ok || softforkRule == nil {
		t.Fatal("missing softforkRule object")
	}
	softforkRule["futureRule"] = "ignored"
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	_, err = byron.NewByronGenesisFromReader(strings.NewReader(string(encoded)))
	require.NoError(t, err)
}

func TestNewByronGenesisFromReaderLovelacePortionBounds(t *testing.T) {
	parameters := [][]string{
		{"blockVersionData", "mpcThd"},
		{"blockVersionData", "heavyDelThd"},
		{"blockVersionData", "updateVoteThd"},
		{"blockVersionData", "updateProposalThd"},
		{"blockVersionData", "softforkRule", "initThd"},
		{"blockVersionData", "softforkRule", "minThd"},
		{"blockVersionData", "softforkRule", "thdDecrement"},
	}
	for _, path := range parameters {
		name := strings.Join(path, ".")
		t.Run(name, func(t *testing.T) {
			for _, value := range []string{"0", "1", "1000000000000000"} {
				_, err := byron.NewByronGenesisFromReader(strings.NewReader(
					genesisWithParameter(t, path, value),
				))
				require.NoErrorf(t, err, "%s = %s", name, value)
			}
			for _, value := range []string{"-1", "1000000000000001"} {
				_, err := byron.NewByronGenesisFromReader(strings.NewReader(
					genesisWithParameter(t, path, value),
				))
				require.ErrorContains(t, err, name)
			}
		})
	}
}

func TestNewByronGenesisFromReaderRejectsNegativeUnsignedProtocolParameters(
	t *testing.T,
) {
	parameters := [][]string{
		{"blockVersionData", "slotDuration"},
		{"blockVersionData", "maxBlockSize"},
		{"blockVersionData", "maxHeaderSize"},
		{"blockVersionData", "maxTxSize"},
		{"blockVersionData", "maxProposalSize"},
		{"blockVersionData", "updateImplicit"},
		{"blockVersionData", "txFeePolicy", "summand"},
	}
	for _, path := range parameters {
		name := strings.Join(path, ".")
		t.Run(name, func(t *testing.T) {
			_, err := byron.NewByronGenesisFromReader(strings.NewReader(
				genesisWithParameter(t, path, "-1"),
			))
			require.ErrorContains(t, err, name)

			_, err = byron.NewByronGenesisFromReader(strings.NewReader(
				genesisWithParameter(t, path, "0"),
			))
			require.NoErrorf(t, err, "%s = 0", name)
		})
	}
}

func TestNewByronGenesisFromReaderScriptVersionRange(t *testing.T) {
	path := []string{"blockVersionData", "scriptVersion"}
	for _, tc := range []struct {
		value   int
		wantErr bool
	}{
		{value: -1, wantErr: true},
		{value: 0},
		{value: 65535},
		{value: 65536, wantErr: true},
	} {
		t.Run(fmt.Sprint(tc.value), func(t *testing.T) {
			_, err := byron.NewByronGenesisFromReader(strings.NewReader(
				genesisWithParameter(t, path, tc.value),
			))
			if tc.wantErr {
				require.ErrorContains(t, err, "blockVersionData.scriptVersion")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNewByronGenesisFromReaderTxFeeSummandUpperBound(t *testing.T) {
	path := []string{"blockVersionData", "txFeePolicy", "summand"}
	for _, tc := range []struct {
		value   string
		wantErr bool
	}{
		{value: "45000000000000000"},
		{value: "45000000000000001", wantErr: true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			_, err := byron.NewByronGenesisFromReader(strings.NewReader(
				genesisWithParameter(t, path, tc.value),
			))
			if tc.wantErr {
				require.ErrorContains(t, err, "blockVersionData.txFeePolicy.summand")
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestGenesis_FtsSeed_EmptyObject(t *testing.T) {
	jsonData := `{
        "avvmDistr": { "addr1": "1000" },
        "blockVersionData": {
            "heavyDelThd": "1",
            "maxBlockSize": "2",
            "maxHeaderSize": "3",
            "maxProposalSize": "4",
            "maxTxSize": "5",
            "mpcThd": "6",
            "scriptVersion": 1,
            "slotDuration": "7",
            "softforkRule": {
                "initThd": "8",
                "minThd": "9",
                "thdDecrement": "10"
            },
            "txFeePolicy": {
                "multiplier": "11",
                "summand": "12"
            },
            "unlockStakeEpoch": "13",
            "updateImplicit": "14",
            "updateProposalThd": "15",
            "updateVoteThd": "16"
        },
        "ftsSeed": {},
        "protocolConsts": {
            "k": 1,
            "protocolMagic": 42,
            "vssMinTtl": 2,
            "vssMaxTtl": 10
        },
        "startTime": 100000,
        "bootStakeholders": { "stakeholder1": 1 },
        "heavyDelegation": {
            "key1": {
                "cert": "cert-val",
                "delegatePk": "delegate-pk",
                "issuerPk": "issuer-pk",
                "omega": 5
            }
        },
        "nonAvvmBalances": { "addr2": "2000" },
        "vssCerts": {
            "cert1": {
                "expiryEpoch": 5,
                "signature": "sig",
                "signingKey": "sign-key",
                "vssKey": "vss-key"
            }
        }
    }`
	got, err := byron.NewByronGenesisFromReader(strings.NewReader(jsonData))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FtsSeed.Value != "" || !got.FtsSeed.IsObject {
		t.Fatalf("ftsSeed not parsed as empty object; got=%#v", got.FtsSeed)
	}
}

func TestGenesisDelegateKeyHashes(t *testing.T) {
	// Use the mainnet-like test config which has real base64-encoded public keys
	genesis, err := byron.NewByronGenesisFromReader(strings.NewReader(byronGenesisConfig))
	require.NoError(t, err, "Failed to parse genesis")

	// Get genesis delegate key hashes
	keyHashes, err := genesis.GenesisDelegateKeyHashes()
	require.NoError(t, err, "GenesisDelegateKeyHashes failed")

	// Should have 7 delegates (matching the test config)
	assert.Len(t, keyHashes, 7, "Expected 7 key hashes")

	// Verify hashes are sorted (lexicographically by hex representation)
	for i := 1; i < len(keyHashes); i++ {
		assert.Less(t, keyHashes[i-1].String(), keyHashes[i].String(),
			"Key hashes not sorted at index %d", i)
	}

	// Verify each hash is 28 bytes (Blake2b-224)
	for i, h := range keyHashes {
		assert.Equal(t, 28, len(h.Bytes()), "Key hash %d has wrong length", i)
	}

	t.Logf("Genesis delegate key hashes (%d total):", len(keyHashes))
	for i, h := range keyHashes {
		t.Logf("  [%d] %s", i, h.String())
	}
}

func TestGenesisDelegateKeyHashes_Empty(t *testing.T) {
	// Genesis with no heavy delegation
	jsonData := `{
        "avvmDistr": {},
        "blockVersionData": {
            "heavyDelThd": "1",
            "maxBlockSize": "2",
            "maxHeaderSize": "3",
            "maxProposalSize": "4",
            "maxTxSize": "5",
            "mpcThd": "6",
            "scriptVersion": 1,
            "slotDuration": "7",
            "softforkRule": {"initThd": "8", "minThd": "9", "thdDecrement": "10"},
            "txFeePolicy": {"multiplier": "11", "summand": "12"},
            "unlockStakeEpoch": "13",
            "updateImplicit": "14",
            "updateProposalThd": "15",
            "updateVoteThd": "16"
        },
        "ftsSeed": "seed",
        "protocolConsts": {"k": 1, "protocolMagic": 42, "vssMinTtl": 2, "vssMaxTtl": 10},
        "startTime": 100000,
        "bootStakeholders": {},
        "heavyDelegation": {},
        "nonAvvmBalances": {},
        "vssCerts": {}
    }`
	genesis, err := byron.NewByronGenesisFromReader(strings.NewReader(jsonData))
	require.NoError(t, err, "Failed to parse genesis")

	keyHashes, err := genesis.GenesisDelegateKeyHashes()
	require.NoError(t, err, "GenesisDelegateKeyHashes failed")

	assert.Nil(t, keyHashes, "Expected nil for empty heavyDelegation")
}
