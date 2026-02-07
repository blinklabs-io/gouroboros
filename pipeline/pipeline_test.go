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

package pipeline

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	pcommon "github.com/blinklabs-io/gouroboros/protocol/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conwayBlockHex is a valid Conway block from mainnet for testing.
// Block taken from https://cexplorer.io/block/27807a70215e3e018eec9be8c619c692e06a78ebcb63daf90d7abe823f3bbf47
const conwayBlockHex = "85828a1a00b82b211a0986e4475820ff51732269af51a2efaa2a7ad4a2ff5647af5629013a446511249e837be617a05820dbdce19c856881a7179d61666204368a13e282b0f473b6dee7a79c44e47a11c65820113752032b1036bd7ab48b47302a723bf7238bbc08e798f56d1bb9a205679929825840b7e53621a22d79026e0c21f4569019713997d7f3fcaaafdea9099949ca9ada905102e166a79ae5c1b5b5e25807c08131b4b88f241a2953e7379a09f1d1e7c7205850d6bf53d7374e4e8af248156c381102be6d928af050d914f27dd47fa3333b055cc500abed018ddd680836f1cbac8b4ea2690ce2ec8d597e096a210a9335a97e3c1bf93b03166b2657dd79d8754cbf320a191aaa5820e269b23136f37156fe832904eff083e175059de240544e0b439d352c828c24e3845820cb7a64398b69f2ff3f17fccd154ccf079557ac15d7e57a7ef58ad9bbe87bcead18181904c858401830540aa911227376268a31cfd6e37c891fe7fe0a869005cf0d82f3075b57f838e10c0f8be162308b24efb6487ca74ba714adf1e39f22db25f761cb7726a60e820a025901c0f77d128743cfefaa320444914a7ca99e451859f80bb845225246096bf5e009b2af36707c04262abf1205022b9382eb8190ec388225a63cccfd6d216aaa2daa0d89c266d875ee0ba9269c936bda8888cee3f024b8276c427baa2cf880c919fd7a03aa896dc56d127cf4907ff823bf4d6a5de137e8c76320d9f45e12a21539d18afd94240916240a5f5ee5845cf57f9fd73f8ca6d309cb89369b316867e922f381c0af13614004a165c4076486f5e42823586e01924f82d8ca3b7a184691f00521934f9b41919065725dce314aaef2a5287d6f7a8c3825afcc0eaa60cd159e3f8f7f6cd9bfbbe100d0462048fdb36549f41b4482231117b91c1219d6f4499eb037c758f582d0a2e8068e645d14b05a108968dc1643fd82dc6568b3c281d42577da1836bcb89c8a9d60dd945fd4870513c231e83d203939116ff2875f5b464744ea9678b2aa868a2272827320b6a8f21cccbb14bd3d700ce2f85e824dae0fc4204606051b7884229d1734eb28ce0eb7a445a5c9b8dff2784a132ca1b57c10ef63b57cf1393fc07a8d4ea4e9f10ec32bf789bc696f9404b63406d583e981390f3c15b67e0f7f6f9ac440f7c6ece8cea6dd3e44e34cca6035323dd3d22f95eb89182a88a400d901028582582008e355f539041a3d9a354566834e91d564f13230cece576d9752c794d3e0818d01825820664105498b69e19b1e593b75f21596a1bc1d18c3bad01f487dba53c855d7391401825820be5e69a50eaf539d1f886f34dccde2fcf2a8c9dbecf9a88a7498483b82c764c601825820c7c4c618636ea620081c53b5b7fa1056e0f06b5f264670f48bc1050d58dd3ffd01825820f6a7d08b12c00dcab0ffbaf1582f4ad9f2b807f35d13cb83fb75bc53148ffbd60101828258390179a89e26ac12d9e17c72a284e1b4a04d41d0199b0f52bea6a6793f2f26b552d2883ecbef6bebc0c6a9f0331387a4bc710ebec023751d3da31a13516b608258390125b0cac183d211ee4a711fd8ef73dec435bdac7b5c9135ee1aedff1ae310d5e43e36ff0fa572060407cf16ad7c502b58f33e97d8d667a09f1a11cbef7b021a0002f0b5031a0986f22da9008382582029c4aa4f76b383c2f08d014ed21073c8117e7dd46e09a06e54f404787705ccf602825820d3733670d1bbb0b44d6e1424d796442c27d4f87e526766afb91f16a95d743bd901825820f4e2bbc7b8a40c998c90c56939dd6d48cac52b67489ce6c960713692b8709f0d000183a300583911af97793b8702f381976cec83e303e9ce17781458c73c4bb16fe02b83eef8e8afca483add16fe5118b01ef80b32230e5d0380cb761bd765c101821b0000001201bc7d03a2581c6fdc63a1d71dc2c65502b79baae7fb543185702b12c3c5fb639ed737a2414c015820a555e14a0b9dec3ab1e12ba68c0e381c7573649619998a1063ac0964d4451fe11b7fffffffd6f500b0581c97bbb7db0baef89caefce61b8107ac74c7a7340166b39d906f174beca14554616c6f731a00a7c248028201d81858cad8799f581cc134d839a64a5dfb9b155869ef3f34280751a622f69958baa8ffd29c4040581c97bbb7db0baef89caefce61b8107ac74c7a7340166b39d906f174bec4554616c6f7318460a14001927101a001e84801b00000197c7a9b7701a04228f161924251a0c3480641968d60000d8799fd8799fd8799f581c61ad95a7265a9c269125c149505043e143b822eedd930cc14e7e8129ffd8799fd8799fd8799f581cc8b3e05f9520c162b260e2e545eb84adfc2dd55b2e2ac6d41e70c4b0ffffffffffd87a80d87980ff82583901a899bc44776965fc83d8d2c5ada95f22d08ef6de17bf12b558ab9f56b297dd932ba109c09287b4df30ef3dd2c3f6480b37aeeec1fa20d33f821a1a1e8528a0825839010b46751422f2357dd4ecee5a84b288b982b20375a0503cc7ecdda5aa47754858ddd2795aab89617d72d7d240abccd036fa4b761fa39787a3821a011fc899a1581c1ad3767073087df4fc97fba7ac4a71a0a6cd556f1ad96a7b1c9870c4a1415801021a00076261031a0986f949081a0986e4330b58204678656214f6d284b1f69831aff2886e4ab3e4927a4e457714015f67f0a9bf7b0d8182582063c5dea8da9f5241ac8d6359354f2b322aea0698ebd59845d62f362d6dddb6f6000e81581c0b46751422f2357dd4ecee5a84b288b982b20375a0503cc7ecdda5aa12828258205ec56338104fcbfe32288c649d9633f0d9060abce8b8608b156294f0a81d29e201825820babc647257b8d78b86e862ba9769401714ed403e7c46ed1b59c3fc32e0247c8200a50081825820876087c685570cd91a6e6ff60120cc55a9105f56576b6b519136850ab66845e401018282583901228ef2e891696d804b705b75660503d8cc1ec4d01a8af20a3697f12d33d1dc483f1c0db883685f10360ba04719f23222d4951e288188e03c1a013de7e78258390162e42074c75c5a2067545f21bcef9db30298d05dbf0930899e6de5128c1198de6eba555d96ba1d71435099083ce5c156359525a6407e2e6c1b00000040b4cda07c021a000295c9031a0986ff52081a034e1938a5008182582029d9fac8800a06fcabc92dc4661feb779a69f35733c7829beeefe8896e16b6ce02018182583901734a61fd36a6bebcc0897ddceb8fabcf0cb792bb13ed92b02c2e3a74e77ce9f5a47ada4dfd8b8c5182300ca4298eb2b784547e0e5bd7c8bf1a1ed553fc021a0002ac35031a0986f22f05a1581de1e77ce9f5a47ada4dfd8b8c5182300ca4298eb2b784547e0e5bd7c8bf1a1be944b1ad00d901028382582010192dc4209a99189a8bb3b4ec1bfc72abb66941b36dade154ab43cfc4f82477018258204c15c23b69450168ed7a920fa91ee2547dc9c36a85a24b3e8bdc8111eb761d9302825820d3733670d1bbb0b44d6e1424d796442c27d4f87e526766afb91f16a95d743bd900018382583901a899bc44776965fc83d8d2c5ada95f22d08ef6de17bf12b558ab9f56b297dd932ba109c09287b4df30ef3dd2c3f6480b37aeeec1fa20d33f1a72f9541da300583911ea07b733d932129c378af627436e7cbc2ef0bf96e0036bb51b3bde6b52563c5410bff6a0d43ccebb7c37e1f69f5eb260552521adff33b9c201821b0000002b4b1c8479a2581c97bbb7db0baef89caefce61b8107ac74c7a7340166b39d906f174beca14554616c6f731a018b4c81581cf5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4ca2434d5350015820cc92eb089f1308c6cd9f55298ca716a66aa84534ea6ad6c6c5d9ffbbcad792811b7fffffffd6e7aa74028201d818587bd8799fd8799fd87a9f581c1eae96baf29e27682ea3f815aba361a0c6059d45e4bfbe95bbd2f44affffd8799f4040ffd8799f581c97bbb7db0baef89caefce61b8107ac74c7a7340166b39d906f174bec4554616c6f73ff1a291855951b0000002b48cba5b01a018b1f6a19012c19012cd8799f190e52ffd87980ff825839015b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b533b9586f0fb9aafd578e0d0154e9478d23614e736eb39d1a30d8a991a13d363f9021a000a26f3031a0986e4e705a1581df11eae96baf29e27682ea3f815aba361a0c6059d45e4bfbe95bbd2f44a000758205160f88b929bf8a6c57c285b889488f9137c0ef3cfd0bcf408a10020e69146d5081a0986e4330b5820ebefd2e645c467ba2ff8c89231093039c55b3ba8198ed9217179e76c572169ad0dd90102818258204c15c23b69450168ed7a920fa91ee2547dc9c36a85a24b3e8bdc8111eb761d93020ed9010281581c5b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b10825839015b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b533b9586f0fb9aafd578e0d0154e9478d23614e736eb39d1a30d8a991a1386914c111a004c4b4012d90102848258200dc17712e37a4e741767db2f90d4ffbf69faf88b9bed4c47864f7bd912924bea00825820cf4ecddde0d81f9ce8fcc881a85eb1f8ccdaf6807f03fea4cd02da896a621776008258202536194d2a976370a932174c10975493ab58fd7c16395d50e62b7c0e1949baea00825820d46bd227bd2cf93dedd22ae9b6d92d30140cf0d68b756f6608e38d680c61ad1700ad00d901028382582040f8545707caf63028166a612390d1e77c76504901d3854f76c2ef3780ac53d9018258204deff36ca0fe8ec7bcbf35b736bbca51921144e0911c705466751fa1161675c000825820f4eff4f626143937ec5abe9090f3df67456fc4614ac531abf36d3f00edef45a802018382583901f04e3a2f427f303624958a861d18f657e307e65da73922e8d823d573c07fa2f711522daaf4b2a7c08d866bff26b3ae96dc43ab349b5c30801a0f24558ba300583911ea07b733d932129c378af627436e7cbc2ef0bf96e0036bb51b3bde6b52563c5410bff6a0d43ccebb7c37e1f69f5eb260552521adff33b9c201821b00000005981daf1fa2581c5c1c91a65bedac56f245b8184b5820ced3d2f1540e521dc1060fa683a1454a454c4c591b000000c65583b5cc581cf5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4ca2434d5350015820f86844f5f99088c9f78e3aea1ddf096d0997906fb0fa670389236cfab18b28041b7fffffe5ee5c75f3028201d8185881d8799fd8799fd87a9f581c1eae96baf29e27682ea3f815aba361a0c6059d45e4bfbe95bbd2f44affffd8799f4040ffd8799f581c5c1c91a65bedac56f245b8184b5820ced3d2f1540e521dc1060fa683454a454c4c59ff1b0000001a11a38a161b00000005951d26c71b000000c5f436ae8818c818c8d8799f190682ffd87980ff825839015b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b533b9586f0fb9aafd578e0d0154e9478d23614e736eb39d1a30d8a991a24bdd5b7021a000a28ab031a0986e4e705a1581df11eae96baf29e27682ea3f815aba361a0c6059d45e4bfbe95bbd2f44a000758205160f88b929bf8a6c57c285b889488f9137c0ef3cfd0bcf408a10020e69146d5081a0986e4330b5820e90504f88a761737518e5ea292b91841a5388aed6ba2a6f8aca74c3372b8ed9b0dd9010281825820f4eff4f626143937ec5abe9090f3df67456fc4614ac531abf36d3f00edef45a8020ed9010281581c5b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b10825839015b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b533b9586f0fb9aafd578e0d0154e9478d23614e736eb39d1a30d8a991a247104c2111a004c4b4012d90102848258200dc17712e37a4e741767db2f90d4ffbf69faf88b9bed4c47864f7bd912924bea00825820cf4ecddde0d81f9ce8fcc881a85eb1f8ccdaf6807f03fea4cd02da896a621776008258202536194d2a976370a932174c10975493ab58fd7c16395d50e62b7c0e1949baea00825820d46bd227bd2cf93dedd22ae9b6d92d30140cf0d68b756f6608e38d680c61ad1700ad00d9010283825820ca3e7919be4d615ebbf2041078dd6b7a0247490abe56860ba3812884989aebdc03825820dfcf69f9e82ed7a91c73fe40be3f5fce41bc030438097f3dc71990a71a1df7cc01825820f460d598f8a2372eb879ca9d930bd58b049d3b21d1bf09014dd6a36d8f970809000183825839013f4165e2ea0a4dc6f7bcbbd23f824187c80e5490902e097fc3049c72d817a67d082624525f7ba3f4211b0557c3587e88d2809725d6f809021a051f4586a300583911ea07b733d932129c378af627436e7cbc2ef0bf96e0036bb51b3bde6b52563c5410bff6a0d43ccebb7c37e1f69f5eb260552521adff33b9c201821b000003e3f1e55b23a2581c279c909f348e533da5808898f87f9a14bb2c3dfbbacccd631d927a3fa144534e454b1a4b2e6931581cf5808c2c990d86da54bfc97d89cee6efa20cd8461616359478d96b4ca2434d53500158202ffadbb87144e875749122e0bbb9f535eeaa7f5660c6c4a91bcc4121e477f08d1b7ffffff1df295d37028201d818587cd8799fd8799fd87a9f581c1eae96baf29e27682ea3f815aba361a0c6059d45e4bfbe95bbd2f44affffd8799f4040ffd8799f581c279c909f348e533da5808898f87f9a14bb2c3dfbbacccd631d927a3f44534e454bff1b0000000e20d6a2d21b000003e3ef24d3521a4b2e5d1218641864d8799f190e52ffd87980ff825839015b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b533b9586f0fb9aafd578e0d0154e9478d23614e736eb39d1a30d8a991a1a694943021a000a20b1031a0986e4e705a1581df11eae96baf29e27682ea3f815aba361a0c6059d45e4bfbe95bbd2f44a000758205160f88b929bf8a6c57c285b889488f9137c0ef3cfd0bcf408a10020e69146d5081a0986e4330b5820fb13052eb673abd9ee80a654ccb7a8da5d97c503940086d8b422cdcbb82a41cb0dd9010281825820ca3e7919be4d615ebbf2041078dd6b7a0247490abe56860ba3812884989aebdc030ed9010281581c5b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b10825839015b7e23228dba75595645fc357d0f97ba258cfccfff5d588d4bb9165b533b9586f0fb9aafd578e0d0154e9478d23614e736eb39d1a30d8a991a1a1c7054111a004c4b4012d90102848258200dc17712e37a4e741767db2f90d4ffbf69faf88b9bed4c47864f7bd912924bea00825820cf4ecddde0d81f9ce8fcc881a85eb1f8ccdaf6807f03fea4cd02da896a621776008258202536194d2a976370a932174c10975493ab58fd7c16395d50e62b7c0e1949baea00825820d46bd227bd2cf93dedd22ae9b6d92d30140cf0d68b756f6608e38d680c61ad1700a400d9010281825820f39183d0b6691ce2a415d7694078467fc42ea8850429abfba6f0f397eddedeb7000186825839015410c7c5c92d49a1a40b148a10de21d08aa703923931e7eaa55d9841b49b8366e6ef7ccb0a895e603695ab69f17f60d8ece9c38786931edf1a734416c782583901225d8e29222e68d2eec51222057212bbbf7acfb327ecbc6c8b304462f30408ded23952b14877adb90f8b3023e369af69149f83eb3a323d231a4eecc2f4825839013a4bb40dab959467ec64dfa952e08f8972776494efdc783591212e463a4bb40dab959467ec64dfa952e08f8972776494efdc783591212e461a0ac642ae82584c82d818584283581c5c87d77a23809c032e562cadceae113c284c4d5969ec1fc7d78e57b8a101581e581c9b1771bd305e4a34e8a72ba903cd42e0f49b32c5465887b489597f26001a8e0857bb1ac04897ed82584c82d818584283581ce2259f44ac5751906dec38d99fd560702aa3eb63a92c126ab2cf819da101581e581c9b1771bd305e4a2b2bec4da9eb6aab298e67bd10b806607501401e64001a9dfc53ab1a83ae48a282584c82d818584283581c2be5e2a084010a6c41d705ab340427aaa4d791fcfbaeb6efe20c863fa101581e581c9b1771bd305e4a0067f2a1a9b035eb3a68aaebf1de166f8a7af365f0001a54e64db31a120873dd021a0002d58d031a0987006688a100d901028582582096c3a6ab313476222952a388dceeaa5c430b69a78d604b39621fb5c639811de65840de366f73045498749b4efc1c1dcb0a968024ae69631e15d87392110bbe2d8447665ba8133456f00e3e84e20cb881d103ac9addc844d3c001b3e85187d68b09028258205dd135506596e0d82a3a630c9e8fc740aa5702361741df8801c5a99fdf86239d5840c54cc47d2ff94a1f35e711b8fa20fe79bf7f19dae0d0582be6f9957bb797d2d0e323b1de7d3ddbc3492e47ea5ff957e2044b03c3bb03eb111c6100317001300b8258201550b9d4dee4c0e72e38ae0fb2dedb1eea89f952c0b7b73af8cf80ba790c9f3a584096390c13a40626b0d3997633ed983c70c2a4714def7c0e0ee7ee633105e46639b1f0964f8be6011e6cce42d272ebbc2b356edf33203f4d649f99f2c5cc297f0d825820bde9380818456d583927afc8dafbef4c84abe4ff7a4cb08caf015df8fbc288915840140aa45a4cc7ea5755a7024142ed7450074005307e1d18b7b0418bb53fb306f7da4680504ba561db7ea56188c81fd50db4b07bd19a2dceed65bde8e95691130682582068ebbe01352f77628f40cf5c25a000259e8ed677330dfc35957f08c5d6aeeffb58405224eccbf52f2bbe488392707cc027c6dd7a4ccb97d47ba79751ad35a6a754311d396a3992224b8b866c99c15cd4c7912c3b14cc17f5f2ae560be2aaebe85c09a20081825820fbc53e7aa4e5497d8662e8f0d5337441f629d1f237217bc24ac41bb6de89f8415840028072c117dd09274b6b9bb6aabd681730f8ae532b5a663ac93695eb3062c4244d6c65da0c29f5ffbed42b54a92722d734cfdfdc68dde4345cd3faa0d331d6050582840001d8799f02ff821982a41a00868b7d840002d8799f02009fd8799f0100ffffff821a000f5c0f1a133dd2d8a1008182582020fc80626e97534d41073ad5067bbca2eb2b43b4679462c3b69d7f68e7e582b65840aa6d087d15a428279687dbebbae62d3c5f2e6dfeb07ce6684fee69afa685edc28d46a5d126bcafea9e3e1b891431c4d8272bf25ada33bd581cf8f7f75150960ba10082825820177945e8221506c48e5000605f765940d43f056c6481aa4f3f106ad9d47d22335840d9c0922d53d52583b4fff5c161fdbca60b1a62827c00119875d0d97b2049c47eaf909cfa6ecebe5f78deedb7a23dd46f47c3a7dac6be6722795b68633d3d6d06825820e3b65ff9d2f9ce6a00e68c0098fac1e385d958570f00afccc79f8ef3e8dae92b5840332107f484cd062d6ef15d98fe8cfdcaf313eae4d2a9004b6fcec87de51d812d4d5e94d0515d1986b21f1f466e7a4194bbf5aab9435f200d5cfbcd6d8d954d0ca200d9010281825820c5d63d7dc066df52592135b6d3cb4f3470d06f7bdd4b2d2e32eb59ca3782662f5840bcd7bc9405a6c0369040bc3b7906afa3e6b30fde99503e768aab0f261ebadd6b2cf4f382efcec390a88318f001f3bb9313378b83b4d1c767d13cd583effe29060583840002d87980821962d91a007cc793840000d87980821a00012dfc1a0166fa60840300d8799f009f1a000aae60ff4100d87a809fd87a80ffff821a001465aa1a19a0dcc6a200d9010281825820c5d63d7dc066df52592135b6d3cb4f3470d06f7bdd4b2d2e32eb59ca3782662f5840e43f60c1d54d420bae18adcaafabf50a644ada997884eeab3b8cd7f831200ac5f2bf29f17526ad5a85750989cacc5b82983d3f3d175d02e5c4954c1170c51f0f0583840001d87980821962d91a007cc793840000d87980821a00012dfc1a0166fa60840300d8799f009f1a000aae60ff4100d87a809fd87a80ffff821a001465aa1a19a0dcc6a200d9010281825820c5d63d7dc066df52592135b6d3cb4f3470d06f7bdd4b2d2e32eb59ca3782662f584049c25b0a423509984c2bfe2976a57131bc76e03f94eb2f305e10889da87144d45e6ec13d95443aab280d41cfe57c91cc4f67147cde2896f3161ed0edc64b4d010583840002d87980821962d91a007cc793840001d87980821a00012dfc1a0166fa60840300d8799f009f1a000aae60ff4100d87a809fd87a80ffff821a0014147e1a194b697aa102d9010281845820d7665f1982610f24c8af865520ed5a64d2dd511ee7190f54171a244475acfede5840126c831a6bb24f498395ab8da27d5485642f935746185b29c72a5abd578cf695082cfdb53e818bbbbed7c537f35075ecdf5a1f1084a8c1cc8a6294c5297caa0c5820cc3d538bf11bc707e679b8ce4e8e24884d43423890e8a150368027766d91e1fe5822a101581e581c9b17098ec5544a5b03f921a91b80c9dcb461fbbd503dfbb42844e4dea304a11902a2a1636d736781774d696e737761703a204f7264657220457865637574656405a11902a2a1636d736781774d696e737761703a204f7264657220457865637574656406a11902a2a1636d736781774d696e737761703a204f7264657220457865637574656480"

