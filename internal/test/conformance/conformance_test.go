package conformance

import (
	"archive/tar"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/blinklabs-io/gouroboros/cbor"
	test_ledger "github.com/blinklabs-io/gouroboros/internal/test/ledger"
	"github.com/blinklabs-io/gouroboros/ledger"
	"github.com/blinklabs-io/gouroboros/ledger/babbage"
	"github.com/blinklabs-io/gouroboros/ledger/common"
	"github.com/blinklabs-io/gouroboros/ledger/conway"
	"github.com/blinklabs-io/gouroboros/ledger/shelley"
	"github.com/blinklabs-io/gouroboros/protocol/localstatequery"
)

var rulesConformanceTarball = filepath.Join(
	"..",
	"..",
	"..",
	"internal",
	"test",
	"amaru",
	"crates",
	"amaru-ledger",
	"tests",
	"data",
	"rules-conformance.tar.gz",
)

type vectorEvent struct {
	tx      []byte
	success bool
	slot    uint64
}

type testVector struct {
	title        string
	initialState cbor.RawMessage
	events       []vectorEvent
	pparamsHash  []byte
}

// utxosMatch compares two transaction inputs to see if they refer to the same UTxO
func utxosMatch(a, b common.TransactionInput) bool {
	// Try converting both to ShelleyTransactionInput for comparison
	aShelley, aOk := a.(shelley.ShelleyTransactionInput)
	bShelley, bOk := b.(shelley.ShelleyTransactionInput)
	if aOk && bOk {
		return aShelley.TxId == bShelley.TxId &&
			aShelley.OutputIndex == bShelley.OutputIndex
	}
	// Fallback: compare as generic inputs (may not work for all types)
	return false
}

func TestRulesConformanceVectors(t *testing.T) {
	tmpDir := t.TempDir()
	extractRulesConformance(t, tmpDir)

	conwayDumpRoot := filepath.Join(
		tmpDir,
		"eras",
		"conway",
		"impl",
		"dump",
		"Conway",
	)
	vectorFiles := collectVectorFiles(t, conwayDumpRoot)
	t.Logf("Found %d conformance vectors", len(vectorFiles))
	if len(vectorFiles) == 0 {
		t.Fatalf("no conformance vectors found")
	}

	for i, vectorPath := range vectorFiles {
		vectorPath := vectorPath
		t.Run(filepath.Base(vectorPath), func(t *testing.T) {
			if i < 5 { // Log first few
				t.Logf("Processing vector %d: %s", i, filepath.Base(vectorPath))
			}
			vector := decodeTestVector(t, vectorPath)
			if i < 5 {
				t.Logf("Vector title: %s", vector.title)
			}
			if strings.Contains(vector.title, "InvalidMetadata") {
				t.Logf(
					"Found InvalidMetadata vector with title: %s",
					vector.title,
				)
			}
			if len(vector.events) == 0 {
				t.Fatalf("vector %s has no transaction events", vector.title)
			}

			// Decode initial_state to extract current pparams hash and UTxOs
			pph := decodeInitialStatePParamsHash(t, vector.initialState)
			vector.pparamsHash = pph
			if len(pph) == 0 {
				t.Fatalf(
					"vector %s missing protocol parameters hash",
					vector.title,
				)
			}

			// Extract UTxOs from initial_state (map[utxoId]bytes format)
			utxos := decodeInitialStateUtxos(t, vector.initialState)

			// Verify the corresponding pparams file exists under pparams-by-hash
			pparamsFile := findPParamsByHash(t, conwayDumpRoot, pph)
			if pparamsFile == "" {
				t.Fatalf(
					"pparams file not found for hash %s",
					hex.EncodeToString(pph),
				)
			}

			// Load protocol parameters
			pp := loadProtocolParameters(t, pparamsFile)

			for txIdx, event := range vector.events {
				tx := decodeTransaction(t, event.tx)
				result, err := executeTransaction(t, tx, event.slot, pp, utxos)
				if result && !event.success {
					t.Errorf(
						"expected failure but got success (tx %d, IsValid=%v, has redeemers=%v)",
						txIdx,
						tx.IsValid(),
						tx.Witnesses() != nil &&
							tx.Witnesses().Redeemers() != nil,
					)
				}
				if !result && event.success {
					t.Errorf(
						"expected success but got failure (tx %d, IsValid=%v): %v",
						txIdx,
						tx.IsValid(),
						err,
					)
				}

				// Update UTxO set only for transactions that should be accepted into a block
				// event.success indicates the transaction should be included (even with IsValid=false)
				// Failed phase-1 transactions (event.success=false) should NOT update UTxOs
				// Successful transactions or phase-2 failures (event.success=true) SHOULD update UTxOs
				// Use Consumed() and Produced() methods which properly handle:
				// - Regular inputs vs collateral (consumed based on IsValid flag)
				// - Reference inputs (never consumed)
				// - Output creation with correct UTxO IDs
				if event.success {
					// Remove consumed inputs
					consumed := tx.Consumed()
					for _, consumedInput := range consumed {
						for i, utxo := range utxos {
							if utxosMatch(consumedInput, utxo.Id) {
								// Remove this UTxO by replacing it with the last element
								// and truncating the slice (avoids shifting all elements)
								utxos[i] = utxos[len(utxos)-1]
								utxos = utxos[:len(utxos)-1]
								break
							}
						}
					}

					// Add produced outputs
					produced := tx.Produced()
					utxos = append(utxos, produced...)
				}
			}
		})
	}
	t.Logf("Processed %d vectors", len(vectorFiles))
	// Summary will be calculated from the logs
}

