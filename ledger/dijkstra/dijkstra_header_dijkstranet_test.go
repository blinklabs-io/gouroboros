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

package dijkstra

import (
	"encoding/hex"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/stretchr/testify/require"
)

// dijkstraNetFirstHeaderHex is the first Dijkstra-era block header on
// DijkstraNet (network magic 6, slot 1296023, block 64739), served over
// node-to-node chain-sync by a cardano-node 11.2.0 relay. Its body carries the
// 10 Babbage header fields and no Leios extension.
const dijkstraNetFirstHeaderHex = "828a19fce31a0013c6975820f14caba43c84e11a6c79b1063ea4d00bcc02fb766e3212c06ab466e84fd2054158204dc1f97115d80c18f767686cbfe48ac4843fab1b3133c5e127f815f38de2daed5820aa48b6067fa7499e5950e49994e9fbd87bef3089216a8a09757070cabc0f3830825840f12d3b379239b57cd89e8d99138616a022543808c14d1bc8e65fa572d16e6a7b5160e3e52c9900f1564bc57c0a9f32ce7a0ca085a8a25416ce3c092345ce549d5850e0a95dc7789967fa9ddb31e2fe691636038189b56abea7d8145dac6ba3ef82a4ccf15d92c4541e368849ede2215e5813f4490010bb6ce0eb14ed7c71a77720beb1dc069940a91ee449ce5b92356f3705045820c93a1b1497883589d7ec8abb158c9b66c311ac8d45be74610bf889443daec9678458206e54419d9bdafc21186adbdf2be5cda85ccdb5db334e902d589d75ea9e48d73400005840955c4d6d06f4a0237d57ddba026598218d5e66441f7f07284237deb50bed119574836813cb953a0885e8c631213c2875fe222ecd366f20ffbf6dd20acde84605820c005901c00615a9b1dcaa7150ad3ef52f612cc9b0b1d23c072ee2404136a5b9c3fa9bf41485546f31129e7bf2c3d59ad9593add95a03d377cf2745c16032c654680c803073c05a816e5f2218032861e92e01d7915f08cd50e42763495741ac5a6d5c4e8171997255801d965c3908d82e79a7d276475b5ffdfba854b75f6ba3c02d5e2ade8c2334b6ccbcea54868174253fa09eb751f8abaa9d3c26f6761722f0590fd5486a57afb2be98c0091e38c4ada6f773726dedb1bb56771b1f4975890d8dfe47cb5876d5cd696b0644d18e884f6f3e09fd40991982c1c89d0e1247a9db40a2ad5bf589beec57e2cc5c733ffd40c92fa515293e32b693d14586926442542f1b12c09bffa1a678bc0220bd60cf963565cbc30850b0419ea4b09376eb4de994d848a06bbd4a6cffe2a2be15beb1402cb5c383fac894ee91a4d389c23011690d964884ff6a895779d6e8315c16180097d5858ed6c3e49d0fd1372edfb64a904047c323340e680fa3d32d3905c2066a94ff80cb2a120314fac199d2206f9ff79c19eb5a92de009e9616f244548f62f1d4d4859a5feacf4f39b3d21378dc007c6bdecd97292f7269213b6639237e82668b10c5e12b1e4079ac4061ddaa4f71364119a0a24"

func TestDijkstraBlockHeaderDecodesDijkstraNetTenFieldBody(t *testing.T) {
	raw, err := hex.DecodeString(dijkstraNetFirstHeaderHex)
	require.NoError(t, err)
	var header DijkstraBlockHeader
	_, err = cbor.Decode(raw, &header)
	require.NoError(t, err)
	require.Equal(t, uint64(1296023), header.SlotNumber())
	require.Equal(t, uint64(64739), header.BlockNumber())
	require.Equal(
		t,
		"f14caba43c84e11a6c79b1063ea4d00bcc02fb766e3212c06ab466e84fd20541",
		header.PrevHash().String(),
	)
	require.Nil(t, header.LeiosHeaderExtension)
	_, present := header.LeiosCertified()
	require.False(t, present)
	_, _, ok := header.LeiosAnnouncement()
	require.False(t, ok)
	require.Equal(t, uint64(12), header.Body.ProtoVersion.Major)

	out, err := header.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, raw, out)
	require.Equal(t, common.Blake2b256Hash(raw), header.Hash())
}

