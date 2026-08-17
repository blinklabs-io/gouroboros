module github.com/blinklabs-io/gouroboros

go 1.25.7

toolchain go1.25.8

require (
	filippo.io/edwards25519 v1.2.0
	github.com/blinklabs-io/ouroboros-mock v0.16.0
	github.com/blinklabs-io/plutigo v0.3.0
	github.com/btcsuite/btcd/btcutil v1.2.0
	github.com/fxamacker/cbor/v2 v2.9.2
	github.com/jinzhu/copier v0.4.0
	github.com/stretchr/testify v1.11.1
	github.com/utxorpc/go-codegen v0.19.2
	go.uber.org/goleak v1.3.0
	golang.org/x/crypto v0.55.0
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/bits-and-blooms/bitset v1.24.4 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.5.0 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.2.0 // indirect
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0 // indirect
	github.com/consensys/gnark-crypto v0.20.1 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.3 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sys v0.47.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/blinklabs-io/plutigo => ../plutigo
