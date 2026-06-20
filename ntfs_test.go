//go:build !windows

package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"log"
	"os"
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
	matchers := []Matcher{NewGlobMatcher("/Folder A/Folder B/*.txt")}

	targets, scanned, err := walkNTFS(ntfs, "/", matchers, nil)
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