// dijkstraNetFirstBlockHex is the full first Dijkstra-era DijkstraNet block
// (slot 1296023) served by block-fetch from a cardano-node 11.2.0 relay.
const dijkstraNetFirstBlockHex = "82828a19fce31a0013c6975820f14caba43c84e11a6c79b1063ea4d00bcc02fb766e3212c06ab466e84fd2054158204dc1f97115d80c18f767686cbfe48ac4843fab1b3133c5e127f815f38de2daed5820aa48b6067fa7499e5950e49994e9fbd87bef3089216a8a09757070cabc0f3830825840f12d3b379239b57cd89e8d99138616a022543808c14d1bc8e65fa572d16e6a7b5160e3e52c9900f1564bc57c0a9f32ce7a0ca085a8a25416ce3c092345ce549d5850e0a95dc7789967fa9ddb31e2fe691636038189b56abea7d8145dac6ba3ef82a4ccf15d92c4541e368849ede2215e5813f4490010bb6ce0eb14ed7c71a77720beb1dc069940a91ee449ce5b92356f3705045820c93a1b1497883589d7ec8abb158c9b66c311ac8d45be74610bf889443daec9678458206e54419d9bdafc21186adbdf2be5cda85ccdb5db334e902d589d75ea9e48d73400005840955c4d6d06f4a0237d57ddba026598218d5e66441f7f07284237deb50bed119574836813cb953a0885e8c631213c2875fe222ecd366f20ffbf6dd20acde84605820c005901c00615a9b1dcaa7150ad3ef52f612cc9b0b1d23c072ee2404136a5b9c3fa9bf41485546f31129e7bf2c3d59ad9593add95a03d377cf2745c16032c654680c803073c05a816e5f2218032861e92e01d7915f08cd50e42763495741ac5a6d5c4e8171997255801d965c3908d82e79a7d276475b5ffdfba854b75f6ba3c02d5e2ade8c2334b6ccbcea54868174253fa09eb751f8abaa9d3c26f6761722f0590fd5486a57afb2be98c0091e38c4ada6f773726dedb1bb56771b1f4975890d8dfe47cb5876d5cd696b0644d18e884f6f3e09fd40991982c1c89d0e1247a9db40a2ad5bf589beec57e2cc5c733ffd40c92fa515293e32b693d14586926442542f1b12c09bffa1a678bc0220bd60cf963565cbc30850b0419ea4b09376eb4de994d848a06bbd4a6cffe2a2be15beb1402cb5c383fac894ee91a4d389c23011690d964884ff6a895779d6e8315c16180097d5858ed6c3e49d0fd1372edfb64a904047c323340e680fa3d32d3905c2066a94ff80cb2a120314fac199d2206f9ff79c19eb5a92de009e9616f244548f62f1d4d4859a5feacf4f39b3d21378dc007c6bdecd97292f7269213b6639237e82668b10c5e12b1e4079ac4061ddaa4f71364119a0a248380f6f6"

func TestDijkstraBlockDecodesDijkstraNetFirstBlock(t *testing.T) {
	raw, err := hex.DecodeString(dijkstraNetFirstBlockHex)
	require.NoError(t, err)
	block, err := NewDijkstraBlockFromCbor(raw)
	require.NoError(t, err)
	require.Equal(t, uint64(1296023), block.SlotNumber())
	require.Equal(
		t,
		"060b416fd53c32e94b6e45a2b03dd01dcdc914702cefe8993d4a205f471a476a",
		block.Hash().String(),
	)
	require.Equal(t, block.BlockBodyHash(), block.CalculatedBlockBodyHash())
}

// TestDijkstraBlockHeaderReencodesDijkstraNetTenFieldBody verifies that a
// 10-field header re-encoded without its retained wire bytes (as for a header
// built in-process) is the 10-field body again, byte for byte, and that a nil
// LeiosHeaderExtension never gains the Leios prototype's two fields.
func TestDijkstraBlockHeaderReencodesDijkstraNetTenFieldBody(t *testing.T) {
	raw, err := hex.DecodeString(dijkstraNetFirstHeaderHex)
	require.NoError(t, err)
	var src DijkstraBlockHeader
	_, err = cbor.Decode(raw, &src)
	require.NoError(t, err)

	h := DijkstraBlockHeader{BabbageBlockHeader: src.BabbageBlockHeader}
	h.SetCbor(nil)
	require.Nil(t, h.Cbor())
	require.Nil(t, h.LeiosHeaderExtension)
	out, err := h.MarshalCBOR()
	require.NoError(t, err)
	require.Equal(t, raw, out)

	var top []cbor.RawMessage
	_, err = cbor.Decode(out, &top)
	require.NoError(t, err)
	require.Len(t, top, 2)
	var body []cbor.RawMessage
	_, err = cbor.Decode(top[0], &body)
	require.NoError(t, err)
	require.Len(t, body, babbageHeaderBodyFieldCount)

	var back DijkstraBlockHeader
	_, err = cbor.Decode(out, &back)
	require.NoError(t, err)
	require.Nil(t, back.LeiosHeaderExtension)
	require.Equal(t, common.Blake2b256Hash(raw), back.Hash())
}