func extractRulesConformance(t testing.TB, dest string) {
	tarball, err := os.Open(rulesConformanceTarball)
	if err != nil {
		t.Fatalf("failed to open rules conformance tarball: %v", err)
	}
	defer tarball.Close()

	gzipReader, err := gzip.NewReader(tarball)
	if err != nil {
		t.Fatalf("failed to decompress tarball: %v", err)
	}
	defer gzipReader.Close()

	tr := tar.NewReader(gzipReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("error reading tarball: %v", err)
		}

		target := filepath.Join(dest, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatalf("failed to create directory %s: %v", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf(
					"failed to create directory %s: %v",
					filepath.Dir(target),
					err,
				)
			}
			out, err := os.Create(target)
			if err != nil {
				t.Fatalf("failed to create file %s: %v", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				t.Fatalf("failed to write %s: %v", target, err)
			}
			out.Close()
		}
	}
}

func collectVectorFiles(t testing.TB, root string) []string {
	var vectors []string
	err := filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if strings.Contains(path, "pparams-by-hash") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.Contains(path, "pparams-by-hash") ||
				strings.Contains(path, "scripts/") {
				return nil
			}
			// Filter obvious non-vector files: we expect leaf files under the spec trees
			// Keep everything else (no strict extension filtering — Amaru vectors lack .cbor)
			if !strings.HasSuffix(path, "/README") &&
				!strings.HasSuffix(path, ".md") {
				vectors = append(vectors, path)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("failed to walk extracted conformance data: %v", err)
	}
	sort.Strings(vectors)
	return vectors
}

func decodeTestVector(t testing.TB, vectorPath string) testVector {
	data, err := os.ReadFile(vectorPath)
	if err != nil {
		t.Fatalf("failed to read vector %s: %v", vectorPath, err)
	}

	var items []cbor.RawMessage
	if _, err := cbor.Decode(data, &items); err != nil {
		t.Fatalf("failed to decode vector %s: %v", vectorPath, err)
	}
	if len(items) < 5 {
		t.Fatalf("unexpected vector structure %s", vectorPath)
	}

	var title string
	if _, err := cbor.Decode(items[4], &title); err != nil {
		t.Fatalf("failed to decode vector title %s: %v", vectorPath, err)
	}

	events := decodeEvents(t, items[3])
	return testVector{
		title:        title,
		initialState: items[1],
		events:       events,
	}
}

func decodeEvents(t testing.TB, raw cbor.RawMessage) []vectorEvent {
	var encodedEvents []cbor.RawMessage
	if _, err := cbor.Decode(raw, &encodedEvents); err != nil {
		t.Fatalf("failed to decode events list: %v", err)
	}

	var events []vectorEvent
	for _, rawEvent := range encodedEvents {
		var payload []any
		if _, err := cbor.Decode(rawEvent, &payload); err != nil {
			t.Fatalf("failed to decode event: %v", err)
		}
		if len(payload) == 0 {
			continue
		}
		variant, ok := payload[0].(uint64)
		if !ok {
			t.Fatalf("unexpected variant type: %T", payload[0])
		}
		if variant != 0 {
			continue
		}
		if len(payload) < 4 {
			t.Fatalf("transaction event missing fields")
		}
		txBytes, ok := payload[1].([]byte)
		if !ok {
			t.Fatalf("unexpected tx bytes type: %T", payload[1])
		}
		success, ok := payload[2].(bool)
		if !ok {
			t.Fatalf("unexpected success flag type: %T", payload[2])
		}
		slot, ok := payload[3].(uint64)
		if !ok {
			t.Fatalf("unexpected slot type: %T", payload[3])
		}
		events = append(
			events,
			vectorEvent{tx: txBytes, success: success, slot: slot},
		)
	}
	return events
}