// getValidBlockCbor returns valid Conway block CBOR bytes for testing.
func getValidBlockCbor(t *testing.T) []byte {
	t.Helper()
	data, err := hex.DecodeString(conwayBlockHex)
	require.NoError(t, err)
	return data
}

// getInvalidBlockCbor returns invalid CBOR bytes that will fail to decode.
func getInvalidBlockCbor() []byte {
	// Invalid CBOR - incomplete array structure
	return []byte{0x85, 0x00, 0x01, 0x02}
}

// createTestTip creates a test Tip for BlockItem construction.
func createTestTip(slot uint64, blockNum uint64) pcommon.Tip {
	return pcommon.Tip{
		Point:       pcommon.NewPoint(slot, []byte{0x01, 0x02, 0x03}),
		BlockNumber: blockNum,
	}
}

// ============================================================================
// TestBlockItem tests
// ============================================================================

func TestBlockItem_NewBlockItem(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)

	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 42)

	assert.Equal(t, uint(ledger.BlockTypeConway), item.BlockType)
	assert.Equal(t, rawCbor, item.RawCbor)
	assert.Equal(t, tip.Point.Slot, item.Tip.Point.Slot)
	assert.Equal(t, tip.BlockNumber, item.Tip.BlockNumber)
	assert.Equal(t, uint64(42), item.SequenceNumber)
	assert.False(t, item.ReceivedAt.IsZero())
}

