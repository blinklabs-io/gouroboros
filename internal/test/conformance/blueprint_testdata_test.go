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

package conformance

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	mockconformance "github.com/blinklabs-io/ouroboros-mock/conformance"
)

const (
	blueprintRevision = "0f0c17e1ca24b062c868d216ae50708fc19c83ab"
	blueprintSHA256   = "574ff7a17857dfc1f0cf477f7eb9eba1c2a0f901453396a779de4b2392ef6863"
	blueprintVectors  = 2574
	blueprintPParams  = 78
	syntheticVectors  = 1
)

type blueprintCorpus struct {
	vectorCount    int
	pparamsCount   int
	syntheticCount int
}

// prepareBlueprintTestdata materializes the pinned Blueprint archive next to
// the synthetic vectors embedded by ouroboros-mock. Keeping the archive in the
// nested submodule avoids depending on generated files being present in the
// downloaded Go module for ouroboros-mock.
func prepareBlueprintTestdata(t *testing.T) (string, blueprintCorpus) {
	t.Helper()

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to locate the conformance test source")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	blueprintDir := filepath.Join(repoRoot, "internal", "test", "cardano-blueprint")
	archivePath := filepath.Join(
		blueprintDir,
		"src",
		"ledger",
		"conformance-test-vectors",
		"vectors.tar.gz",
	)

	if _, err := os.Stat(filepath.Join(blueprintDir, ".git")); err != nil {
		t.Fatalf("Cardano Blueprint submodule is not initialized: %v", err)
	}
	actualRevision, err := gitRevision(blueprintDir)
	if err != nil {
		t.Fatalf("failed to read Cardano Blueprint revision: %v", err)
	}
	if actualRevision != blueprintRevision {
		t.Fatalf("unexpected Cardano Blueprint revision: got %s, want %s", actualRevision, blueprintRevision)
	}

	actualSHA256, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatalf("failed to hash Cardano Blueprint archive: %v", err)
	}
	if actualSHA256 != blueprintSHA256 {
		t.Fatalf("unexpected Cardano Blueprint archive checksum: got %s, want %s", actualSHA256, blueprintSHA256)
	}

	testdataRoot, err := mockconformance.ExtractEmbeddedTestdata(t.TempDir())
	if err != nil {
		t.Fatalf("failed to extract shared conformance testdata: %v", err)
	}
	// A source checkout of ouroboros-mock may have generated eras/ files from
	// its development script embedded in the local replacement used by
	// validation. The pinned Blueprint archive below is the source of truth.
	erasedPath := filepath.Join(testdataRoot, "eras")
	if err := os.RemoveAll(erasedPath); err != nil {
		t.Fatalf("failed to clear extracted ledger testdata: %v", err)
	}
	corpus, err := extractBlueprintArchive(archivePath, erasedPath)
	if err != nil {
		t.Fatalf("failed to extract Cardano Blueprint archive: %v", err)
	}
	if corpus.vectorCount != blueprintVectors || corpus.pparamsCount != blueprintPParams {
		t.Fatalf(
			"unexpected Blueprint inventory: got %d vectors and %d protocol-parameter files, want %d and %d",
			corpus.vectorCount,
			corpus.pparamsCount,
			blueprintVectors,
			blueprintPParams,
		)
	}

	corpus.syntheticCount = countSyntheticVectors(testdataRoot, t)
	if corpus.syntheticCount != syntheticVectors {
		t.Fatalf("unexpected synthetic vector count: got %d, want %d", corpus.syntheticCount, syntheticVectors)
	}
	return testdataRoot, corpus
}

