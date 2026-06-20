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

	"github.com/gobwas/glob"
	zip "github.com/yeka/zip"
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

func NewGlobMatcher(pattern string) Matcher {
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\")
	m, _ := glob.Compile(strings.ToLower(pattern))
	return func(filename string) bool {
		return m.Match(filename)
	}
}

func NewRegexpMatcher(pattern string) Matcher {
	re, _ := regexp.Compile("(?i)" + pattern)
	return re.MatchString
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

// archiveEntry creates a zip entry named name, AES-256 encrypted when a
// password is configured and a plain deflate entry otherwise. It is the single
// place the encrypt-vs-plain decision lives, so evidence and `_donald/` metadata
// are protected identically. (CollectFile's plain branch differs only in that it
// preserves the source mod-time via FileInfoHeader.)
func archiveEntry(cfg Configuration, archive *zip.Writer, name string) (io.Writer, error) {
	if cfg.ZipPass != "" {
		return archive.Encrypt(name, cfg.ZipPass, zip.AES256Encryption)
	}
	return archive.Create(name)
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

	var w io.Writer
	if cfg.ZipPass != "" {
		w, err = archive.Encrypt(rel, cfg.ZipPass, zip.AES256Encryption)
		if err != nil {
			return rel, 0, "", "", err
		}
	} else {
		fi, err := r.Stat()
		if err != nil {
			return rel, 0, "", "", err
		}

		fh, err := zip.FileInfoHeader(fi)
		if err != nil {
			return rel, 0, "", "", err
		}

		fh.Name = rel
		fh.Method = zip.Deflate
		w, err = archive.CreateHeader(fh)
		if err != nil {
			return rel, 0, "", "", err
		}
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
