package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
)

type Matcher func(string) bool

func NewStaticMatcher(pattern string) Matcher {
	return func(filename string) bool {
		return pattern == filename
	}
}

func NewGlobMatcher(pattern string) Matcher {
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\")
	m, _ := glob.Compile(pattern)
	return m.Match
}

func NewRegexpMatcher(pattern string) Matcher {
	re, _ := regexp.Compile(pattern)
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
	} else {
		r := bytes.NewReader(defaultQuack)
		return ParseQuack(r)
	}
}

func GetPaths(cfg Configuration) ([]string, error) {
	scanned := 0
	matchers, paths, err := LoadMatchers(cfg)
	if err != nil {
		return nil, fmt.Errorf("load matchers: %w", err)
	}

	roots := cfg.CollectionRoots
	if len(roots) == 0 {
		roots = DefaulRootPaths()
	}

	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, info fs.DirEntry, err error) error {
			if err != nil {
				WarnLogger.Printf("traverse | %v", err)
				return fs.SkipDir
			}

			pathTrimmed := strings.TrimPrefix(path, root)
			// InfoLogger.Printf("traverse | trimmed path: %s -> %s", path, pathTrimmed)

			scanned++
			if !info.IsDir() {
				for _, match := range matchers {
					if match(path) || match(pathTrimmed) {
						paths = append(paths, path)
						break
					}
				}
			}

			return nil
		})
		if err != nil {
			return paths, err
		}
	}

	InfoLogger.Printf("traverse | scanned %d paths, resulted in %d files to collect", scanned, len(paths))
	return paths, err
}

func CollectFile(cfg Configuration, archive *zip.Writer, path string) error {
	rel, _ := filepath.Rel(filepath.VolumeName(path)+"/", filepath.ToSlash(path))
	r, err := os.Open(path)
	if err != nil {
		return err
	}
	defer r.Close()

	fi, err := r.Stat()
	if err != nil {
		return err
	}

	fh, err := zip.FileInfoHeader(fi)
	if err != nil {
		return err
	}

	fh.Name = rel
	fh.Method = zip.Deflate
	w, err := archive.CreateHeader(fh)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, r)
	return err
}
