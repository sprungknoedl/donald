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
			m = append(m, NewGlobMatcher(pattern))

		case "regex":
			m = append(m, NewRegexpMatcher(pattern))

		case "force":
			paths = append(paths, pattern)

		default:
			return nil, nil, fmt.Errorf("line %d: unknown matcher %q", line, matcher)
		}
	}

	return m, paths, nil
}