// decodeInitialStatePParamsHash navigates the initial_state structure (mirroring
// the Rust decode_ledger_state) and extracts current_pparams_hash. It decodes into
// cbor.Value so byte-string map keys do not explode Go's map typing.
func decodeInitialStatePParamsHash(t testing.TB, raw cbor.RawMessage) []byte {
	var v cbor.Value
	if _, err := cbor.Decode(raw, &v); err != nil {
		t.Fatalf("failed to decode initial_state: %v", err)
	}
	top := v.Value()
	arr, ok := top.([]any)
	if !ok || len(arr) < 4 {
		t.Fatalf("unexpected initial_state shape: %T len=%d", top, len(arr))
	}
	bes, ok := arr[3].([]any)
	if !ok || len(bes) < 2 {
		t.Fatalf(
			"unexpected begin_epoch_state: %T len=%d types=%T",
			arr[3],
			len(bes),
			bes,
		)
	}
	ls, ok := bes[1].([]any)
	if !ok || len(ls) < 1 {
		t.Fatalf("unexpected ledger_state: %T", bes[1])
	}
	// gov_state is [proposals, committee, constitution, current_pparams_hash, ...]
	// current_pparams_hash is an array: [key, status, ..., current_pparams_hash_bytes, ...]
	for idx, item := range ls {
		sub, ok := item.([]any)
		if !ok || len(sub) <= 3 {
			continue
		}
		switch v := sub[3].(type) {
		case []byte:
			if len(v) > 0 {
				return v
			}
		case cbor.ByteString:
			b := v.Bytes()
			if len(b) > 0 {
				return b
			}
		case []any:
			// sub[3] is an array whose [3] element contains the hash
			if len(v) > 3 {
				switch hashv := v[3].(type) {
				case []byte:
					if len(hashv) > 0 {
						return hashv
					}
				case cbor.ByteString:
					b := hashv.Bytes()
					if len(b) > 0 {
						return b
					}
				}
			}
		}
		_ = idx
	}
	// Help debugging: log the ledger_state shape to understand where pparams hash sits
	typeNames := make([]string, len(ls))
	subLens := make([]int, len(ls))
	for i, item := range ls {
		typeNames[i] = fmt.Sprintf("%T", item)
		if s, ok := item.([]any); ok {
			subLens[i] = len(s)
		}
	}
	t.Fatalf("failed to locate protocol parameters hash in initial_state")
	return nil
}

// decodeInitialStateUtxos extracts UTxOs from initial_state by parsing CBOR bytes directly
type initialStateUtxos struct {
	utxos  []common.Utxo
	logger testing.TB
}

