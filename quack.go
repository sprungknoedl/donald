package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

func ParseQuack(r io.Reader) ([]Matcher, []string, error) {
	m := []Matcher{}
	paths := []string{}

	line := 0
	scanner := bufio.NewScanner(r)
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
			matcher, err := NewGlobMatcher(pattern)
			if err != nil {
				return nil, nil, fmt.Errorf("line %d: invalid glob: %w", line, err)
			}
			m = append(m, matcher)

		case "regex":
			matcher, err := NewRegexpMatcher(pattern)
			if err != nil {
				return nil, nil, fmt.Errorf("line %d: invalid regex: %w", line, err)
			}
			m = append(m, matcher)

		case "force":
			paths = append(paths, pattern)

		default:
			return nil, nil, fmt.Errorf("line %d: unknown matcher %q", line, matcher)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}

	return m, paths, nil
}
