//go:build !windows

package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"os"
	"slices"
	"sort"
	"testing"

	zip "github.com/sprungknoedl/zip"
	"www.velocidex.com/golang/go-ntfs/parser"
)

// These tests exercise the platform-independent raw-NTFS core (ntfs.go) against
// a vendored NTFS image, so the correctness-critical logic — MFT walk, target
// matching, ADS extraction, and non-resident streaming+hashing — is validated
// on Mac/Linux without a Windows device. They cover the separator-independent
// logic only; Windows backslash-path handling and the \\.\C: device layer
// (collection_windows.go) still run only on real Windows.
//
// testdata/test.ntfs.dd.gz is the uncompressed go-ntfs fixture (parser/test_data/
// test.ntfs.dd, gzipped 10 MB -> ~180 KB). Golden values below were computed
// directly from that image's source bytes.

// TestMain initializes the package-level globals the core touches (Jrnl and the
// loggers), mirroring main()'s setup but discarding output. Without this, the
// dir-skip / warn paths would nil-panic.
func TestMain(m *testing.M) {
	InfoLogger = log.New(io.Discard, "INFO: ", 0)
	WarnLogger = log.New(io.Discard, "WARN: ", 0)
	ErrLogger = log.New(io.Discard, "ERRR: ", 0)
	Jrnl = NewJournal(&bytes.Buffer{})
	os.Exit(m.Run())
}

// loadTestNTFS decompresses the vendored fixture into memory and returns an NTFS
// context over it, wrapped in SectorReaderAt to exercise the same alignment
// adapter the Windows device path uses. bytes.Reader satisfies io.ReaderAt.
func loadTestNTFS(t *testing.T) *parser.NTFSContext {
	t.Helper()

	f, err := os.Open("testdata/test.ntfs.dd.gz")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	data, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("decompress fixture: %v", err)
	}

	ntfs, err := parser.GetNTFSContext(NewSectorReaderAt(bytes.NewReader(data), 512), 0)
	if err != nil {
		t.Fatalf("get ntfs context: %v", err)
	}
	return ntfs
}