func (s *initialStateUtxos) UnmarshalCBOR(data []byte) error {
	// Decode the top-level array
	var arr []cbor.RawMessage
	if _, err := cbor.Decode(data, &arr); err != nil {
		return err
	}
	if len(arr) < 4 {
		return fmt.Errorf("initial_state array too short")
	}

	// arr[3] = begin_epoch_state
	var bes []cbor.RawMessage
	if _, err := cbor.Decode(arr[3], &bes); err != nil {
		return err
	}
	if len(bes) < 2 {
		return fmt.Errorf("begin_epoch_state array too short")
	}

	// bes[1] = begin_ledger_state
	var bls []cbor.RawMessage
	if _, err := cbor.Decode(bes[1], &bls); err != nil {
		return err
	}
	if len(bls) < 2 {
		return fmt.Errorf("begin_ledger_state array too short")
	}

	// bls[1] = utxo_state
	var utxoState []cbor.RawMessage
	if _, err := cbor.Decode(bls[1], &utxoState); err != nil {
		return err
	}
	if len(utxoState) < 1 {
		return fmt.Errorf("utxo_state array too short")
	}

	// utxoState[0] = utxos
	// Try to decode UTxOs from all elements in utxoState
	for _, utxoData := range utxoState {
		// Try direct map[UtxoId]BabbageTransactionOutput format
		var utxosMapDirect map[localstatequery.UtxoId]babbage.BabbageTransactionOutput
		if _, err := cbor.Decode(utxoData, &utxosMapDirect); err == nil {
			s.logger.Logf(
				"Decoded UTxOs using direct map[UtxoId] format: %d entries",
				len(utxosMapDirect),
			)
			// Convert map entries to common.Utxo format
			for utxoId, output := range utxosMapDirect {
				s.utxos = append(s.utxos, common.Utxo{
					Id: shelley.ShelleyTransactionInput{
						TxId:        utxoId.Hash,
						OutputIndex: uint32(utxoId.Idx),
					},
					Output: output,
				})
			}
			continue
		}

		// Try array of [UtxoId, Output] pairs
		var utxoPairs [][]cbor.RawMessage
		if _, err := cbor.Decode(utxoData, &utxoPairs); err == nil {
			s.logger.Logf(
				"Decoded UTxOs using array of pairs format: %d pairs",
				len(utxoPairs),
			)
			for _, pair := range utxoPairs {
				if len(pair) != 2 {
					continue
				}
				var utxoId localstatequery.UtxoId
				if _, err := cbor.Decode(pair[0], &utxoId); err != nil {
					continue
				}
				var output babbage.BabbageTransactionOutput
				if _, err := cbor.Decode(pair[1], &output); err != nil {
					continue
				}
				s.utxos = append(s.utxos, common.Utxo{
					Id: shelley.ShelleyTransactionInput{
						TxId:        utxoId.Hash,
						OutputIndex: uint32(utxoId.Idx),
					},
					Output: output,
				})
			}
			continue
		}

		// Try map with string keys (hex-encoded hashes)
		var utxosMapString map[string]babbage.BabbageTransactionOutput
		if _, err := cbor.Decode(utxoData, &utxosMapString); err == nil {
			for key, output := range utxosMapString {
				// Parse key as "hash#index"
				if parts := strings.Split(key, "#"); len(parts) == 2 {
					hashBytes, err := hex.DecodeString(parts[0])
					if err != nil {
						continue
					}
					var hash ledger.Blake2b256
					copy(hash[:], hashBytes)
					idx, err := strconv.Atoi(parts[1])
					if err != nil {
						continue
					}
					s.utxos = append(s.utxos, common.Utxo{
						Id: shelley.ShelleyTransactionInput{
							TxId:        hash,
							OutputIndex: uint32(idx),
						},
						Output: output,
					})
				}
			}
			continue
		}

		// Try decoding as the full utxo_state structure (array with map as first element)
		var fullUtxoState []cbor.RawMessage
		if _, err := cbor.Decode(utxoData, &fullUtxoState); err == nil &&
			len(fullUtxoState) > 0 {
			// Try to decode as the complete utxo_state from Amaru
			var utxoMap map[localstatequery.UtxoId]babbage.BabbageTransactionOutput
			if _, err := cbor.Decode(utxoData, &utxoMap); err == nil {
				for utxoId, output := range utxoMap {
					s.utxos = append(s.utxos, common.Utxo{
						Id: shelley.ShelleyTransactionInput{
							TxId:        utxoId.Hash,
							OutputIndex: uint32(utxoId.Idx),
						},
						Output: output,
					})
				}
				continue
			}

			// Try to recursively search for UTxO maps in the array elements
			for _, elem := range fullUtxoState {
				var elemUtxosMap map[localstatequery.UtxoId]babbage.BabbageTransactionOutput
				if _, err := cbor.Decode(elem, &elemUtxosMap); err == nil &&
					len(elemUtxosMap) > 0 {
					for utxoId, output := range elemUtxosMap {
						s.utxos = append(s.utxos, common.Utxo{
							Id: shelley.ShelleyTransactionInput{
								TxId:        utxoId.Hash,
								OutputIndex: uint32(utxoId.Idx),
							},
							Output: output,
						})
					}
				}
			}
		}

		// Try using cbor.Value for complex key structures
		var val cbor.Value
		if _, err := cbor.Decode(utxoData, &val); err == nil {
			if m, ok := val.Value().(map[any]any); ok {
				s.logger.Logf(
					"Decoded UTxOs using cbor.Value map[any]any format: %d entries",
					len(m),
				)
				for k, v := range m {
					// k is *interface{}, dereference
					var key any
					if ptr, ok := k.(*interface{}); ok && ptr != nil {
						key = *ptr
					} else {
						key = k
					}
					// Try to extract hash and index from key
					var hash ledger.Blake2b256
					var index uint32
					if arr, ok := key.([]any); ok && len(arr) == 2 {
						if h, ok := arr[0].([]byte); ok {
							copy(hash[:], h)
						}
						if i, ok := arr[1].(uint64); ok {
							index = uint32(i)
						}
					} else if id, ok := key.(localstatequery.UtxoId); ok {
						hash = id.Hash
						index = uint32(id.Idx)
					} else {
						// Try encoding key and decoding as UtxoId
						if keyData, err := cbor.Encode(key); err == nil {
							var utxoId localstatequery.UtxoId
							if _, err := cbor.Decode(keyData, &utxoId); err == nil {
								hash = utxoId.Hash
								index = uint32(utxoId.Idx)
							}
						}
					}
					// Decode output
					var output babbage.BabbageTransactionOutput
					if outData, err := cbor.Encode(v); err == nil {
						if _, err := cbor.Decode(outData, &output); err == nil {
							s.utxos = append(s.utxos, common.Utxo{
								Id: shelley.ShelleyTransactionInput{
									TxId:        hash,
									OutputIndex: index,
								},
								Output: output,
							})
						}
					}
				}
				continue
			}
		}

		// Could not decode this element
		// Continue to next element
	}
	return nil
}

