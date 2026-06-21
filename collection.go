package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gobwas/glob"
	zip "github.com/sprungknoedl/zip"
)

// Matcher reports whether a path matches a collection target. Matching is
// case-insensitive, mirroring CyLR's behaviour, but the input is expected to be
// already lowercased by the caller: appendIfMatch folds each path once and
// passes the folded form here, so matchers must not re-fold their input.
type Matcher func(string) bool

func NewStaticMatcher(pattern string) Matcher {
	pattern = strings.ToLower(pattern)
	return func(filename string) bool {
		return pattern == filename
	}
}

func NewGlobMatcher(pattern string) (Matcher, error) {
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\")
	m, err := glob.Compile(strings.ToLower(pattern))
	if err != nil {
		return nil, err
	}
	return func(filename string) bool {
		return m.Match(filename)
	}, nil
}

func NewRegexpMatcher(pattern string) (Matcher, error) {
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		// regexp wraps its message in "error parsing regexp: "; drop it since
		// the caller already labels the failure as an invalid regex.
		return nil, fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "error parsing regexp: "))
	}
	return re.MatchString, nil
}

func LoadMatchers(cfg Configuration) ([]Matcher, []string, error) {
	// parse collector configuration
	if cfg.QuackTargets != "" {
		fh, err := os.Open(cfg.QuackTargets)
		if err != nil {
			return nil, nil, err
		}

		return ParseQuack(fh)
	} else if cfg.KapeTargets != "" {
		return ParseKapeTargets(cfg.KapeTargets, cfg.KapeFiles)
	} else {
		r := bytes.NewReader(defaultQuack)
		return ParseQuack(r)
	}
}

// loadTargetsAndRoots performs the shared prologue of both traversal codepaths:
// it loads the matchers, seeds the target list with the unconditional `force`
// paths, and resolves the collection roots (falling back to the platform default).
func loadTargetsAndRoots(cfg Configuration) (matchers []Matcher, targets []CollectTarget, roots []string, err error) {
	matchers, forced, err := LoadMatchers(cfg)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load matchers: %w", err)
	}

	for _, p := range forced {
		targets = append(targets, CollectTarget{Path: p, Source: "force"})
	}

	roots = cfg.CollectionRoots
	if len(roots) == 0 {
		roots = DefaulRootPaths()
	}

	return matchers, targets, roots, nil
}

// appendIfMatch tests path (and its root-trimmed form) against every matcher and
// appends a "match" target on the first hit. Shared by the normal and raw walks.
func appendIfMatch(targets []CollectTarget, matchers []Matcher, path, root string) []CollectTarget {
	// Trim stays case-sensitive (as today); fold each form once so matchers
	// operate on already-lowercased input instead of folding per call.
	pathLower := strings.ToLower(path)
	pathTrimmedLower := strings.ToLower(strings.TrimPrefix(path, root))
	for _, match := range matchers {
		if match(pathLower) || match(pathTrimmedLower) {
			return append(targets, CollectTarget{Path: path, Source: "match"})
		}
	}
	return targets
}

func GetPaths(cfg Configuration) ([]CollectTarget, error) {
	scanned := 0
	matchers, targets, roots, err := loadTargetsAndRoots(cfg)
	if err != nil {
		return nil, err
	}

	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				WarnLogger.Printf("traverse | %v", err)
				Jrnl.RecordDirSkipped(path, err)
				return fs.SkipDir
			}

			scanned++
			if !info.IsDir() {
				targets = appendIfMatch(targets, matchers, path, root)
			}

			return nil
		})
		if err != nil {
			return targets, err
		}
	}

	Jrnl.SetScanned(scanned)
	InfoLogger.Printf("traverse | scanned %d paths, resulted in %d files to collect", scanned, len(targets))
	return targets, err
}

// zipMethod returns the storage method for an entry given the configured
// -zip-level: Store for level 0 (true no-op container entries) and Deflate
// otherwise. Levels 1..9 are applied via the leveled compressor registered in
// step2CollectFiles; level -1 (unset) keeps the stdlib default Deflate.
func zipMethod(cfg Configuration) uint16 {
	if cfg.CompressionLevel == 0 {
		return zip.Store
	}
	return zip.Deflate
}

// createEntry stamps the cfg-driven compression method and encryption onto fh,
// then creates the archive entry. It is the single place the method/encrypt
// decision lives, so every entry — FileInfo-derived evidence and bare-header
// `_donald/` metadata alike — is compressed and protected identically. It builds
// from a caller-supplied FileHeader (rather than the Encrypt/Create helpers) so
// the source mod-time and mode survive onto the entry even for encrypted output.
func createEntry(cfg Configuration, archive *zip.Writer, fh *zip.FileHeader) (io.Writer, error) {
	fh.Method = zipMethod(cfg)
	if cfg.ZipPass != "" {
		fh.SetPassword(cfg.ZipPass)
		fh.SetEncryptionMethod(zip.AES256Encryption)
	}
	return archive.CreateHeader(fh)
}

// createNamedEntry creates an entry from just a name and modTime, for callers
// with no os.FileInfo to derive a header from: raw-NTFS evidence (mod-time from
// the MFT) and synthesized `_donald/` metadata.
func createNamedEntry(cfg Configuration, archive *zip.Writer, name string, modTime time.Time) (io.Writer, error) {
	return createEntry(cfg, archive, &zip.FileHeader{Name: name, Modified: modTime})
}

// newHashers returns a writer feeding both a SHA-256 and an MD5 hasher and a
// finish func returning their hex digests. Collection tees the source→archive
// copy through the writer so the digests cover the plaintext source bytes with
// no extra read.
func newHashers() (w io.Writer, finish func() (sha256sum, md5sum string)) {
	h256 := sha256.New()
	hmd5 := md5.New()
	return io.MultiWriter(h256, hmd5), func() (string, string) {
		return hex.EncodeToString(h256.Sum(nil)), hex.EncodeToString(hmd5.Sum(nil))
	}
}

func CollectFile(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	rel, _ := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	r, err := os.Open(path)
	if err != nil {
		return rel, 0, "", "", err
	}
	defer r.Close()

	// Build the entry header from the source FileInfo so it carries the file's
	// mod-time and mode, then layer encryption on top when configured — so
	// encrypted entries keep their mod-time too (a plain Encrypt() would drop it).
	fi, err := r.Stat()
	if err != nil {
		return rel, 0, "", "", err
	}

	fh, err := zip.FileInfoHeader(fi)
	if err != nil {
		return rel, 0, "", "", err
	}

	fh.Name = rel

	w, err := createEntry(cfg, archive, fh)
	if err != nil {
		return rel, 0, "", "", err
	}

	// Tee the source→archive copy through the hashers: digests cover the
	// plaintext source bytes, with no extra read. size is the bytes streamed.
	hashes, digests := newHashers()
	size, err := io.Copy(io.MultiWriter(w, hashes), r)
	if err != nil {
		return rel, 0, "", "", err
	}

	sha256sum, md5sum := digests()
	return rel, size, sha256sum, md5sum, nil
}