func gitRevision(dir string) (string, error) {
	gitPath := filepath.Join(dir, ".git")
	gitInfo, err := os.Stat(gitPath)
	if err != nil {
		return "", err
	}
	gitDir := gitPath
	if !gitInfo.IsDir() {
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return "", err
		}
		const prefix = "gitdir: "
		location := strings.TrimSpace(string(data))
		if !strings.HasPrefix(location, prefix) {
			return "", fmt.Errorf("invalid gitdir file %q", gitPath)
		}
		gitDir = filepath.Clean(filepath.Join(dir, strings.TrimPrefix(location, prefix)))
	}

	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(head))
	if !strings.HasPrefix(value, "ref: ") {
		return value, nil
	}
	ref := strings.TrimPrefix(value, "ref: ")
	if revision, err := os.ReadFile(filepath.Join(gitDir, ref)); err == nil {
		return strings.TrimSpace(string(revision)), nil
	}
	packed, err := os.ReadFile(filepath.Join(gitDir, "packed-refs"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(packed), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == ref {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("git ref %q not found", ref)
}

func fileSHA256(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func extractBlueprintArchive(archivePath, destination string) (blueprintCorpus, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return blueprintCorpus{}, err
	}
	defer file.Close()

	compressed, err := gzip.NewReader(file)
	if err != nil {
		return blueprintCorpus{}, err
	}
	defer compressed.Close()

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return blueprintCorpus{}, err
	}
	reader := tar.NewReader(compressed)
	seen := make(map[string]string)
	var corpus blueprintCorpus

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return blueprintCorpus{}, err
		}

		components, err := safeBlueprintPath(header.Name)
		if err != nil {
			return blueprintCorpus{}, err
		}
		if components[0] != "eras" {
			return blueprintCorpus{}, fmt.Errorf("unexpected archive path %q", header.Name)
		}
		if len(components) == 1 {
			if header.Typeflag != tar.TypeDir {
				return blueprintCorpus{}, fmt.Errorf("unexpected archive entry %q", header.Name)
			}
			continue
		}
		components = components[1:]
		for index := range components {
			components[index] = normalizeBlueprintComponent(components[index])
			if components[index] == "" {
				return blueprintCorpus{}, fmt.Errorf("empty normalized component in %q", header.Name)
			}
		}
		relative := filepath.Join(components...)
		if previous, ok := seen[relative]; ok && previous != header.Name {
			return blueprintCorpus{}, fmt.Errorf(
				"normalized archive path collision: %q and %q -> %q",
				previous,
				header.Name,
				relative,
			)
		}
		seen[relative] = header.Name
		outputPath := filepath.Join(destination, relative)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(outputPath, 0o755); err != nil {
				return blueprintCorpus{}, err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
				return blueprintCorpus{}, err
			}
			output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return blueprintCorpus{}, err
			}
			_, copyErr := io.Copy(output, reader)
			closeErr := output.Close()
			if copyErr != nil {
				return blueprintCorpus{}, copyErr
			}
			if closeErr != nil {
				return blueprintCorpus{}, closeErr
			}
			if containsComponent(components, "pparams-by-hash") {
				corpus.pparamsCount++
			} else {
				corpus.vectorCount++
			}
		default:
			return blueprintCorpus{}, fmt.Errorf("unsupported archive entry %q", header.Name)
		}
	}
	return corpus, nil
}

func safeBlueprintPath(filename string) ([]string, error) {
	filename = strings.TrimRight(filepath.ToSlash(filename), "/")
	if filename == "" || path.IsAbs(filename) {
		return nil, fmt.Errorf("unsafe archive path %q", filename)
	}
	components := strings.Split(filepath.ToSlash(filename), "/")
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, fmt.Errorf("unsafe archive path %q", filename)
		}
	}
	return components, nil
}

func normalizeBlueprintComponent(component string) string {
	var builder strings.Builder
	underscore := false
	for _, char := range component {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '.' || char == '-' {
			builder.WriteRune(char)
			underscore = false
		} else {
			if !underscore {
				builder.WriteByte('_')
			}
			underscore = true
		}
	}
	return strings.TrimRight(builder.String(), "_")
}

func containsComponent(components []string, wanted string) bool {
	for _, component := range components {
		if component == wanted {
			return true
		}
	}
	return false
}

func countSyntheticVectors(testdataRoot string, t *testing.T) int {
	t.Helper()
	vectors, err := mockconformance.CollectVectorFiles(filepath.Join(testdataRoot, "synthetic"))
	if err != nil {
		t.Fatalf("failed to collect synthetic vectors: %v", err)
	}
	return len(vectors)
}

