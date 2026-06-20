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

type Matcher func(string) bool

// All matchers are case-insensitive, mirroring CyLR's behaviour.

func NewStaticMatcher(pattern string) Matcher {
	return func(filename string) bool {
		return strings.EqualFold(pattern, filename)
	}
}

func NewGlobMatcher(pattern string) Matcher {
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\")
	m, _ := glob.Compile(strings.ToLower(pattern))
	return func(filename string) bool {
		return m.Match(strings.ToLower(filename))
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

func GetPaths(cfg Configuration) ([]CollectTarget, error) {
	scanned := 0
	matchers, forced, err := LoadMatchers(cfg)
	if err != nil {
		return nil, fmt.Errorf("load matchers: %w", err)
	}

	var targets []CollectTarget
	for _, p := range forced {
		targets = append(targets, CollectTarget{Path: p, Source: "force"})
	}

	roots := cfg.CollectionRoots
	if len(roots) == 0 {
		roots = DefaulRootPaths()
	}

	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				WarnLogger.Printf("traverse | %v", err)
				Jrnl.RecordDirSkipped(path, err)
				return fs.SkipDir
			}

			pathTrimmed := strings.TrimPrefix(path, root)
			// InfoLogger.Printf("traverse | trimmed path: %s -> %s", path, pathTrimmed)

			scanned++
			if !info.IsDir() {
				for _, match := range matchers {
					if match(path) || match(pathTrimmed) {
						targets = append(targets, CollectTarget{Path: path, Source: "match"})
						break
					}
				}
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
	h256 := sha256.New()
	hmd5 := md5.New()
	size, err := io.Copy(io.MultiWriter(w, h256, hmd5), r)
	if err != nil {
		return rel, 0, "", "", err
	}

	return rel, size, hex.EncodeToString(h256.Sum(nil)), hex.EncodeToString(hmd5.Sum(nil)), nil
}