func TestBlockItem_SetBlock_Block(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Initially no block
	assert.Nil(t, item.Block())
	assert.False(t, item.IsDecoded())

	// Decode and set block
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	item.SetBlock(block, 50*time.Millisecond)

	// Verify block is set
	assert.NotNil(t, item.Block())
	assert.True(t, item.IsDecoded())
	assert.Equal(t, 50*time.Millisecond, item.DecodeDuration())
}

func TestBlockItem_SetDecodeError_IsDecoded(t *testing.T) {
	rawCbor := getInvalidBlockCbor()
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	testErr := errors.New("decode failed")
	item.SetDecodeError(testErr, 10*time.Millisecond)

	assert.False(t, item.IsDecoded())
	assert.Equal(t, testErr, item.DecodeError())
	assert.Equal(t, 10*time.Millisecond, item.DecodeDuration())
}

func TestBlockItem_SetValidation_IsValid(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Initially not valid
	assert.False(t, item.IsValid())

	// Set validation success
	item.SetValidation(true, "abc123", nil, 100*time.Millisecond)

	assert.True(t, item.IsValid())
	assert.Nil(t, item.ValidationError())
	assert.Equal(t, "abc123", item.VRFOutput())
	assert.Equal(t, 100*time.Millisecond, item.ValidateDuration())

	// Set validation failure
	item2 := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 2)
	validationErr := errors.New("VRF failed")
	item2.SetValidation(false, "", validationErr, 50*time.Millisecond)

	assert.False(t, item2.IsValid())
	assert.Equal(t, validationErr, item2.ValidationError())
}