func decodeInitialStateUtxos(t testing.TB, raw cbor.RawMessage) []common.Utxo {
	var state initialStateUtxos
	state.logger = t
	if _, err := cbor.Decode(raw, &state); err != nil {
		// Debug: log the decoding failure to understand UTxO format
		t.Logf("UTxO decoding failed: %v, raw CBOR length: %d", err, len(raw))
		return []common.Utxo{}
	}

	t.Logf("Decoded %d UTxOs from initial state", len(state.utxos))
	return state.utxos
}

// findPParamsByHash searches the pparams-by-hash directory for a file named by the given hash
func findPParamsByHash(
	t testing.TB,
	conwayDumpRoot string,
	hash []byte,
) string {
	if len(hash) == 0 {
		return ""
	}
	// pparams-by-hash is under the dump root, not under Conway/
	dir := filepath.Join(filepath.Dir(conwayDumpRoot), "pparams-by-hash")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read pparams-by-hash: %v", err)
	}
	target := hex.EncodeToString(hash)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == target {
			return filepath.Join(dir, e.Name())
		}
	}
	// Some datasets may prefix the filename; fallback to contains
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.Contains(e.Name(), target) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// loadProtocolParameters decodes the protocol parameters from a pparams file
func loadProtocolParameters(
	t testing.TB,
	pparamsFile string,
) common.ProtocolParameters {
	data, err := os.ReadFile(pparamsFile)
	if err != nil {
		t.Fatalf("failed to read pparams file: %v", err)
	}
	pp := &conway.ConwayProtocolParameters{}
	if _, err := cbor.Decode(data, pp); err != nil {
		t.Fatalf("failed to decode protocol parameters: %v", err)
	}
	return pp
}

// decodeTransaction decodes a transaction from CBOR bytes
func decodeTransaction(t testing.TB, txBytes []byte) common.Transaction {
	tx := &conway.ConwayTransaction{}
	if _, err := cbor.Decode(txBytes, tx); err != nil {
		t.Fatalf("failed to decode transaction: %v", err)
	}
	return tx
}

// executeTransaction runs a transaction through the validation rules with the given UTxOs
func executeTransaction(
	t testing.TB,
	tx common.Transaction,
	slot uint64,
	pp common.ProtocolParameters,
	utxos []common.Utxo,
) (bool, error) {
	// Use preview network (testnet variant) for all conformance tests
	// as specified by Amaru implementation (NetworkName::Testnet(1))
	// Preview network uses AddressNetworkTestnet (0) in address encoding
	networkId := uint(common.AddressNetworkTestnet)

	// Create mock ledger state with the UTxOs from initial_state
	ls := &test_ledger.MockLedgerState{
		NetworkIdVal: networkId,
		UtxoByIdFunc: func(id common.TransactionInput) (common.Utxo, error) {
			for _, u := range utxos {
				if u.Id == id {
					return u, nil
				}
			}
			return common.Utxo{}, fmt.Errorf("utxo not found for %v", id)
		},
	}

	// Try to validate the transaction
	err := common.VerifyTransaction(
		tx,
		slot,
		ls,
		pp,
		conway.UtxoValidationRules,
	)
	return err == nil, err
}
