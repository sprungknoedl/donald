package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type TKape struct {
	ID                  string `yaml:"Id"`
	Description         string `yaml:"Description"`
	Author              string `yaml:"Author"`
	Version             string `yaml:"Version"`
	RecreateDirectories bool   `yaml:"RecreateDirectories"`
	Targets             []struct {
		Name      string `yaml:"Name"`
		Category  string `yaml:"Category"`
		Path      string `yaml:"Path"`
		Recursive bool   `yaml:"Recursive"`
		FileMask  string `yaml:"FileMask"`
		Comment   string `yaml:"Comment"`
	} `yaml:"Targets"`
}

type MKape struct {
	ID           string `yaml:"Id"`
	Description  string `yaml:"Description"`
	Category     string `yaml:"Category"`
	Author       string `yaml:"Author"`
	Version      string `yaml:"Version"`
	BinaryUrl    string `yaml:"BinaryUrl"`
	ExportFormat string `yaml:"ExportFormat"`
	WaitTimeout  int    `yaml:"WaitTimeout"`
	FileMask     string `yaml:"FileMask"`

	Processors []struct {
		Executable   string `yaml:"Executable"`
		CommandLine  string `yaml:"CommandLine"`
		ExportFormat string `yaml:"ExportFormat"`
	} `yaml:"Processors"`
}

func ParseKapeTargets(target string, dir string) ([]Matcher, []string, error) {
	targets := map[string]string{}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		targets[strings.ToLower(filepath.Base(path))] = path
		return nil
	})

	return convert(targets, target)
}

func convert(targets map[string]string, name string) ([]Matcher, []string, error) {
	fh, err := os.Open(targets[strings.ToLower(name)])
	if err != nil {
		return nil, nil, err
	}

	tkape := TKape{}
	d := yaml.NewDecoder(fh)
	err = d.Decode(&tkape)
	if err != nil {
		return nil, nil, err
	}

	var matchers []Matcher
	var paths []string
	for _, t := range tkape.Targets {
		switch {
		case strings.HasSuffix(t.Path, ".tkape"):
			m, p, err := convert(targets, t.Path)
			if err != nil {
				return nil, nil, err
			}
			matchers = append(matchers, m...)
			paths = append(paths, p...)

		case t.FileMask == "" && t.Recursive:
			pattern := fmt.Sprintf("%s\\**", strings.TrimSuffix(t.Path, "\\"))
			pattern = strings.ReplaceAll(pattern, "%user%", "*")
			matchers = append(matchers, NewGlobMatcher(pattern))

		case t.FileMask == "" && !t.Recursive:
			pattern := fmt.Sprintf("%s\\*", strings.TrimSuffix(t.Path, "\\"))
			pattern = strings.ReplaceAll(pattern, "%user%", "*")
			matchers = append(matchers, NewGlobMatcher(pattern))

		case t.FileMask != "":
			pattern := fmt.Sprintf("%s\\%s", strings.TrimSuffix(t.Path, "\\"), t.FileMask)
			pattern = strings.ReplaceAll(pattern, "%user%", "*")
			matchers = append(matchers, NewGlobMatcher(pattern))

		default:
			return nil, nil, fmt.Errorf("unsupported kape target: %+v", t)
		}
	}

	return matchers, paths, nil
}