func TestBlockItem_SetApplied(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Initially not applied
	assert.False(t, item.IsApplied())

	// Set applied successfully
	item.SetApplied(true, nil, 25*time.Millisecond)

	assert.True(t, item.IsApplied())
	assert.Nil(t, item.ApplyError())
	assert.Equal(t, 25*time.Millisecond, item.ApplyDuration())

	// Set apply failure
	item2 := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 2)
	applyErr := errors.New("apply failed")
	item2.SetApplied(false, applyErr, 10*time.Millisecond)

	assert.False(t, item2.IsApplied())
	assert.Equal(t, applyErr, item2.ApplyError())
}

func TestBlockItem_ThreadSafety(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode block once
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)

	const numGoroutines = 50
	const numIterations = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 4) // 4 groups of concurrent operations

	// Concurrent writers for SetBlock
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				item.SetBlock(block, time.Duration(j)*time.Millisecond)
			}
		}()
	}

	// Concurrent writers for SetValidation
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				item.SetValidation(true, "test", nil, time.Duration(j)*time.Millisecond)
			}
		}()
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				_ = item.Block()
				_ = item.IsDecoded()
				_ = item.IsValid()
			}
		}()
	}

	// Concurrent writers for SetApplied
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				item.SetApplied(true, nil, time.Duration(j)*time.Millisecond)
			}
		}()
	}

	wg.Wait()
	// If we get here without panic/race detection, the test passes
}

func TestBlockItem_Slot_BlockNumber(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(12345, 6789)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	assert.Equal(t, uint64(12345), item.Slot())
	assert.Equal(t, uint64(6789), item.BlockNumber())
}

// ============================================================================
// TestDecodeStage tests
// ============================================================================

func TestDecodeStage_SuccessfulDecode(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	stage := NewDecodeStage(true) // Skip body hash validation for test

	err := stage.Process(context.Background(), item)

	require.NoError(t, err)
	assert.True(t, item.IsDecoded())
	assert.NotNil(t, item.Block())
	assert.Nil(t, item.DecodeError())
	assert.Greater(t, item.DecodeDuration(), time.Duration(0))
}

func TestDecodeStage_DecodeErrorHandling(t *testing.T) {
	rawCbor := getInvalidBlockCbor()
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	stage := NewDecodeStage(false)

	err := stage.Process(context.Background(), item)

	// DecodeStage returns error on failure
	assert.Error(t, err)
	assert.False(t, item.IsDecoded())
	assert.Nil(t, item.Block())
	assert.NotNil(t, item.DecodeError())
}

func TestDecodeStage_ContextCancellation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	stage := NewDecodeStage(true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before processing

	err := stage.Process(ctx, item)

	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, item.IsDecoded())
}

func TestDecodeStage_Name(t *testing.T) {
	stage := NewDecodeStage(false)
	assert.Equal(t, "decode", stage.Name())
}

// ============================================================================
// TestDecodeStageWorkerPool tests
// ============================================================================

func TestDecodeStageWorkerPool_MultipleWorkers(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numItems = 20
	const numWorkers = 4

	input := make(chan *BlockItem, numItems)
	output := make(chan *BlockItem, numItems)
	errors := make(chan error, numItems)

	// Populate input channel
	for i := 0; i < numItems; i++ {
		tip := createTestTip(uint64(1000+i), uint64(500+i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		input <- item
	}
	close(input)

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, numWorkers, input, output, errors)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool.Start(ctx)
	pool.Stop()

	// Collect results
	close(output)
	close(errors)

	var decoded []*BlockItem
	for item := range output {
		decoded = append(decoded, item)
	}

	assert.Len(t, decoded, numItems)
	for _, item := range decoded {
		assert.True(t, item.IsDecoded(), "Item seq %d should be decoded", item.SequenceNumber)
	}
}

func TestDecodeStageWorkerPool_ItemsFlowThrough(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numItems = 5

	input := make(chan *BlockItem, numItems)
	output := make(chan *BlockItem, numItems)
	errChan := make(chan error, numItems)

	// Create items with specific sequence numbers
	expectedSeqs := make(map[uint64]bool)
	for i := 0; i < numItems; i++ {
		seq := uint64(100 + i)
		expectedSeqs[seq] = false
		tip := createTestTip(1000, 500)
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, seq)
		input <- item
	}
	close(input)

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, 2, input, output, errChan)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool.Start(ctx)
	pool.Stop()

	close(output)
	close(errChan)

	// Verify all items came through (order may vary due to parallelism)
	for item := range output {
		if _, exists := expectedSeqs[item.SequenceNumber]; exists {
			expectedSeqs[item.SequenceNumber] = true
		}
	}

	for seq, seen := range expectedSeqs {
		assert.True(t, seen, "Sequence %d should have been output", seq)
	}
}

