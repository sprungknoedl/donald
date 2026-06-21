package main

import (
	_ "embed"
)

//go:embed targets/default_darwin.quack
var defaultQuack []byte

// DefaultSkipDirs lists directories pruned during traversal when no -skip-dir is
// given. These are duplicate volumes, pseudo-filesystems, and mount roots that
// never themselves hold evidence. Notably /System/Volumes/Data is the data
// volume re-exposed alongside its firmlinked appearance under / — skipping it
// alone roughly halves the walk. Cache trees (which can hold evidence) are
// deliberately excluded.
func DefaultSkipDirs() []string {
	return []string{
		"/System/Volumes/Data",
		"/dev",
		"/Volumes",
		"/net",
		"/home",
		"/System/Volumes/VM",
		"/System/Volumes/Preboot",
		"/System/Volumes/Update",
		"/.vol",
	}
}
