package main

import (
	"archive/zip"
	"bufio"
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
	m := DefaultCollection()
	paths := ForcedFiles()

	// replace default matchers and forced files
	if cfg.CustomListReplace {
		m = []Matcher{}
		paths = []string{}
	}

	// parse collector configuration
	if cfg.CustomListFile != "" {
		fh, err := os.Open(cfg.CustomListFile)
		if err != nil {
			return nil, nil, err
		}

		line := 0
		scanner := bufio.NewScanner(fh)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			line++
			text := scanner.Text()
			if text == "" || text[0] == '#' {
				continue
			}

			matcher, pattern, ok := strings.Cut(text, "\t")
			if !ok {
				return nil, nil, fmt.Errorf("line %d: missing tab delimiter", line)
			}

			switch matcher {
			case "static":
				m = append(m, NewStaticMatcher(pattern))

			case "glob":
				m = append(m, NewGlobMatcher(pattern))

			case "regex":
				m = append(m, NewRegexpMatcher(pattern))

			case "force":
				paths = append(paths, pattern)

			default:
				return nil, nil, fmt.Errorf("line %d: unknown matcher %q", line, matcher)
			}
		}
	}

	return m, paths, nil
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
