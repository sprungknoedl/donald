package main

import (
	zip "github.com/yeka/zip"

	_ "embed"
)

//go:embed targets/default_linux.quack
var defaultQuack []byte

func DefaulRootPaths() []string {
	return []string{
		"/",
	}
}

func GetPathsRaw(cfg Configuration) ([]string, error) {
	return GetPaths(cfg)
}

func CollectFileRaw(cfg Configuration, archive *zip.Writer, path string) error {
	return CollectFile(cfg, archive, path)
}