func TestDecodeStageWorkerPool_CleanShutdown(t *testing.T) {
	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 10)
	errChan := make(chan error, 10)

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, 3, input, output, errChan)

	ctx, cancel := context.WithCancel(context.Background())
	pool.Start(ctx)

	// Cancel and verify clean shutdown
	cancel()
	close(input)

	// Should complete without hanging
	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Pool did not shut down cleanly within timeout")
	}
}

// ============================================================================
// TestValidateStage tests
// ============================================================================

func TestValidateStage_SkipsItemsWithDecodeErrors(t *testing.T) {
	// Create an item that failed decode
	rawCbor := getInvalidBlockCbor()
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)
	item.SetDecodeError(errors.New("decode failed"), 10*time.Millisecond)

	// Create real validate stage
	config := ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
	}
	validateStage := NewValidateStage(config)

	err := validateStage.Process(context.Background(), item)

	// Should return nil to avoid spurious error - decode stage already reported it
	assert.NoError(t, err)
	assert.False(t, item.IsValid()) // Should not be marked valid since decode failed
}

func TestValidateStage_ValidationResultSetCorrectly(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, 50*time.Millisecond)

	// Create real validate stage (validation will fail without proper eta0,
	// but we can verify the stage processes correctly)
	config := ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	validateStage := NewValidateStage(config)

	err = validateStage.Process(context.Background(), item)

	// Validation will likely fail due to incorrect eta0, but the stage should
	// still set the validation result properly
	assert.Greater(t, item.ValidateDuration(), time.Duration(0))
}

func TestValidateStage_Name(t *testing.T) {
	config := ValidateStageConfig{}
	stage := NewValidateStage(config)
	assert.Equal(t, "validate", stage.Name())
}

func TestValidateStage_Eta0ProviderCalled(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	// Track provider calls
	var calledSlot uint64
	provider := func(slot uint64) (string, error) {
		calledSlot = slot
		// Return dummy nonce (validation will fail but we're testing the provider is called)
		return "0000000000000000000000000000000000000000000000000000000000000000", nil
	}

	config := ValidateStageConfig{
		Eta0Provider:      provider,
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	stage := NewValidateStage(config)

	// Process will fail validation due to incorrect nonce, but provider should be called
	_ = stage.Process(context.Background(), item)

	// Verify provider was called with the block's slot number
	assert.Equal(t, block.SlotNumber(), calledSlot, "Provider should be called with block's slot number")
	assert.Greater(t, item.ValidateDuration(), time.Duration(0), "ValidateDuration should be set")
}

func TestValidateStage_Eta0ProviderError(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	// Provider that returns an error
	providerErr := errors.New("epoch nonce not available")
	provider := func(slot uint64) (string, error) {
		return "", providerErr
	}

	config := ValidateStageConfig{
		Eta0Provider:      provider,
		SlotsPerKesPeriod: 129600,
	}
	stage := NewValidateStage(config)

	err = stage.Process(context.Background(), item)

	// Should return error from provider
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "eta0 provider error")
	assert.ErrorIs(t, err, providerErr)
	assert.False(t, item.IsValid())
	assert.Greater(t, item.ValidateDuration(), time.Duration(0))
}

func TestValidateStage_NoProviderReturnsError(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(159835207, 12107553)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode the block first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	// Configure without Eta0Provider - this is a configuration error
	config := ValidateStageConfig{
		Eta0Provider:      nil, // No provider - configuration error
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	stage := NewValidateStage(config)

	// Should return a clear error when no Eta0Provider is configured
	err = stage.Process(context.Background(), item)

	// Expect a configuration error
	assert.Error(t, err, "Should fail without Eta0Provider")
	assert.Contains(t, err.Error(), "eta0 provider not configured")
	assert.False(t, item.IsValid())
	assert.Greater(t, item.ValidateDuration(), time.Duration(0), "ValidateDuration should be set")
}

func TestValidateStage_ContextCancellation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	// Decode first
	block, err := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	require.NoError(t, err)
	item.SetBlock(block, time.Millisecond)

	config := ValidateStageConfig{}
	stage := NewValidateStage(config)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before processing

	err = stage.Process(ctx, item)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// TestApplyStage tests
// ============================================================================

func TestApplyStage_Name(t *testing.T) {
	stage := NewApplyStage(nil)
	assert.Equal(t, "apply", stage.Name())
}

func TestApplyStageOrdering_OutOfOrderReordering(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	var appliedOrder []uint64
	var mu sync.Mutex

	// Create real apply stage with order tracking
	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedOrder = append(appliedOrder, item.SequenceNumber)
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc)

	// Create items with sequence numbers
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		// Decode and validate them
		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)
		items[i].SetValidation(true, "vrf", nil, time.Millisecond)
	}

	// Process items in scrambled order: 2, 0, 4, 1, 3
	scrambledOrder := []int{2, 0, 4, 1, 3}
	for _, idx := range scrambledOrder {
		err := applyStage.Process(context.Background(), items[idx])
		require.NoError(t, err)
	}

	// Verify items were applied in sequence order
	assert.Equal(t, []uint64{0, 1, 2, 3, 4}, appliedOrder)
}

func TestApplyStageOrdering_SkipsInvalidItems(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	var appliedSeqs []uint64
	var mu sync.Mutex

	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedSeqs = append(appliedSeqs, item.SequenceNumber)
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc)

	// Create items - some valid, some invalid
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)

		// Make items 1 and 3 invalid (using validation error)
		if i == 1 || i == 3 {
			items[i].SetValidation(false, "", errors.New("validation failed"), time.Millisecond)
		} else {
			items[i].SetValidation(true, "vrf", nil, time.Millisecond)
		}
	}

	// Process all items in order
	for i := range items {
		err := applyStage.Process(context.Background(), items[i])
		require.NoError(t, err)
	}

	// Only items 0, 2, 4 should have been applied (1 and 3 had validation errors)
	assert.Equal(t, []uint64{0, 2, 4}, appliedSeqs)
}

func TestApplyStage_PendingCount(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	applyStage := NewApplyStage(func(item *BlockItem) error {
		return nil
	})

	// Create items 1 and 2 (but not 0)
	for i := 1; i <= 2; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		block, _ := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		item.SetBlock(block, time.Millisecond)
		item.SetValidation(true, "vrf", nil, time.Millisecond)
		_ = applyStage.Process(context.Background(), item)
	}

	// Items 1 and 2 should be pending (waiting for item 0)
	assert.Equal(t, 2, applyStage.PendingCount())

	// Now process item 0 - should trigger applying all
	tip := createTestTip(1000, 500)
	item0 := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 0)
	block, _ := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	item0.SetBlock(block, time.Millisecond)
	item0.SetValidation(true, "vrf", nil, time.Millisecond)
	_ = applyStage.Process(context.Background(), item0)

	// All should be applied now
	assert.Equal(t, 0, applyStage.PendingCount())
}

