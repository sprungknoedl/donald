//go:build !windows

package main

import (
	zip "github.com/sprungknoedl/zip"
)

func DefaulRootPaths() []string {
	return []string{
		"/",
	}
}

// Raw NTFS access is Windows-only; on unix the *Raw functions delegate to the
// normal filesystem codepath in collection.go.

func GetPathsRaw(cfg Configuration) ([]CollectTarget, error) {
	return GetPaths(cfg)
}

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) (string, int64, string, string, error) {
	return CollectFile(cfg, archive, path)
}

// CloseRawCollectors is a no-op off Windows; there is no cached raw volume
// state to release (the *Raw functions delegate to the normal codepath).
func CloseRawCollectors() {}
