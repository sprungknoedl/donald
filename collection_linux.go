package main

import (
	_ "embed"
)

//go:embed targets/default_linux.quack
var defaultQuack []byte

// DefaultSkipDirs lists directories pruned during traversal when no -skip-dir is
// given: pseudo-filesystems and runtime mount roots that never hold evidence.
func DefaultSkipDirs() []string {
	return []string{
		"/proc",
		"/sys",
		"/dev",
		"/run",
	}
}
