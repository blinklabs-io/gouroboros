module github.com/blinklabs-io/gouroboros

go 1.26.10

require (
	filippo.io/edwards25519 v1.2.0
	github.com/blinklabs-io/ouroboros-mock v0.20.5
	github.com/blinklabs-io/plutigo v0.9.6
	github.com/btcsuite/btcd/btcutil v1.2.0
	github.com/consensys/gnark-crypto v0.22.0
	github.com/fxamacker/cbor/v2 v2.9.6
	github.com/jinzhu/copier v0.4.0
	github.com/stretchr/testify v1.12.1
	github.com/utxorpc/go-codegen v0.19.2
	go.uber.org/goleak v1.3.0
	golang.org/x/crypto v0.58.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/aclements/go-moremath v0.0.0-20210112150236-f10218a38794 // indirect
	github.com/bits-and-blooms/bitset v1.25.0 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.5.0 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.2.0 // indirect
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.3 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/nilaway v0.0.0-20260918162853-acb8859b9031 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/exp/typeparams v0.0.0-20260908205506-85c1c2202aba // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/perf v0.0.0-20261009192801-be2c69fb417e // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/tools v0.50.0 // indirect
)

// replace github.com/blinklabs-io/plutigo => ../plutigo

tool (
	go.uber.org/nilaway/cmd/nilaway
	golang.org/x/perf/cmd/benchstat
)
