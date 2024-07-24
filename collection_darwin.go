package main

import (
	"archive/zip"

	_ "embed"
)

//go:embed targets/default_darwin.quack
var defaultQuack []byte

func DefaulRootPaths() []string {
	return []string{
		"/",
	}
}

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) error {
	return CollectFile(cfg, archive, path)
}