// collectVectorFiles intentionally owns the final walk instead of calling the
// shared collector. The current shared collector treats any path containing
// "scripts/" as a generated helper directory, which also matches a real
// Blueprint vector directory named can_use_reference_scripts.
func collectVectorFiles(testdataRoot string) ([]string, error) {
	var vectors []string
	for _, subdirectory := range []string{"eras", "synthetic"} {
		root := filepath.Join(testdataRoot, subdirectory)
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) && subdirectory == "synthetic" {
				continue
			}
			return nil, err
		}
		if err := filepath.WalkDir(root, func(pathname string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			normalized := filepath.ToSlash(pathname)
			if entry.IsDir() {
				if strings.Contains(normalized, "/pparams-by-hash") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.Contains(normalized, "/pparams-by-hash/") {
				return nil
			}
			base := filepath.Base(pathname)
			if base == "README" || strings.HasSuffix(base, ".md") {
				return nil
			}
			vectors = append(vectors, pathname)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(vectors)
	return vectors, nil
}

type coverageKey struct {
	era      string
	family   string
	expected string
}

type coverageBucket struct {
	vectors       map[string]struct{}
	transactions  int
	passedVectors map[string]struct{}
	failedVectors map[string]struct{}
}

type coverageSummary struct {
	buckets               map[coverageKey]*coverageBucket
	acceptedTransactions  int
	rejectedTransactions  int
	referenceTransactions int
}

func summarizeCoverage(t *testing.T, testdataRoot string, vectorPaths []string) coverageSummary {
	t.Helper()
	summary := coverageSummary{buckets: make(map[coverageKey]*coverageBucket)}
	for _, vectorPath := range vectorPaths {
		vector, err := mockconformance.DecodeTestVector(vectorPath)
		if err != nil {
			t.Fatalf("failed to decode vector %q: %v", vectorPath, err)
		}
		relative := relativeVectorPath(testdataRoot, vectorPath)
		for _, event := range vector.Events {
			if event.Type != mockconformance.EventTypeTransaction {
				continue
			}
			key := coverageKeyFor(vector.Title, event.Success)
			bucket := summary.buckets[key]
			if bucket == nil {
				bucket = &coverageBucket{
					vectors:       make(map[string]struct{}),
					passedVectors: make(map[string]struct{}),
					failedVectors: make(map[string]struct{}),
				}
				summary.buckets[key] = bucket
			}
			bucket.vectors[relative] = struct{}{}
			bucket.transactions++
			if event.Success {
				summary.acceptedTransactions++
			} else {
				summary.rejectedTransactions++
			}
			if strings.Contains(strings.ToLower(vector.Title), "reference") {
				summary.referenceTransactions++
			}
		}
	}
	if summary.referenceTransactions == 0 {
		t.Fatal("pinned corpus did not exercise any reference-input vectors")
	}
	if summary.acceptedTransactions == 0 || summary.rejectedTransactions == 0 {
		t.Fatalf(
			"pinned corpus must contain both accepted and rejected transactions: accepted=%d rejected=%d",
			summary.acceptedTransactions,
			summary.rejectedTransactions,
		)
	}
	return summary
}

func recordVectorResult(summary coverageSummary, testdataRoot, vectorPath string, passed bool) {
	vector, err := mockconformance.DecodeTestVector(vectorPath)
	if err != nil {
		return
	}
	relative := relativeVectorPath(testdataRoot, vectorPath)
	for _, event := range vector.Events {
		if event.Type != mockconformance.EventTypeTransaction {
			continue
		}
		bucket := summary.buckets[coverageKeyFor(vector.Title, event.Success)]
		if passed {
			bucket.passedVectors[relative] = struct{}{}
		} else {
			bucket.failedVectors[relative] = struct{}{}
		}
	}
}

func coverageKeyFor(title string, success bool) coverageKey {
	expected := "rejected"
	if success {
		expected = "accepted"
	}
	if strings.HasPrefix(title, "synthetic/") {
		parts := strings.Split(title, "/")
		family := "unknown"
		if len(parts) > 1 {
			family = parts[1]
		}
		return coverageKey{era: "synthetic", family: family, expected: expected}
	}
	parts := strings.Split(title, ".")
	for index, part := range parts {
		if spec := strings.Index(part, "ImpSpec"); spec > 0 {
			family := "unknown"
			if index+1 < len(parts) {
				family = parts[index+1]
			}
			return coverageKey{era: part[:spec], family: family, expected: expected}
		}
	}
	return coverageKey{era: "unknown", family: "unknown", expected: expected}
}

func logCoverage(t *testing.T, summary coverageSummary) {
	t.Helper()
	keys := make([]coverageKey, 0, len(summary.buckets))
	for key := range summary.buckets {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].era != keys[right].era {
			return keys[left].era < keys[right].era
		}
		if keys[left].family != keys[right].family {
			return keys[left].family < keys[right].family
		}
		return keys[left].expected < keys[right].expected
	})

	t.Logf("Ledger coverage: %d era/rule/expected-result categories; reference-input transactions: %d", len(keys), summary.referenceTransactions)
	for _, key := range keys {
		bucket := summary.buckets[key]
		t.Logf(
			"Coverage era=%s rule=%s expected=%s vectors=%d transactions=%d vector_pass=%d vector_fail=%d",
			key.era,
			key.family,
			key.expected,
			len(bucket.vectors),
			bucket.transactions,
			len(bucket.passedVectors),
			len(bucket.failedVectors),
		)
	}
}

func relativeVectorPath(root, filename string) string {
	if relative, err := filepath.Rel(root, filename); err == nil {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(filename)
}