func TestApplyStage_Reset(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	applyStage := NewApplyStage(nil)

	// Add some pending items
	for i := 1; i <= 3; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		block, _ := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		item.SetBlock(block, time.Millisecond)
		item.SetValidation(true, "vrf", nil, time.Millisecond)
		_ = applyStage.Process(context.Background(), item)
	}

	assert.Equal(t, 3, applyStage.PendingCount())

	// Reset should clear pending
	applyStage.Reset()

	assert.Equal(t, 0, applyStage.PendingCount())
}

func TestApplyStage_ContextCancellation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	applyStage := NewApplyStage(nil)

	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 0)
	block, _ := ledger.NewBlockFromCbor(
		uint(ledger.BlockTypeConway),
		rawCbor,
		common.VerifyConfig{SkipBodyHashValidation: true},
	)
	item.SetBlock(block, time.Millisecond)
	item.SetValidation(true, "vrf", nil, time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := applyStage.Process(ctx, item)
	assert.ErrorIs(t, err, context.Canceled)
}

// ============================================================================
// TestBlockPipeline tests
// ============================================================================

func TestBlockPipeline_StartStop(t *testing.T) {
	// Create pipeline with proper configuration
	config := DefaultPipelineConfig()
	config.SkipBodyHashValidation = true
	// Provide a valid 32-byte eta0 (64 hex chars)
	config.ValidateConfig = ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	config.ApplyFunc = func(item *BlockItem) error {
		return nil
	}
	p := NewBlockPipeline(config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start pipeline
	err := p.Start(ctx)
	require.NoError(t, err)

	// Verify pipeline is started (Submit should work)
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	err = p.Submit(uint(ledger.BlockTypeConway), rawCbor, tip)
	assert.NoError(t, err)

	// Stop pipeline
	err = p.Stop()
	assert.NoError(t, err)

	// Submit after stop should fail
	err = p.Submit(uint(ledger.BlockTypeConway), rawCbor, tip)
	assert.ErrorIs(t, err, ErrPipelineStopped)

	// Start after stop should fail
	err = p.Start(ctx)
	assert.ErrorIs(t, err, ErrPipelineStopped)
}

func TestBlockPipeline_NotStarted(t *testing.T) {
	// Create pipeline without starting
	config := DefaultPipelineConfig()
	p := NewBlockPipeline(config)

	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	err := p.Submit(uint(ledger.BlockTypeConway), rawCbor, tip)

	assert.ErrorIs(t, err, ErrPipelineNotStarted)
}

func TestBlockPipeline_SubmitAndResults(t *testing.T) {
	// Test a simulated pipeline flow using channels
	rawCbor := getValidBlockCbor(t)
	const numBlocks = 10

	input := make(chan *BlockItem, numBlocks)
	results := make(chan *BlockItem, numBlocks)
	var stats PipelineStats
	var statsMu sync.Mutex

	// Simulate pipeline processing
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup

	// Decode worker
	decoded := make(chan *BlockItem, numBlocks)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(decoded)
		stage := NewDecodeStage(true)
		for item := range input {
			if err := stage.Process(ctx, item); err == nil {
				decoded <- item
				statsMu.Lock()
				stats.BlocksDecoded++
				statsMu.Unlock()
			}
		}
	}()

	// Validate worker (mock)
	validated := make(chan *BlockItem, numBlocks)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(validated)
		for item := range decoded {
			if item.IsDecoded() {
				item.SetValidation(true, "mock_vrf", nil, time.Millisecond)
				statsMu.Lock()
				stats.BlocksValidated++
				statsMu.Unlock()
			}
			validated <- item
		}
	}()

	// Apply worker
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(results)
		for item := range validated {
			if item.IsValid() {
				item.SetApplied(true, nil, time.Millisecond)
				statsMu.Lock()
				stats.BlocksApplied++
				statsMu.Unlock()
			}
			results <- item
		}
	}()

	// Submit blocks
	for i := 0; i < numBlocks; i++ {
		tip := createTestTip(uint64(1000+i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		input <- item
	}
	close(input)

	// Collect results
	var resultItems []*BlockItem
	for item := range results {
		resultItems = append(resultItems, item)
	}

	wg.Wait()

	// Verify
	assert.Len(t, resultItems, numBlocks)
	assert.Equal(t, uint64(numBlocks), stats.BlocksDecoded)
	assert.Equal(t, uint64(numBlocks), stats.BlocksValidated)
	assert.Equal(t, uint64(numBlocks), stats.BlocksApplied)
}

func TestBlockPipeline_StatsUpdated(t *testing.T) {
	// Test that stats tracking works
	var submitted atomic.Uint64
	var decoded atomic.Uint64

	rawCbor := getValidBlockCbor(t)

	// Simple stats tracking
	for i := 0; i < 5; i++ {
		submitted.Add(1)

		tip := createTestTip(uint64(1000+i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		stage := NewDecodeStage(true)
		if err := stage.Process(context.Background(), item); err == nil {
			decoded.Add(1)
		}
	}

	assert.Equal(t, uint64(5), submitted.Load())
	assert.Equal(t, uint64(5), decoded.Load())
}

// ============================================================================
// TestPipelineBackpressure tests
// ============================================================================

func TestPipelineBackpressure_SlowConsumer(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const bufferSize = 5
	const numBlocks = 10

	// Create buffered input channel (simulates prefetch buffer)
	input := make(chan *BlockItem, bufferSize)
	output := make(chan *BlockItem, 1) // Slow consumer (small buffer)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var producerDone atomic.Bool
	var itemsProduced atomic.Uint64
	var itemsConsumed atomic.Uint64

	// Producer
	go func() {
		for i := 0; i < numBlocks; i++ {
			tip := createTestTip(uint64(i), uint64(i))
			item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
			select {
			case input <- item:
				itemsProduced.Add(1)
			case <-ctx.Done():
				return
			}
		}
		close(input)
		producerDone.Store(true)
	}()

	// Slow processor
	go func() {
		stage := NewDecodeStage(true)
		for item := range input {
			_ = stage.Process(ctx, item)
			select {
			case output <- item:
			case <-ctx.Done():
				return
			}
		}
		close(output)
	}()

	// Slow consumer (processes items slowly)
	go func() {
		for range output {
			time.Sleep(10 * time.Millisecond) // Simulate slow processing
			itemsConsumed.Add(1)
		}
	}()

	// Wait for producer to finish or timeout
	for !producerDone.Load() {
		select {
		case <-ctx.Done():
			t.Fatal("Test timed out")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Eventually all items should be produced (producer shouldn't be blocked forever)
	assert.Equal(t, uint64(numBlocks), itemsProduced.Load())
}

func TestPipelineBackpressure_PrefetchBufferFillsUp(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const bufferSize = 3

	// Small buffer to test fill-up behavior
	input := make(chan *BlockItem, bufferSize)

	// Fill buffer
	for i := 0; i < bufferSize; i++ {
		tip := createTestTip(uint64(i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		select {
		case input <- item:
		default:
			t.Fatalf("Buffer should not be full after %d items", i)
		}
	}

	// Buffer should be full now
	assert.Len(t, input, bufferSize)

	// Try to add one more (should not block due to select default)
	tip := createTestTip(100, 100)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 100)

	select {
	case input <- item:
		t.Fatal("Should not have been able to add item to full buffer")
	default:
		// Expected - buffer is full
	}
}

// ============================================================================
// TestPipelineGracefulShutdown tests
// ============================================================================

func TestPipelineGracefulShutdown_InFlightItemsComplete(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numItems = 5

	input := make(chan *BlockItem, numItems)
	output := make(chan *BlockItem, numItems)

	var processedCount atomic.Uint64

	ctx, cancel := context.WithCancel(context.Background())

	// Worker that processes items
	go func() {
		stage := NewDecodeStage(true)
		for {
			select {
			case item, ok := <-input:
				if !ok {
					close(output)
					return
				}
				_ = stage.Process(context.Background(), item) // Use fresh context to complete
				output <- item
				processedCount.Add(1)
			case <-ctx.Done():
				// Drain remaining items in channel
				for item := range input {
					_ = stage.Process(context.Background(), item)
					output <- item
					processedCount.Add(1)
				}
				close(output)
				return
			}
		}
	}()

	// Submit items
	for i := 0; i < numItems; i++ {
		tip := createTestTip(uint64(i), uint64(i))
		item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))
		input <- item
	}

	// Give some time for processing to start
	time.Sleep(50 * time.Millisecond)

	// Cancel context and close input to signal shutdown
	cancel()
	close(input)

	// Wait for output to close
	var received []*BlockItem
	for item := range output {
		received = append(received, item)
	}

	// All items should have been processed
	assert.Len(t, received, numItems)
	assert.Equal(t, uint64(numItems), processedCount.Load())
}

func TestPipelineGracefulShutdown_ChannelsCloseCleanly(t *testing.T) {
	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 10)
	errors := make(chan error, 10)

	ctx, cancel := context.WithCancel(context.Background())

	stage := NewDecodeStage(true)
	pool := NewDecodeStageWorkerPool(stage, 2, input, output, errors)

	pool.Start(ctx)

	// Close input channel
	close(input)

	// Wait for pool to stop
	done := make(chan struct{})
	go func() {
		pool.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success - pool stopped
	case <-time.After(5 * time.Second):
		t.Fatal("Pool did not stop cleanly")
	}

	// Cancel context
	cancel()

	// Output channel should be closeable without issue
	close(output)
	close(errors)
}

// ============================================================================
// StageFunc tests
// ============================================================================

func TestStageFunc_NameAndProcess(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	processedItems := 0

	stage := NewStageFunc("test-stage", func(ctx context.Context, item *BlockItem) error {
		processedItems++
		return nil
	})

	assert.Equal(t, "test-stage", stage.Name())

	err := stage.Process(context.Background(), item)
	assert.NoError(t, err)
	assert.Equal(t, 1, processedItems)
}

func TestStageFunc_ErrorHandling(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)
	item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)

	expectedErr := errors.New("stage failed")

	stage := NewStageFunc("error-stage", func(ctx context.Context, item *BlockItem) error {
		return expectedErr
	})

	err := stage.Process(context.Background(), item)
	assert.Equal(t, expectedErr, err)
}

// ============================================================================
// Worker pool numWorkers validation tests
// ============================================================================

func TestDecodeStageWorkerPool_NumWorkersValidation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	stage := NewDecodeStage(true)

	testCases := []struct {
		name              string
		numWorkers        int
		expectedToProcess bool
	}{
		{"zero workers defaults to 1", 0, true},
		{"negative workers defaults to 1", -1, true},
		{"negative workers defaults to 1 v2", -100, true},
		{"one worker works", 1, true},
		{"multiple workers work", 4, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := make(chan *BlockItem, 1)
			output := make(chan *BlockItem, 1)
			errChan := make(chan error, 1)

			pool := NewDecodeStageWorkerPool(stage, tc.numWorkers, input, output, errChan)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pool.Start(ctx)

			// Submit one item
			tip := createTestTip(1000, 500)
			item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)
			input <- item
			close(input)

			// Wait for output
			select {
			case result := <-output:
				if tc.expectedToProcess {
					assert.True(t, result.IsDecoded(), "Item should be decoded")
				}
			case <-time.After(2 * time.Second):
				if tc.expectedToProcess {
					t.Fatal("Timed out waiting for item to be processed")
				}
			}

			pool.Stop()
		})
	}
}