// Case 1: walkNTFS enumerates the MFT and the matcher selects a nested file.
func TestWalkNTFSMatchesNestedFile(t *testing.T) {
	ntfs := loadTestNTFS(t)

	// Glob over the forward-slash paths the walk yields when rooted at "/".
	matchers := []Matcher{mustGlob(t, "/Folder A/Folder B/*.txt")}

	targets, scanned, err := walkNTFS(ntfs, "/", matchers, nil, nil)
	if err != nil {
		t.Fatalf("walkNTFS: %v", err)
	}
	if scanned < 2 {
		t.Errorf("expected a real walk to scan multiple entries, got scanned=%d", scanned)
	}

	const want = "/Folder A/Folder B/Hello world text document.txt"
	var found *CollectTarget
	for i := range targets {
		if targets[i].Path == want {
			found = &targets[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("nested file %q not found among %d targets: %v", want, len(targets), targets)
	}
	if found.Source != "match" {
		t.Errorf("target source = %q, want %q", found.Source, "match")
	}
}

// Case 1b: a directory in the skip set is pruned — its children never reach the
// target list even though they match.
func TestWalkNTFSSkipsDir(t *testing.T) {
	ntfs := loadTestNTFS(t)

	matchers := []Matcher{mustGlob(t, "/Folder A/Folder B/*.txt")}
	const child = "/Folder A/Folder B/Hello world text document.txt"

	// Sanity: with no skip set, the nested match is collected.
	targets, _, err := walkNTFS(ntfs, "/", matchers, nil, nil)
	if err != nil {
		t.Fatalf("walkNTFS (no skip): %v", err)
	}
	if !containsTarget(targets, child) {
		t.Fatalf("expected %q to be collected without a skip set", child)
	}

	// With "/Folder A" skipped, the subtree is pruned and the child disappears.
	set := skipDirSet(Configuration{SkipDirs: []string{"/Folder A"}})
	targets, _, err = walkNTFS(ntfs, "/", matchers, set, nil)
	if err != nil {
		t.Fatalf("walkNTFS (skip): %v", err)
	}
	if containsTarget(targets, child) {
		t.Errorf("%q should have been pruned by skip-dir /Folder A", child)
	}
}

func containsTarget(targets []CollectTarget, path string) bool {
	for _, tg := range targets {
		if tg.Path == path {
			return true
		}
	}
	return false
}

// sortedTargetPaths returns the targets' paths sorted, for set comparison.
func sortedTargetPaths(targets []CollectTarget) []string {
	paths := make([]string, 0, len(targets))
	for _, tg := range targets {
		paths = append(paths, tg.Path)
	}
	sort.Strings(paths)
	return paths
}

// noopUnprunable returns a matcher with an empty literal prefix (so it disables
// directory pruning) that matches nothing in the test image. Appending it forces
// a full walk for differential comparison without changing the collected set —
// the production seam for "pruning off", with no test-only branch in walkNTFS.
func noopUnprunable(t *testing.T) Matcher {
	return mustGlob(t, "**/__no_such_marker_zzz__/**")
}

// Case 1c: a focused literal prefix prunes the sibling subtrees — the nested
// match is still collected, but the walk enumerates strictly fewer entries than
// the unpruned run (the $Extend / $RECYCLE.BIN / System Volume Information
// subtrees are never descended).
func TestWalkNTFSFocusedPrunePrunes(t *testing.T) {
	ntfs := loadTestNTFS(t)

	matchers := []Matcher{mustGlob(t, "/Folder A/Folder B/*.txt")}
	const child = "/Folder A/Folder B/Hello world text document.txt"

	// The focused matcher's prefix is non-empty, so this walk prunes.
	pruned, prunedScanned, err := walkNTFS(ntfs, "/", matchers, nil, nil)
	if err != nil {
		t.Fatalf("walkNTFS (pruned): %v", err)
	}
	// Adding the unprunable no-op matcher forces a full walk over the same matches.
	full, fullScanned, err := walkNTFS(ntfs, "/", []Matcher{matchers[0], noopUnprunable(t)}, nil, nil)
	if err != nil {
		t.Fatalf("walkNTFS (full): %v", err)
	}

	if !containsTarget(pruned, child) {
		t.Errorf("nested match %q should still be collected when pruning", child)
	}
	if prunedScanned >= fullScanned {
		t.Errorf("pruning should enumerate fewer entries: pruned=%d, full=%d", prunedScanned, fullScanned)
	}
	if !slices.Equal(sortedTargetPaths(pruned), sortedTargetPaths(full)) {
		t.Errorf("pruned set != unpruned set:\n pruned=%v\n full=%v", sortedTargetPaths(pruned), sortedTargetPaths(full))
	}
}

// TestWalkNTFSPruningPreservesResults is the PRIMARY differential test for the
// raw-NTFS codepath: each config walks the vendored image once with pruning as
// the matchers imply and once with pruning forced off, asserting the collected
// sets are identical. Each case also pins whether the matcher shape enables
// pruning, so a future change to prunable() — e.g. one that wrongly prunes a
// leading-** glob, which must stay off — is caught immediately. The shapes are
// spelled out explicitly rather than read from the OS-specific shipped defaults
// so the test asserts the same desired behaviour on every platform.
func TestWalkNTFSPruningPreservesResults(t *testing.T) {
	ntfs := loadTestNTFS(t)

	cases := []struct {
		name      string
		matchers  []Matcher
		wantPrune bool
	}{
		// A leading-** glob has no literal prefix, so it must disable pruning
		// for the whole set (this is the shape the darwin/windows defaults use).
		{"leading **", []Matcher{mustGlob(t, "**/*.txt")}, false},
		// A glob with a literal prefix anchors its matches, so pruning is on
		// (this is the shape the fully path-anchored linux defaults use).
		{"focused", []Matcher{mustGlob(t, "/Folder A/Folder B/*.txt")}, true},
		// One unprunable matcher in the set forces a full walk, so mixing a
		// prefixed glob with a leading-** glob still disables pruning.
		{"mixed leading **", []Matcher{mustGlob(t, "/Folder A/Folder B/*.txt"), mustGlob(t, "**/*.txt")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, prune := prunable(tc.matchers); prune != tc.wantPrune {
				t.Errorf("prunable = %v, want %v", prune, tc.wantPrune)
			}

			pruned, _, err := walkNTFS(ntfs, "/", tc.matchers, nil, nil)
			if err != nil {
				t.Fatalf("walkNTFS (pruned): %v", err)
			}
			// Force a full walk by adding the unprunable no-op matcher.
			full, _, err := walkNTFS(ntfs, "/", append(slices.Clone(tc.matchers), noopUnprunable(t)), nil, nil)
			if err != nil {
				t.Fatalf("walkNTFS (full): %v", err)
			}
			if !slices.Equal(sortedTargetPaths(pruned), sortedTargetPaths(full)) {
				t.Errorf("pruned set != unpruned set:\n pruned=%v\n full=%v", sortedTargetPaths(pruned), sortedTargetPaths(full))
			}
		})
	}
}

// Case 2: collectFromNTFS extracts an alternate data stream and the archived
// bytes round-trip exactly. ADS access is the headline raw-NTFS feature.
func TestCollectFromNTFSAlternateDataStream(t *testing.T) {
	ntfs := loadTestNTFS(t)

	const (
		rel         = "Folder A/Folder B/Hello world text document.txt:goodbye.txt"
		wantContent = "Goodbye cruel world."
		wantSHA256  = "3ca4373a4561dc5140cdd1cd04874a8babd92a331efb6e6dab6e7b7394a28d6c"
	)

	var buf bytes.Buffer
	archive := zip.NewWriter(&buf)

	entry, size, sha256sum, _, err := collectFromNTFS(Configuration{}, archive, ntfs, rel, make([]byte, 1024*1024*10))
	if err != nil {
		t.Fatalf("collectFromNTFS: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	if entry != rel {
		t.Errorf("entry = %q, want %q", entry, rel)
	}
	if size != int64(len(wantContent)) {
		t.Errorf("size = %d, want %d", size, len(wantContent))
	}
	if sha256sum != wantSHA256 {
		t.Errorf("sha256 = %s, want %s", sha256sum, wantSHA256)
	}

	// Read the stored entry back out and byte-compare: proves the source->archive
	// copy is intact end-to-end, not just that the hash tap saw the right bytes.
	got := readZipEntry(t, buf.Bytes(), rel)
	if got != wantContent {
		t.Errorf("archived ADS content = %q, want %q", got, wantContent)
	}
}

// Case 3: collectFromNTFS streams a larger non-resident file, exercising the
// ReadAt chunk loop and the digest tap; assert size + SHA-256.
func TestCollectFromNTFSNonResidentFile(t *testing.T) {
	ntfs := loadTestNTFS(t)

	const (
		rel        = "ones.bin"
		wantSize   = int64(2949120)
		wantSHA256 = "396d2ca10e09a4a70bed0dcf580c13d5608e59c433b8592fd3d1f0ccd87244ce"
	)

	archive := zip.NewWriter(&bytes.Buffer{})
	_, size, sha256sum, _, err := collectFromNTFS(Configuration{}, archive, ntfs, rel, make([]byte, 1024*1024*10))
	if err != nil {
		t.Fatalf("collectFromNTFS: %v", err)
	}
	if err := archive.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	if size != wantSize {
		t.Errorf("size = %d, want %d", size, wantSize)
	}
	if sha256sum != wantSHA256 {
		t.Errorf("sha256 = %s, want %s", sha256sum, wantSHA256)
	}
}

// readZipEntry returns the decompressed contents of the named entry in a zip
// archive held in memory.
func readZipEntry(t *testing.T, archive []byte, name string) string {
	t.Helper()

	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatalf("open zip reader: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %q: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read zip entry %q: %v", name, err)
		}
		return string(b)
	}
	t.Fatalf("entry %q not found in archive", name)
	return ""
}
