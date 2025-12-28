package conformance

import (
	"archive/tar"
	"compress/gzip"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/blinklabs-io/gouroboros/cbor"
)

// Quick inspector to decode a vector file in the rules-conformance tarball and
// print transaction witness information for debugging signature/utxo issues.
func main() {
	tarpath := flag.String(
		"tar",
		"internal/test/amaru/crates/amaru-ledger/tests/data/rules-conformance.tar.gz",
		"rules-conformance tarball",
	)
	vecName := flag.String(
		"vector",
		"",
		"path within tarball to vector file to inspect",
	)
	flag.Parse()

	if *vecName == "" {
		fmt.Println("provide -vector <path/inside/tarball>")
		os.Exit(2)
	}

	f, err := os.Open(*tarpath)
	if err != nil {
		fmt.Printf("open tarball: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		fmt.Printf("gzip: %v\n", err)
		os.Exit(1)
	}
	tr := tar.NewReader(gr)

	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("tar read: %v\n", err)
			os.Exit(1)
		}
		if filepath.Clean(h.Name) != filepath.Clean(*vecName) {
			continue
		}

		// read vector contents
		data := make([]byte, h.Size)
		if _, err := io.ReadFull(tr, data); err != nil {
			fmt.Printf("read vector: %v\n", err)
			os.Exit(1)
		}

		vec := &testVector{}
		if _, err := cbor.Decode(data, vec); err != nil {
			fmt.Printf("decode vector cbor: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Vector: %s\n", vec.title)
		pph := decodeInitialStatePParamsHash(nil, vec.initialState)
		fmt.Printf("pparams hash: %s\n", hex.EncodeToString(pph))

		utxos := decodeInitialStateUtxos(nil, vec.initialState)
		fmt.Printf("decoded %d UTxOs from initial state\n", len(utxos))

		for ei, ev := range vec.events {
			fmt.Printf(
				"--- event %d success=%v slot=%d\n",
				ei,
				ev.success,
				ev.slot,
			)
			tx := decodeTransaction(nil, ev.tx)
			txHash := tx.Hash()
			fmt.Printf("tx hash: %s\n", hex.EncodeToString(txHash[:]))

			// print witnesses
			w := tx.Witnesses()
			if w == nil {
				fmt.Println("no witnesses")
				continue
			}
			fmt.Printf("vkeys: %d\n", len(w.Vkey()))
			for i, v := range w.Vkey() {
				fmt.Printf(
					" vkey %d pubkey len=%d sig len=%d\n",
					i,
					len(v.Vkey),
					len(v.Signature),
				)
			}
			fmt.Printf("bootstrap: %d\n", len(w.Bootstrap()))
			fmt.Printf("native scripts: %d\n", len(w.NativeScripts()))
			fmt.Printf(
				"plutus v1: %d v2: %d v3: %d\n",
				len(w.PlutusV1Scripts()),
				len(w.PlutusV2Scripts()),
				len(w.PlutusV3Scripts()),
			)

			// check presence of collateral UTxOs referenced
			for _, c := range tx.Collateral() {
				found := false
				for _, u := range utxos {
					// compare inputs
					if u.Id == c {
						found = true
						break
					}
				}
				fmt.Printf("collateral %v present=%v\n", c, found)
			}
		}
		return
	}
	fmt.Printf("vector %s not found in tarball\n", *vecName)
}