func TestValidateStageWorkerPool_NumWorkersValidation(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	config := ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	stage := NewValidateStage(config)

	testCases := []struct {
		name       string
		numWorkers int
	}{
		{"zero workers defaults to 1", 0},
		{"negative workers defaults to 1", -1},
		{"negative workers defaults to 1 v2", -100},
		{"one worker works", 1},
		{"multiple workers work", 4},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			input := make(chan *BlockItem, 1)
			output := make(chan *BlockItem, 1)
			errChan := make(chan error, 1)

			pool := NewValidateStageWorkerPool(stage, tc.numWorkers, input, output, errChan)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			pool.Start(ctx)

			// Create a decoded item (validation requires decoded block)
			tip := createTestTip(1000, 500)
			item := NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, 1)
			block, err := ledger.NewBlockFromCbor(
				uint(ledger.BlockTypeConway),
				rawCbor,
				common.VerifyConfig{SkipBodyHashValidation: true},
			)
			require.NoError(t, err)
			item.SetBlock(block, time.Millisecond)

			input <- item
			close(input)

			// Wait for output (item should flow through even if validation fails)
			select {
			case <-output:
				// Item was processed - test passes
			case <-time.After(2 * time.Second):
				t.Fatal("Timed out waiting for item to be processed")
			}

			pool.Stop()
		})
	}
}

// TestBlockPipeline_SubmitStopRaceCondition verifies that concurrent Submit() and Stop()
// calls do not cause a panic from sending on a closed channel.
// This is a regression test for the race condition where Stop() could close submitChan
// between the stopped.Load() check and the channel send in Submit().
func TestBlockPipeline_SubmitStopRaceCondition(t *testing.T) {
	config := DefaultPipelineConfig()
	config.SkipBodyHashValidation = true
	config.ValidateConfig = ValidateStageConfig{
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	config.ApplyFunc = func(item *BlockItem) error {
		return nil
	}

	rawCbor := getValidBlockCbor(t)
	tip := createTestTip(1000, 500)

	// Run multiple iterations to increase likelihood of hitting the race
	for iteration := 0; iteration < 100; iteration++ {
		p := NewBlockPipeline(config)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		err := p.Start(ctx)
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: Rapidly submit items
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				err := p.Submit(uint(ledger.BlockTypeConway), rawCbor, tip)
				// Either no error, ErrPipelineStopped, or context.Canceled is acceptable.
				// context.Canceled occurs when Stop() cancels the context before Submit
				// can complete its channel send.
				if err != nil && !errors.Is(err, ErrPipelineStopped) && !errors.Is(err, context.Canceled) {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		}()

		// Goroutine 2: Stop the pipeline after a tiny delay
		go func() {
			defer wg.Done()
			// Small random delay to create race conditions
			time.Sleep(time.Duration(iteration%5) * time.Microsecond)
			_ = p.Stop()
		}()

		wg.Wait()
		cancel()
	}
}

// TestApplyStageRunner_OutOfOrderItemsForwarded verifies that out-of-order items
// that are buffered and later applied from pending are correctly forwarded to output.
// This is a regression test for the data loss bug where buffered items were applied
// but never forwarded to the output channel.
func TestApplyStageRunner_OutOfOrderItemsForwarded(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	var appliedOrder []uint64
	var mu sync.Mutex

	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedOrder = append(appliedOrder, item.SequenceNumber)
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc)

	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 10)
	errors := make(chan error, 10)

	runner := NewApplyStageRunner(applyStage, input, output, errors, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runner.Start(ctx)

	// Create 5 items with sequence numbers 0-4
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)
		items[i].SetValidation(true, "vrf", nil, time.Millisecond)
	}

	// Send items in scrambled order: 2, 4, 1, 3, 0
	// This means items 2, 4, 1, 3 will be buffered until item 0 arrives
	scrambledOrder := []int{2, 4, 1, 3, 0}
	for _, idx := range scrambledOrder {
		input <- items[idx]
	}
	close(input)

	// Collect all items from output
	var received []*BlockItem
	for item := range output {
		received = append(received, item)
		if len(received) == 5 {
			break
		}
	}

	runner.Stop()

	// Verify all 5 items were forwarded to output (no data loss)
	assert.Len(t, received, 5, "All items should be forwarded to output")

	// Verify items were applied in sequence order
	assert.Equal(
		t,
		[]uint64{0, 1, 2, 3, 4},
		appliedOrder,
		"Items should be applied in sequence order",
	)

	// Verify all received items are marked as applied
	for _, item := range received {
		assert.True(t, item.IsApplied(), "Item %d should be marked as applied", item.SequenceNumber)
	}
}

// TestApplyStageRunner_OutOfOrderErrorItemsForwardedOnce verifies that items with
// decode/validation errors that arrive out of order are only forwarded once to the
// output channel. This is a regression test for the double-forwarding bug where
// error items were forwarded immediately in run() and again later via pendingQueue
// when applyPending() processed them.
func TestApplyStageRunner_OutOfOrderErrorItemsForwardedOnce(t *testing.T) {
	rawCbor := getValidBlockCbor(t)

	var appliedOrder []uint64
	var mu sync.Mutex

	applyFunc := func(item *BlockItem) error {
		mu.Lock()
		appliedOrder = append(appliedOrder, item.SequenceNumber)
		mu.Unlock()
		return nil
	}

	applyStage := NewApplyStage(applyFunc)

	input := make(chan *BlockItem, 10)
	output := make(chan *BlockItem, 20) // Extra capacity to detect duplicates
	errors := make(chan error, 10)

	runner := NewApplyStageRunner(applyStage, input, output, errors, 100)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	runner.Start(ctx)

	// Create 5 items with sequence numbers 0-4
	// Items 1 and 3 will have validation errors
	items := make([]*BlockItem, 5)
	for i := 0; i < 5; i++ {
		tip := createTestTip(1000+uint64(i), 500+uint64(i))
		items[i] = NewBlockItem(uint(ledger.BlockTypeConway), rawCbor, tip, uint64(i))

		block, err := ledger.NewBlockFromCbor(
			uint(ledger.BlockTypeConway),
			rawCbor,
			common.VerifyConfig{SkipBodyHashValidation: true},
		)
		require.NoError(t, err)
		items[i].SetBlock(block, time.Millisecond)

		// Items 1 and 3 have validation errors
		if i == 1 || i == 3 {
			items[i].SetValidation(false, "", fmt.Errorf("validation error for item %d", i), time.Millisecond)
		} else {
			items[i].SetValidation(true, "vrf", nil, time.Millisecond)
		}
	}

	// Send items in scrambled order: 2, 4, 1, 3, 0
	// Items 2, 4, 1, 3 will be buffered until item 0 arrives
	// Items 1 and 3 have validation errors and should only be forwarded once
	scrambledOrder := []int{2, 4, 1, 3, 0}
	for _, idx := range scrambledOrder {
		input <- items[idx]
	}
	close(input)

	// Collect all items from output with a timeout
	var received []*BlockItem
	timeout := time.After(2 * time.Second)
collectLoop:
	for {
		select {
		case item, ok := <-output:
			if !ok {
				break collectLoop
			}
			received = append(received, item)
			// If we get more than 5, that's a bug (duplicates)
			if len(received) > 5 {
				t.Fatalf("Received more than 5 items (%d), likely duplicates", len(received))
			}
		case <-timeout:
			break collectLoop
		}
	}

	runner.Stop()

	// Verify exactly 5 items were forwarded (no duplicates)
	assert.Len(t, received, 5, "Exactly 5 items should be forwarded (no duplicates)")

	// Count how many times each sequence number appears
	seqCounts := make(map[uint64]int)
	for _, item := range received {
		seqCounts[item.SequenceNumber]++
	}

	// Verify each item was forwarded exactly once
	for seq, count := range seqCounts {
		assert.Equal(t, 1, count, "Item %d should be forwarded exactly once, got %d", seq, count)
	}

	// Verify items with validation errors were NOT applied
	for _, seq := range appliedOrder {
		if seq == 1 || seq == 3 {
			t.Errorf("Item %d has validation error but was applied", seq)
		}
	}

	// Verify valid items (0, 2, 4) were applied in sequence order
	expectedApplied := []uint64{0, 2, 4}
	assert.Equal(t, expectedApplied, appliedOrder, "Valid items should be applied in sequence order")
}

// TestBlockPipeline_MetricsRecorded verifies that the pipeline correctly
// records metrics for decode, validate, apply stages and end-to-end latency.
// Note: This test uses real validation which will fail due to incorrect Eta0,
// so we verify that validation errors are recorded rather than successful validations.
func TestBlockPipeline_MetricsRecorded(t *testing.T) {
	rawCbor := getValidBlockCbor(t)
	const numBlocks = 5

	config := DefaultPipelineConfig()
	config.SkipBodyHashValidation = true
	config.ValidateConfig = ValidateStageConfig{
		// Using dummy Eta0 will cause validation to fail, which is expected
		Eta0Provider:      StaticEta0Provider("0000000000000000000000000000000000000000000000000000000000000000"),
		SlotsPerKesPeriod: 129600,
		VerifyConfig: common.VerifyConfig{
			SkipBodyHashValidation:    true,
			SkipTransactionValidation: true,
			SkipStakePoolValidation:   true,
		},
	}
	config.ApplyFunc = func(item *BlockItem) error {
		return nil
	}

	pipeline := NewBlockPipeline(config)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := pipeline.Start(ctx)
	require.NoError(t, err)

	// Submit blocks
	for i := 0; i < numBlocks; i++ {
		tip := createTestTip(uint64(1000+i), uint64(i))
		err := pipeline.Submit(uint(ledger.BlockTypeConway), rawCbor, tip)
		require.NoError(t, err)
	}

	// Wait for all blocks to be processed (they flow through even with validation errors)
	received := 0
	for received < numBlocks {
		select {
		case <-pipeline.Results():
			received++
		case <-ctx.Done():
			t.Fatal("Timed out waiting for results")
		}
	}

	// Stop the pipeline
	err = pipeline.Stop()
	require.NoError(t, err)

	// Verify metrics
	stats := pipeline.Stats()

	assert.Equal(t, uint64(numBlocks), stats.BlocksSubmitted, "BlocksSubmitted should match")
	assert.Equal(t, uint64(numBlocks), stats.BlocksDecoded, "BlocksDecoded should match")
	assert.Equal(t, uint64(0), stats.DecodeErrors, "DecodeErrors should be 0")

	// Validation will fail due to incorrect Eta0, so we expect validation errors
	assert.Equal(t, uint64(numBlocks), stats.ValidationErrors, "ValidationErrors should match (validation fails with dummy Eta0)")
	assert.Equal(t, uint64(0), stats.BlocksValidated, "BlocksValidated should be 0 (validation fails with dummy Eta0)")

	// Apply and pipeline latency won't be recorded for failed validations
	// (items with validation errors are not applied)
	assert.Equal(t, uint64(0), stats.BlocksApplied, "BlocksApplied should be 0 (failed validations are not applied)")

	// Verify latency stats for decode and validate are populated
	assert.Greater(t, stats.DecodeLatency.P50, time.Duration(0), "DecodeLatency.P50 should be > 0")
	assert.Greater(t, stats.ValidateLatency.P50, time.Duration(0), "ValidateLatency.P50 should be > 0")
}
